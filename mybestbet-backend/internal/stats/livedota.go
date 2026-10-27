package stats

import (
	"context"
	"errors"
	"strconv"

	"mybestbet/internal/stats/esports"
	"mybestbet/pkg/canon"
)

func (c *LiveScoreCollector) dotaScores(ctx context.Context, events []SourcedEvent) ([]liveScore, error) {
	finishedGames, finishedErr := c.opendota.ProGames(ctx)
	liveGames, liveErr := c.opendota.LiveGames(ctx)
	if finishedErr != nil && liveErr != nil {
		return nil, errors.Join(finishedErr, liveErr)
	}
	errs := []error{finishedErr, liveErr}
	games := withLiveGames(finishedGames, liveGames)
	var scores []liveScore
	for _, e := range events {
		series, err := dotaSeriesGamesOf(ctx, c.store, e, games)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(series) > 0 {
			score, finished := dotaSeriesScore(series)
			scores = append(scores, liveScore{event: e, score: score, finished: finished})
		}
	}
	return scores, errors.Join(errs...)
}

func withLiveGames(finished, live []esports.ProGame) []esports.ProGame {
	known := make(map[int64]bool, len(finished))
	for _, g := range finished {
		known[g.MatchID] = true
	}
	games := finished
	for _, g := range live {
		if !known[g.MatchID] {
			games = append(games, g)
		}
	}
	return games
}

func dotaSeriesScore(series []dotaSeriesGame) (canon.EventScore, bool) {
	var score canon.EventScore
	seriesType := esports.BestOfOne
	var liveGame dotaSeriesGame
	for _, g := range series {
		switch {
		case g.Live:
			liveGame = g
		case g.RadiantWin == g.radiantIsHome:
			seriesType = g.SeriesType
			score.Home++
		default:
			seriesType = g.SeriesType
			score.Away++
		}
	}
	if !liveGame.Live && seriesType.IsDecided(score.Home, score.Away) {
		score.Period = canon.PeriodFinished
		return score, true
	}
	score.Period = mapPeriodPrefix + strconv.Itoa(score.Home+score.Away+1)
	if liveGame.Live {
		homeKills, awayKills := liveGame.RadiantKills, liveGame.DireKills
		if !liveGame.radiantIsHome {
			homeKills, awayKills = awayKills, homeKills
		}
		score.Detail = strconv.Itoa(homeKills) + "-" + strconv.Itoa(awayKills)
	}
	return score, false
}
