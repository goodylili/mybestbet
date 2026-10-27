package signal

import (
	"maps"
	"slices"
	"strings"
	"time"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/pricing"
	"mybestbet/pkg/canon"
)

const StakeBankroll = 100

const minArbOutcomes = 2

func FindArbs(key canon.MarketKey, quotes []Quote, p config.Pricing, now time.Time) []canon.Signal {
	bettable := filterQuotes(quotes, func(q Quote) bool { return q.isBettable(p.MinTouchLiquidity) })
	if len(bettable) < minArbOutcomes {
		return nil
	}
	outcomes, ok := completeOutcomeSet(quotes)
	if !ok || !coversShape(key.Code, outcomes) {
		return nil
	}
	var found []canon.Signal
	for _, group := range groupBySettleRule(bettable) {
		if s, ok := arbWithinRuleGroup(key, outcomes, withinLegSkew(group, p.MaxLegSkew), p.ArbMinMargin, now); ok {
			found = append(found, s)
		}
	}
	return found
}

func groupBySettleRule(quotes []Quote) [][]Quote {
	if len(quotes) == 0 {
		return nil
	}
	if allSettleLikeFirst(quotes) {
		return [][]Quote{quotes}
	}
	var groups [][]Quote
	for _, q := range quotes {
		i := slices.IndexFunc(groups, func(g []Quote) bool { return g[0].Rules.SettlesLike(q.Rules) })
		if i < 0 {
			groups = append(groups, nil)
			i = len(groups) - 1
		}
		groups[i] = append(groups[i], q)
	}
	return groups
}

func allSettleLikeFirst(quotes []Quote) bool {
	return !slices.ContainsFunc(quotes, func(q Quote) bool { return !q.Rules.SettlesLike(quotes[0].Rules) })
}

func withinLegSkew(quotes []Quote, maxSkew time.Duration) []Quote {
	var newest time.Time
	for _, q := range quotes {
		if q.Price.ObservedAt.After(newest) {
			newest = q.Price.ObservedAt
		}
	}
	return filterQuotes(quotes, func(q Quote) bool { return newest.Sub(q.Price.ObservedAt) <= maxSkew })
}

func arbWithinRuleGroup(key canon.MarketKey, outcomes []Outcome, quotes []Quote, minMargin float64, now time.Time) (canon.Signal, bool) {
	best := bestQuotePerOutcome(quotes)
	legs := make([]Quote, len(outcomes))
	var overround float64
	for i, o := range outcomes {
		leg, ok := best[o]
		if !ok {
			return canon.Signal{}, false
		}
		legs[i] = leg
		overround += 1 / leg.EffectiveDecimal()
	}
	margin := arbMargin(key, legs, overround)
	if margin < minMargin {
		return canon.Signal{}, false
	}
	signalLegs := make([]canon.Leg, len(legs))
	routing := make([]string, len(legs))
	for i, q := range legs {
		signalLegs[i] = q.leg(StakeBankroll / (q.EffectiveDecimal() * overround))
		routing[i] = q.outcomeTag() + "=" + q.Source
	}
	s := newSignal(canon.SignalArb, key, signalLegs, now, strings.Join(routing, ","))
	s.Margin = margin
	return s, true
}

type sourceOutcome struct {
	source string
	Outcome
}

func completeOutcomeSet(quotes []Quote) ([]Outcome, bool) {
	union := map[Outcome]bool{}
	offered := make(map[sourceOutcome]bool, len(quotes))
	outcomesPerSource := map[string]int{}
	for _, q := range quotes {
		union[q.Outcome] = true
		key := sourceOutcome{source: q.Source, Outcome: q.Outcome}
		if !offered[key] {
			offered[key] = true
			outcomesPerSource[q.Source]++
		}
	}
	if len(union) < minArbOutcomes {
		return nil, false
	}
	for _, n := range outcomesPerSource {
		if n == len(union) {
			return slices.SortedFunc(maps.Keys(union), compareOutcomes), true
		}
	}
	return nil, false
}

var threeWaySides = []canon.Side{canon.SideHome, canon.SideDraw, canon.SideAway}

func coversShape(code canon.MarketCode, outcomes []Outcome) bool {
	spec, ok := canon.SpecOf(code)
	if !ok || spec.Shape != canon.ShapeThreeWay {
		return true
	}
	return !slices.ContainsFunc(threeWaySides, func(side canon.Side) bool {
		return !slices.ContainsFunc(outcomes, func(o Outcome) bool { return o.Side == side })
	})
}

func bestQuotePerOutcome(quotes []Quote) map[Outcome]Quote {
	best := map[Outcome]Quote{}
	for _, q := range quotes {
		current, seen := best[q.Outcome]
		if !seen || isBetterLeg(q, current) {
			best[q.Outcome] = q
		}
	}
	return best
}

func isBetterLeg(candidate, current Quote) bool {
	cd, cur := candidate.EffectiveDecimal(), current.EffectiveDecimal()
	if cd != cur {
		return cd > cur
	}
	return candidate.Source < current.Source
}

func arbMargin(key canon.MarketKey, legs []Quote, overround float64) float64 {
	if key.Line == nil || len(legs) != 2 || !pricing.SettlesAgainstLine(key.Code) {
		return 1 - overround
	}
	first, second := legs[0], legs[1]
	if second.Side == canon.SideHome || second.Side == canon.SideOver {
		first, second = second, first
	}
	offset, ok := pricing.PushOffset(*key.Line, first.Side)
	if !ok {
		return 1 - overround
	}
	worst := pricing.WorstPushAwareReturn(offset,
		pricing.TwoWayStake{Decimal: first.EffectiveDecimal(), Stake: StakeBankroll / (first.EffectiveDecimal() * overround)},
		pricing.TwoWayStake{Decimal: second.EffectiveDecimal(), Stake: StakeBankroll / (second.EffectiveDecimal() * overround)},
	)
	return 1 - StakeBankroll/worst
}
