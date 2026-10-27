package flashscore

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	wholeMatchPeriod   = "Match"
	statAces           = "Aces"
	statDoubleFaults   = "Double Faults"
	statTotalPointsWon = "Total Points Won"
)

type ServeStats struct {
	Aces         int
	DoubleFaults int
}

type MatchServeStats struct {
	Home ServeStats
	Away ServeStats
}

func serveStatsFrom(records []record) (MatchServeStats, error) {
	var stats MatchServeStats
	hasAces, hasDoubleFaults, hasPlayedPoints := false, false, true
	inWholeMatch := false
	for _, rec := range records {
		if period, isPeriod := rec[keyStatPeriod]; isPeriod {
			inWholeMatch = period == wholeMatchPeriod
			continue
		}
		if !inWholeMatch {
			continue
		}
		var err error
		switch rec[keyStatName] {
		case statAces:
			stats.Home.Aces, stats.Away.Aces, err = homeAndAwayCounts(rec)
			hasAces = true
		case statDoubleFaults:
			stats.Home.DoubleFaults, stats.Away.DoubleFaults, err = homeAndAwayCounts(rec)
			hasDoubleFaults = true
		case statTotalPointsWon:
			hasPlayedPoints = pointsPlayed(rec[keyStatHome]) > 0
		}
		if err != nil {
			return MatchServeStats{}, err
		}
	}
	if !hasAces || !hasDoubleFaults || !hasPlayedPoints {
		return MatchServeStats{}, ErrNoStats
	}
	return stats, nil
}

func homeAndAwayCounts(rec record) (int, int, error) {
	home, homeErr := strconv.Atoi(rec[keyStatHome])
	away, awayErr := strconv.Atoi(rec[keyStatAway])
	if homeErr != nil || awayErr != nil {
		return 0, 0, fmt.Errorf("flashscore stat %q: %w", rec[keyStatName], ErrMalformedStat)
	}
	return home, away, nil
}

func pointsPlayed(ratio string) int {
	_, fraction, found := strings.Cut(ratio, "/")
	if !found {
		return 0
	}
	played, err := strconv.Atoi(strings.TrimSuffix(fraction, ")"))
	if err != nil {
		return 0
	}
	return played
}
