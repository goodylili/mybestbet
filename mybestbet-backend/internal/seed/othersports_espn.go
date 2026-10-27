package seed

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"mybestbet/internal/stats/espn"
)

var eventStatusByESPN = map[string]EventStatus{
	"STATUS_SCHEDULED":   EventScheduled,
	"STATUS_IN_PROGRESS": EventLive,
	"STATUS_HALFTIME":    EventLive,
	"STATUS_END_PERIOD":  EventLive,
	"STATUS_FINAL":       EventFinished,
	"STATUS_POSTPONED":   EventPostponed,
	"STATUS_CANCELED":    EventCancelled,
	"STATUS_ABANDONED":   EventCancelled,
}

func StatusFromESPN(statusName string) EventStatus {
	if status, ok := eventStatusByESPN[statusName]; ok {
		return status
	}
	return EventScheduled
}

func ESPNSourceID(leagueCode, espnID string) string {
	return strings.ToLower(leagueCode) + ":" + espnID
}

const tennisSourcePrefix = "tennis"

type espnTeamIDs map[string]string

func (o *OtherSportsSeeder) seedESPNLeague(ctx context.Context, league League) error {
	games, leagueLogo, err := o.espnGamesInWindow(ctx, league.Code)
	if err != nil {
		return err
	}
	teamIDs := espnTeamIDs{}
	errs := []error{o.store.UpsertLeagueLogo(ctx, league.Code, leagueLogo)}
	if espn.Leagues[league.Code].ListsAllTeams {
		errs = append(errs, o.seedAllESPNTeams(ctx, league, teamIDs))
	}
	scheduledTeams := map[string]bool{}
	for _, g := range games {
		homeID, homeErr := o.espnTeamID(ctx, league, g.Home, teamIDs)
		awayID, awayErr := o.espnTeamID(ctx, league, g.Away, teamIDs)
		if homeErr != nil || awayErr != nil {
			errs = append(errs, homeErr, awayErr)
			continue
		}
		scheduledTeams[g.Home.ID], scheduledTeams[g.Away.ID] = true, true
		_, err := o.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: espn.SourceName, SourceID: ESPNSourceID(league.Code, g.ID), LeagueCode: league.Code,
			HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: g.Kickoff, Status: StatusFromESPN(g.Status),
		})
		errs = append(errs, err)
	}
	for espnTeamID := range scheduledTeams {
		errs = append(errs, o.seedESPNRoster(ctx, league, espnTeamID, teamIDs[espnTeamID]))
	}
	return o.logSeeded(espn.SourceName, league, len(teamIDs), len(games), errors.Join(errs...))
}

func (o *OtherSportsSeeder) espnGamesInWindow(ctx context.Context, leagueCode string) ([]espn.Game, string, error) {
	var games []espn.Game
	leagueLogo := ""
	seen := map[string]bool{}
	for _, day := range o.seedWindowDays() {
		board, err := o.espn.Games(ctx, leagueCode, day)
		if err != nil {
			return nil, "", err
		}
		leagueLogo = cmp.Or(leagueLogo, board.LeagueLogoURL)
		for _, g := range board.Games {
			if !seen[g.ID] && o.isInSeedWindow(g.Kickoff) {
				seen[g.ID] = true
				games = append(games, g)
			}
		}
	}
	return games, leagueLogo, nil
}

func (o *OtherSportsSeeder) seedAllESPNTeams(ctx context.Context, league League, teamIDs espnTeamIDs) error {
	teams, err := o.espn.Teams(ctx, league.Code)
	if err != nil {
		return err
	}
	var errs []error
	for _, t := range teams {
		_, err := o.espnTeamID(ctx, league, t, teamIDs)
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (o *OtherSportsSeeder) espnTeamID(ctx context.Context, league League, t espn.Team, teamIDs espnTeamIDs) (string, error) {
	if id, ok := teamIDs[t.ID]; ok {
		return id, nil
	}
	id, err := o.store.UpsertSourceTeam(ctx, SourceTeam{
		Source: espn.SourceName, SourceID: ESPNSourceID(league.Code, t.ID), LeagueCode: league.Code,
		Name: t.Name, Aliases: espnTeamAliases(league.Code, t), LogoURL: t.LogoURL,
	})
	if err != nil {
		return "", err
	}
	teamIDs[t.ID] = id
	return id, nil
}

func espnTeamAliases(leagueCode string, t espn.Team) []string {
	if espn.Leagues[leagueCode].ListsAllTeams {
		return []string{t.ShortName, t.Abbreviation}
	}
	return []string{t.Location}
}

func (o *OtherSportsSeeder) seedESPNRoster(ctx context.Context, league League, espnTeamID, teamID string) error {
	athletes, err := o.espn.Roster(ctx, league.Code, espnTeamID)
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range athletes {
		_, err := o.store.UpsertSourcePlayer(ctx, SourcePlayer{
			Source: espn.SourceName, SourceID: ESPNSourceID(league.Code, a.ID), TeamID: teamID, Name: a.Name, Position: NormalizePosition(league.Sport, a.Position),
		})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (o *OtherSportsSeeder) seedTennisLeague(ctx context.Context, league League) error {
	if o.tennisByLeague == nil {
		if err := o.loadTennisWindow(ctx); err != nil {
			return err
		}
	}
	matches := o.tennisByLeague[league.Code]
	competitors := map[string]string{}
	var errs []error
	for _, m := range matches {
		homeID, homeErr := o.tennisCompetitorID(ctx, league, m.Home, competitors)
		awayID, awayErr := o.tennisCompetitorID(ctx, league, m.Away, competitors)
		if homeErr != nil || awayErr != nil {
			errs = append(errs, homeErr, awayErr)
			continue
		}
		_, err := o.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: espn.SourceName, SourceID: ESPNSourceID(tennisSourcePrefix, m.ID), LeagueCode: league.Code,
			HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: m.Kickoff, Status: StatusFromESPN(m.Status),
		})
		errs = append(errs, err)
	}
	return o.logSeeded(espn.SourceName, league, len(competitors), len(matches), errors.Join(errs...))
}

func (o *OtherSportsSeeder) loadTennisWindow(ctx context.Context) error {
	byLeague := map[string][]espn.TennisMatch{}
	seen := map[string]bool{}
	for _, tour := range espn.TennisTours {
		for _, day := range o.seedWindowDays() {
			matches, err := o.espn.TennisMatches(ctx, tour, day)
			if err != nil {
				return fmt.Errorf("tennis window: %w", err)
			}
			for _, m := range matches {
				if !seen[m.ID] && o.isInSeedWindow(m.Kickoff) {
					seen[m.ID] = true
					byLeague[m.LeagueCode] = append(byLeague[m.LeagueCode], m)
				}
			}
		}
	}
	o.tennisByLeague = byLeague
	return nil
}

func (o *OtherSportsSeeder) tennisCompetitorID(ctx context.Context, league League, a espn.Athlete, competitors map[string]string) (string, error) {
	if id, ok := competitors[a.ID]; ok {
		return id, nil
	}
	teamID, _, err := o.store.UpsertCompetitor(ctx, SourceTeam{
		Source: espn.SourceName, SourceID: ESPNSourceID(tennisSourcePrefix, a.ID), LeagueCode: league.Code, Name: a.Name,
	})
	if err != nil {
		return "", err
	}
	competitors[a.ID] = teamID
	return teamID, nil
}
