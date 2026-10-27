package fotmob

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

const (
	dayMatchesDate     = "20060102"
	firstHalfMinutes   = 45
	regulationMinutes  = 90
	stoppageSeparator  = "+"
	liveMinuteSuffixes = "'’"
)

type LiveMatch struct {
	ID       string
	Started  bool
	Finished bool
	Score    canon.EventScore
}

func (c *Client) DayMatches(ctx context.Context, day time.Time) ([]LiveMatch, error) {
	var resp dayResponse
	if err := c.get(ctx, "/matches?date="+url.QueryEscape(day.UTC().Format(dayMatchesDate)), &resp); err != nil {
		return nil, err
	}
	var matches []LiveMatch
	for _, league := range resp.Leagues {
		for _, m := range league.Matches {
			if !m.Status.Cancelled {
				matches = append(matches, liveMatchFrom(m))
			}
		}
	}
	return matches, nil
}

func liveMatchFrom(m dayMatch) LiveMatch {
	live := LiveMatch{ID: string(m.ID), Started: m.Status.Started, Finished: m.Status.Finished}
	live.Score = canon.EventScore{Home: m.Home.Score, Away: m.Away.Score}
	switch {
	case m.Status.Finished:
		live.Score.Period, live.Score.Detail = canon.PeriodFinished, m.Status.Reason.Short
	case m.Status.LiveTime != nil:
		live.Score.Clock = strings.TrimRight(m.Status.LiveTime.Short, liveMinuteSuffixes)
		live.Score.Minute = minuteOf(live.Score.Clock)
		live.Score.Period = periodOf(live.Score.Clock, live.Score.Minute)
	}
	return live
}

func minuteOf(clock string) int {
	regular, _, _ := strings.Cut(clock, stoppageSeparator)
	minute, _ := strconv.Atoi(regular)
	return minute
}

func periodOf(clock string, minute int) string {
	switch {
	case minute == 0:
		return strings.ToUpper(clock)
	case minute <= firstHalfMinutes:
		return canon.PeriodFirstHalf
	case minute <= regulationMinutes:
		return canon.PeriodSecondHalf
	}
	return canon.PeriodExtraTime
}
