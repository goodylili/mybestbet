package ingest

import (
	"cmp"
	"math"
	"slices"
	"time"

	"mybestbet/internal/platform/config"
)

const (
	noTier               = -1
	hotTier              = 0
	hotShare             = 0.6
	maxStretchedInterval = 30 * time.Minute
	minFetchTimeout      = 5 * time.Second
	maxFetchTimeout      = 20 * time.Second
)

func sortedTiers(tiers []config.Tier) []config.Tier {
	sorted := slices.Clone(tiers)
	slices.SortFunc(sorted, func(a, b config.Tier) int { return cmp.Compare(a.Within, b.Within) })
	return sorted
}

func tierFor(tiers []config.Tier, untilKickoff time.Duration) int {
	if untilKickoff < 0 {
		return noTier
	}
	for i, t := range tiers {
		if untilKickoff <= t.Within {
			return i
		}
	}
	return noTier
}

func baseIntervals(tiers []config.Tier) []time.Duration {
	intervals := make([]time.Duration, len(tiers))
	for i, t := range tiers {
		intervals[i] = t.Interval
	}
	return intervals
}

func requestRate(eventsPerTier []int, intervals []time.Duration) float64 {
	total := 0.0
	for i, n := range eventsPerTier {
		total += tierRate(n, intervals[i])
	}
	return total
}

func tierRate(events int, interval time.Duration) float64 {
	if events == 0 || interval <= 0 {
		return 0
	}
	return float64(events) / interval.Seconds()
}

func stretchToCeiling(tiers []config.Tier, eventsPerTier []int, ceilingRPS float64) []time.Duration {
	intervals := baseIntervals(tiers)
	if ceilingRPS <= 0 {
		return intervals
	}
	if hotBudget := hotShare * ceilingRPS; len(tiers) > 0 && tierRate(eventsPerTier[hotTier], intervals[hotTier]) > hotBudget {
		intervals[hotTier] = stretchedInterval(eventsPerTier[hotTier], hotBudget, intervals[hotTier])
	}
	for i := len(tiers) - 1; i > hotTier && requestRate(eventsPerTier, intervals) > ceilingRPS; i-- {
		if eventsPerTier[i] == 0 {
			continue
		}
		spare := ceilingRPS - (requestRate(eventsPerTier, intervals) - tierRate(eventsPerTier[i], intervals[i]))
		intervals[i] = stretchedInterval(eventsPerTier[i], spare, intervals[i])
	}
	return intervals
}

func stretchedInterval(events int, spareRPS float64, current time.Duration) time.Duration {
	if spareRPS <= 0 {
		return maxStretchedInterval
	}
	needed := time.Duration(math.Ceil(float64(events) / spareRPS * float64(time.Second)))
	return min(max(needed, current), maxStretchedInterval)
}

func jittered(interval time.Duration, jitter, unitRandom float64) time.Duration {
	return time.Duration(float64(interval) * (1 + jitter*(2*unitRandom-1)))
}

func fetchTimeoutFor(interval time.Duration) time.Duration {
	return min(max(interval, minFetchTimeout), maxFetchTimeout)
}
