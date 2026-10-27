package signal

import (
	"time"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

func FindValue(key canon.MarketKey, quotes []Quote, fair map[Outcome]float64, p config.Pricing, now time.Time) []canon.Signal {
	candidates := filterQuotes(quotes, func(q Quote) bool {
		fairProb, ok := fair[q.Outcome]
		return ok && isValueCandidate(q, fairProb, p)
	})
	var found []canon.Signal
	for _, q := range BestPerFeed(candidates) {
		fairProb := fair[q.Outcome]
		ev := fairProb*q.EffectiveDecimal() - 1
		if ev < p.EVMin {
			continue
		}
		s := newSignal(canon.SignalEV, key, []canon.Leg{q.leg(0)}, now, q.Source, q.outcomeTag())
		s.EV = ev
		s.FairProb = fairProb
		found = append(found, s)
	}
	return found
}

func isValueCandidate(q Quote, fairProb float64, p config.Pricing) bool {
	return fairProb > 0 && q.isBettable(p.MinTouchLiquidity) && p.EVProbRange.Contains(fairProb) && p.EVProbRange.Contains(q.Price.Prob)
}
