package stats

import (
	"slices"

	"mybestbet/pkg/canon"
)

const maxHistoryMatches = 200

type History struct {
	PlayerID    string
	newestFirst []MatchStat
}

func NewHistory(playerID string, newestFirst []MatchStat) History {
	return History{PlayerID: playerID, newestFirst: newestFirst}
}

func (h History) HitRates(code canon.MarketCode, line float64, lastN []int) (HitRates, error) {
	rule, effectiveLine, err := ruleFor(code, line)
	if err != nil {
		return HitRates{}, err
	}
	if len(lastN) == 0 {
		lastN = DefaultWindows
	}
	matches := h.newest(maxWindow(lastN), playedAtLeast(QualifyingMinutes(code)))
	rates := computeHitRates(matches, rule, effectiveLine, lastN)
	rates.PlayerID, rates.Code = h.PlayerID, code
	return rates, nil
}

func (h History) VsOpponent(opponentTeamID string, code canon.MarketCode, line float64) (HitRate, []MatchStat, error) {
	rule, effectiveLine, err := ruleFor(code, line)
	if err != nil {
		return HitRate{}, nil, err
	}
	minutes := playedAtLeast(QualifyingMinutes(code))
	matches := h.newest(vsOpponentMatches, func(m MatchStat) bool { return m.OpponentTeamID == opponentTeamID && minutes(m) })
	return hitRateOver(matches, rule.read, effectiveLine, len(matches)), matches, nil
}

func (h History) MinutesTrend() []MinutesPoint {
	matches := h.newest(minutesTrendMatches, nil)
	points := make([]MinutesPoint, 0, len(matches))
	for _, m := range slices.Backward(matches) {
		points = append(points, MinutesPoint{EventID: m.EventID, PlayedAt: m.PlayedAt, Minutes: m.Minutes, Started: m.Started})
	}
	return points
}

func (h History) RatingDistribution() RatingDistribution {
	matches := h.newest(ratingDistributionMatches, func(m MatchStat) bool { return m.Rating != nil })
	ratings := make([]float64, len(matches))
	for i, m := range matches {
		ratings[i] = *m.Rating
	}
	return ratingDistribution(ratings)
}

func (h History) newest(limit int, keep func(MatchStat) bool) []MatchStat {
	var matches []MatchStat
	for _, m := range h.newestFirst {
		if len(matches) == limit {
			break
		}
		if keep == nil || keep(m) {
			matches = append(matches, m)
		}
	}
	return matches
}

func playedAtLeast(minutes int) func(MatchStat) bool {
	return func(m MatchStat) bool { return m.Minutes >= minutes }
}
