package seed

import (
	"context"
	"errors"
	"time"

	"mybestbet/internal/stats/bo3"
)

const bo3SeriesHistory = 30 * 24 * time.Hour

var eventStatusByBo3 = map[string]EventStatus{
	"finished": EventFinished, "defwin": EventFinished, "current": EventLive, "live": EventLive,
	"canceled": EventCancelled, "cancelled": EventCancelled,
}

func StatusFromBo3(status string) EventStatus {
	if mapped, ok := eventStatusByBo3[status]; ok {
		return mapped
	}
	return EventScheduled
}

func Bo3EventSourceID(leagueCode, slug string) string { return leagueCode + ":" + slug }

func (o *OtherSportsSeeder) seedBo3League(ctx context.Context, league League) error {
	if !bo3.Supports(league.Code) {
		return nil
	}
	now := o.now()
	matches, err := o.bo3.Matches(ctx, league.Code, now.Add(-bo3SeriesHistory), now.Add(UpcomingHorizon))
	if err != nil {
		return o.logSeeded(bo3.SourceName, league, 0, 0, err)
	}
	teamIDs := map[string]string{}
	teamsPlayingNext := map[string]bo3.Team{}
	var errs []error
	seeded := 0
	for _, m := range matches {
		homeID, homeErr := o.bo3TeamID(ctx, league, m.Team1, teamIDs)
		awayID, awayErr := o.bo3TeamID(ctx, league, m.Team2, teamIDs)
		if homeErr != nil || awayErr != nil {
			errs = append(errs, homeErr, awayErr)
			continue
		}
		_, err := o.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: bo3.SourceName, SourceID: Bo3EventSourceID(league.Code, m.Slug), LeagueCode: league.Code,
			HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: m.Start, Status: StatusFromBo3(m.Status),
		})
		errs = append(errs, err)
		seeded++
		if !m.IsFinished() {
			teamsPlayingNext[m.Team1.ID], teamsPlayingNext[m.Team2.ID] = m.Team1, m.Team2
		}
	}
	for teamID, team := range teamsPlayingNext {
		errs = append(errs, o.seedBo3Roster(ctx, team, teamIDs[teamID]))
	}
	return o.logSeeded(bo3.SourceName, league, len(teamIDs), seeded, errors.Join(errs...))
}

func (o *OtherSportsSeeder) bo3TeamID(ctx context.Context, league League, t bo3.Team, teamIDs map[string]string) (string, error) {
	if id, ok := teamIDs[t.ID]; ok {
		return id, nil
	}
	id, err := o.store.UpsertSourceTeam(ctx, SourceTeam{Source: bo3.SourceName, SourceID: t.ID, LeagueCode: league.Code, Name: t.Name, LogoURL: t.LogoURL})
	if err != nil {
		return "", err
	}
	teamIDs[t.ID] = id
	return id, nil
}

func (o *OtherSportsSeeder) seedBo3Roster(ctx context.Context, team bo3.Team, teamID string) error {
	roster, err := o.bo3.Roster(ctx, team.Slug)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range roster {
		_, err := o.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: bo3.SourceName, SourceID: p.ID, TeamID: teamID, Name: p.Name})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
