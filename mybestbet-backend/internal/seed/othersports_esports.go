package seed

import (
	"context"
	"errors"

	"mybestbet/internal/names"
	"mybestbet/internal/stats/esports"
)

const dotaLeagueCode = "DOTA2"

func LiquipediaSourceID(leagueCode, id string) string { return leagueCode + ":" + id }

func (o *OtherSportsSeeder) seedEsportsLeague(ctx context.Context, league League) error {
	matches, err := o.liquipedia.UpcomingMatches(ctx, league.Code)
	if err != nil {
		return err
	}
	teamIDs := map[string]string{}
	var errs []error
	seeded := 0
	for _, m := range matches {
		if !o.isInSeedWindow(m.Kickoff) {
			continue
		}
		homeID, homeErr := o.esportsTeamID(ctx, league, m.Team1, teamIDs)
		awayID, awayErr := o.esportsTeamID(ctx, league, m.Team2, teamIDs)
		if homeErr != nil || awayErr != nil {
			errs = append(errs, homeErr, awayErr)
			continue
		}
		seeded++
		_, err := o.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: esports.LiquipediaSource, SourceID: LiquipediaSourceID(league.Code, m.ID), LeagueCode: league.Code,
			HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: m.Kickoff, Status: EventScheduled,
		})
		errs = append(errs, err)
	}
	if league.Code == dotaLeagueCode {
		errs = append(errs, o.seedDotaRosters(ctx, league, teamIDs))
	}
	return o.logSeeded(esports.LiquipediaSource, league, len(teamIDs), seeded, errors.Join(errs...))
}

func (o *OtherSportsSeeder) esportsTeamID(ctx context.Context, league League, t esports.Team, teamIDs map[string]string) (string, error) {
	if id, ok := teamIDs[t.Name]; ok {
		return id, nil
	}
	id, err := o.store.UpsertSourceTeam(ctx, SourceTeam{
		Source: esports.LiquipediaSource, SourceID: LiquipediaSourceID(league.Code, t.Name), LeagueCode: league.Code,
		Name: t.Name, Aliases: []string{t.ShortName}, LogoURL: t.LogoURL,
	})
	if err != nil {
		return "", err
	}
	teamIDs[t.Name] = id
	return id, nil
}

func (o *OtherSportsSeeder) seedDotaRosters(ctx context.Context, league League, teamIDs map[string]string) error {
	if o.dotaTeams == nil {
		teams, err := o.opendota.Teams(ctx)
		if err != nil {
			return err
		}
		o.dotaTeams = teams
	}
	var errs []error
	for name, teamID := range teamIDs {
		dotaTeam, ok := FindDotaTeam(o.dotaTeams, name)
		if !ok {
			continue
		}
		errs = append(errs, o.seedDotaRoster(ctx, league, name, teamID, dotaTeam))
	}
	return errors.Join(errs...)
}

func (o *OtherSportsSeeder) seedDotaRoster(ctx context.Context, league League, name, teamID string, dotaTeam esports.DotaTeam) error {
	if _, err := o.store.UpsertSourceTeam(ctx, SourceTeam{
		Source: esports.OpenDotaSource, SourceID: dotaTeam.ID, LeagueCode: league.Code, Name: name, Aliases: []string{dotaTeam.Name, dotaTeam.Tag}, LogoURL: dotaTeam.LogoURL,
	}); err != nil {
		return err
	}
	roster, err := o.opendota.CurrentRoster(ctx, dotaTeam.ID)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range roster {
		_, err := o.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: esports.OpenDotaSource, SourceID: p.AccountID, TeamID: teamID, Name: p.Name})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func FindDotaTeam(teams []esports.DotaTeam, name string) (esports.DotaTeam, bool) {
	normalized := names.Team(name)
	for _, t := range teams {
		if names.Team(t.Name) == normalized {
			return t, true
		}
	}
	return esports.DotaTeam{}, false
}
