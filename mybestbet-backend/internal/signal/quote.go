package signal

import (
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

type Outcome struct {
	Side  canon.Side
	Label string
}

func (o Outcome) outcomeTag() string { return string(o.Side) + ":" + o.Label }

func compareOutcomes(a, b Outcome) int {
	if c := strings.Compare(string(a.Side), string(b.Side)); c != 0 {
		return c
	}
	return strings.Compare(a.Label, b.Label)
}

type Quote struct {
	Outcome
	Source      string
	Feed        string
	Price       canon.Price
	Rules       canon.SettleRule
	DeepLink    string
	Derived     bool
	Degraded    bool
	Stale       bool
	FeePerShare float64
}

func (q Quote) feed() string {
	if q.Feed == "" {
		return q.Source
	}
	return q.Feed
}

type feedOutcome struct {
	feed string
	Outcome
}

func BestPerFeed(quotes []Quote) []Quote {
	slotOf := make(map[feedOutcome]int, len(quotes))
	bestIndex := make([]int, 0, len(quotes))
	for i, q := range quotes {
		key := feedOutcome{feed: q.feed(), Outcome: q.Outcome}
		slot, seen := slotOf[key]
		if !seen {
			slotOf[key] = len(bestIndex)
			bestIndex = append(bestIndex, i)
			continue
		}
		if isBetterLeg(q, quotes[bestIndex[slot]]) {
			bestIndex[slot] = i
		}
	}
	if len(bestIndex) == len(quotes) {
		return quotes
	}
	best := make([]Quote, len(bestIndex))
	for slot, i := range bestIndex {
		best[slot] = quotes[i]
	}
	return best
}

func filterQuotes(quotes []Quote, keep func(Quote) bool) []Quote {
	kept := 0
	for _, q := range quotes {
		if keep(q) {
			kept++
		}
	}
	if kept == len(quotes) {
		return quotes
	}
	filtered := make([]Quote, 0, kept)
	for _, q := range quotes {
		if keep(q) {
			filtered = append(filtered, q)
		}
	}
	return filtered
}

func isPredictionMarketForm(form canon.PriceForm) bool {
	switch form {
	case canon.FormShare, canon.FormCents, canon.FormNaira:
		return true
	case canon.FormDecimal, canon.FormLineOnly:
		return false
	}
	return false
}

func (q Quote) EffectiveDecimal() float64 {
	cost := q.Price.Prob
	if isPredictionMarketForm(q.Price.Form) {
		cost += q.FeePerShare
	}
	if cost <= 0 {
		return 0
	}
	return 1 / cost
}

func (q Quote) isBettable(minTouchLiquidity float64) bool {
	if q.Derived || q.Degraded || q.Stale || q.Price.Suspended || q.Price.Form == canon.FormLineOnly || q.Price.Prob <= 0 {
		return false
	}
	return !q.hasThinTouch(minTouchLiquidity)
}

func (q Quote) hasThinTouch(minTouchLiquidity float64) bool {
	return isPredictionMarketForm(q.Price.Form) && q.Price.Liquidity != nil && *q.Price.Liquidity < minTouchLiquidity
}

func (q Quote) leg(stake float64) canon.Leg {
	return canon.Leg{
		Source:     q.Source,
		Side:       q.Side,
		Label:      q.Label,
		Price:      q.Price,
		Stake:      stake,
		DeepLink:   q.DeepLink,
		ObservedAt: q.Price.ObservedAt,
	}
}

func newSignal(kind canon.SignalType, key canon.MarketKey, legs []canon.Leg, now time.Time, dedupeParts ...string) canon.Signal {
	marketKey := key.String()
	for i := range legs {
		if legs[i].MarketKey == "" {
			legs[i].MarketKey, legs[i].Line = marketKey, key.Line
		}
	}
	return canon.Signal{
		DedupeKey:    dedupeKey(kind, marketKey, dedupeParts...),
		Type:         kind,
		Status:       canon.SignalOpen,
		EventID:      key.EventID,
		Market:       key,
		Legs:         legs,
		OldestLegAge: oldestLegAge(legs, now),
		DetectedAt:   now,
	}
}

func oldestLegAge(legs []canon.Leg, now time.Time) time.Duration {
	var oldest time.Duration
	for _, l := range legs {
		oldest = max(oldest, now.Sub(l.ObservedAt))
	}
	return oldest
}

func dedupeKey(kind canon.SignalType, marketKey string, parts ...string) string {
	return string(kind) + "|" + marketKey + "|" + strings.Join(parts, "|")
}
