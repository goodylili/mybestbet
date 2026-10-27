package seed

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"time"

	"mybestbet/internal/stats/acb"
)

var eventStatusByACB = map[acb.MatchStatus]EventStatus{
	acb.StatusFinalized: EventFinished, acb.StatusStarted: EventLive,
	acb.StatusPostponed: EventPostponed, acb.StatusSuspended: EventCancelled,
}

func StatusFromACB(status acb.MatchStatus) EventStatus {
	if mapped, ok := eventStatusByACB[status]; ok {
		return mapped
	}
	return EventScheduled
}

func (o *OtherSportsSeeder) seedACB(ctx context.Context, league League) error {
	matches, err := o.acbSeasonMatchesUntil(ctx, o.now().Add(UpcomingHorizon))
	if err != nil {
		return o.logSeeded(acb.SourceName, league, 0, 0, err)
	}
	clubTeamIDs := map[string]string{}
	var errs []error
	for _, m := range matches {
		homeID, homeErr := o.acbTeamID(ctx, league, m.Home, clubTeamIDs)
		awayID, awayErr := o.acbTeamID(ctx, league, m.Away, clubTeamIDs)
		if homeErr != nil || awayErr != nil {
			errs = append(errs, homeErr, awayErr)
			continue
		}
		_, err := o.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: acb.SourceName, SourceID: strconv.FormatInt(m.ID, 10), LeagueCode: league.Code,
			HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: m.Start, Status: StatusFromACB(m.Status),
		})
		errs = append(errs, err)
	}
	errs = append(errs, o.seedACBRostersFromLatestBoxscores(ctx, matches, clubTeamIDs))
	return o.logSeeded(acb.SourceName, league, len(clubTeamIDs), len(matches), errors.Join(errs...))
}

func (o *OtherSportsSeeder) acbSeasonMatchesUntil(ctx context.Context, until time.Time) ([]acb.Match, error) {
	weeks, err := o.acb.CurrentWeeks(ctx)
	if err != nil {
		return nil, err
	}
	var matches []acb.Match
	for _, w := range weeks {
		if w.Start.After(until) {
			continue
		}
		weekMatches, err := o.acb.WeekMatches(ctx, w.ID)
		if err != nil {
			return nil, err
		}
		matches = append(matches, weekMatches...)
	}
	return matches, nil
}

func (o *OtherSportsSeeder) acbTeamID(ctx context.Context, league League, t acb.Team, clubTeamIDs map[string]string) (string, error) {
	if id, ok := clubTeamIDs[t.ClubID]; ok {
		return id, nil
	}
	id, err := o.store.UpsertSourceTeam(ctx, SourceTeam{
		Source: acb.SourceName, SourceID: t.ClubID, LeagueCode: league.Code, Name: t.Name, Aliases: []string{t.ShortName, t.Abbreviation}, LogoURL: t.LogoURL,
	})
	if err != nil {
		return "", err
	}
	clubTeamIDs[t.ClubID] = id
	return id, nil
}

func (o *OtherSportsSeeder) seedACBRostersFromLatestBoxscores(ctx context.Context, matches []acb.Match, clubTeamIDs map[string]string) error {
	finished := slices.DeleteFunc(slices.Clone(matches), func(m acb.Match) bool { return m.Status != acb.StatusFinalized })
	slices.SortFunc(finished, func(a, b acb.Match) int { return b.Start.Compare(a.Start) })
	clubsWithoutRoster := maps.Clone(clubTeamIDs)
	var errs []error
	for _, m := range finished {
		if len(clubsWithoutRoster) == 0 {
			break
		}
		if !hasKey(clubsWithoutRoster, m.Home.ClubID) && !hasKey(clubsWithoutRoster, m.Away.ClubID) {
			continue
		}
		box, err := o.acb.Boxscore(ctx, m.ID)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, l := range box.Lines {
			teamID, pending := clubsWithoutRoster[l.Club.ClubID]
			if !pending {
				continue
			}
			_, err := o.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: acb.SourceName, SourceID: l.Player.ID, TeamID: teamID, Name: l.Player.Name})
			errs = append(errs, err)
		}
		delete(clubsWithoutRoster, m.Home.ClubID)
		delete(clubsWithoutRoster, m.Away.ClubID)
	}
	return errors.Join(errs...)
}
