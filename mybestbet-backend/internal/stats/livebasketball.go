package stats

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"mybestbet/internal/stats/acb"
	"mybestbet/pkg/canon"
)

var basketballQuarters = []string{canon.PeriodQuarter1, canon.PeriodQuarter2, canon.PeriodQuarter3, canon.PeriodQuarter4}

type basketballProgress struct {
	home     int
	away     int
	quarter  int
	halfTime bool
	clock    string
	finished bool
}

func (p basketballProgress) score() canon.EventScore {
	score := canon.EventScore{Home: p.home, Away: p.away}
	switch {
	case p.finished:
		score.Period = canon.PeriodFinished
	case p.halfTime:
		score.Period = canon.PeriodHalfTime
	case p.quarter > len(basketballQuarters):
		score.Period, score.Clock = canon.PeriodOvertime, p.clock
	case p.quarter > 0:
		score.Period, score.Clock = basketballQuarters[p.quarter-1], p.clock
	}
	return score
}

func (c *LiveScoreCollector) euroLeagueScores(ctx context.Context, events []SourcedEvent) ([]liveScore, error) {
	var scores []liveScore
	var errs []error
	for _, e := range events {
		seasonCode, gameCode, err := euroLeagueGameOf(e)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		state, err := c.euroleague.GameState(ctx, seasonCode, gameCode)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if state.Started {
			progress := basketballProgress{home: state.HomeScore, away: state.AwayScore, quarter: state.Quarter, clock: state.Clock, finished: state.Finished}
			scores = append(scores, liveScore{event: e, score: progress.score(), finished: state.Finished})
		}
	}
	return scores, errors.Join(errs...)
}

func euroLeagueGameOf(e SourcedEvent) (string, int, error) {
	seasonCode, gameCodeText, found := strings.Cut(e.SourceID, ":")
	gameCode, err := strconv.Atoi(gameCodeText)
	if !found || err != nil {
		return "", 0, fmt.Errorf("%s: %w", e.SourceID, errMalformedSourceID)
	}
	return seasonCode, gameCode, nil
}

func (c *LiveScoreCollector) acbScores(ctx context.Context, events []SourcedEvent) ([]liveScore, error) {
	weeks, err := c.acb.CurrentWeeks(ctx)
	if err != nil {
		return nil, err
	}
	matches := map[string]acb.Match{}
	fetchedWeeks := map[int]bool{}
	var errs []error
	for _, e := range events {
		week, found := acb.WeekOf(weeks, e.Kickoff)
		if !found || fetchedWeeks[week.ID] {
			continue
		}
		fetchedWeeks[week.ID] = true
		weekMatches, err := c.acb.WeekMatches(ctx, week.ID)
		errs = append(errs, err)
		for _, m := range weekMatches {
			matches[strconv.FormatInt(m.ID, 10)] = m
		}
	}
	var scores []liveScore
	for _, e := range events {
		m, found := matches[e.SourceID]
		if !found || (m.Status != acb.StatusStarted && m.Status != acb.StatusFinalized) {
			continue
		}
		finished := m.Status == acb.StatusFinalized
		progress := basketballProgress{home: m.HomeScore, away: m.AwayScore, quarter: m.Quarter, halfTime: m.HalfTime, finished: finished}
		scores = append(scores, liveScore{event: e, score: progress.score(), finished: finished})
	}
	return scores, errors.Join(errs...)
}
