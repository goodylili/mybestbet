package stats

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func ptr(f float64) *float64 { return &f }

func newestFirstMatches() []MatchStat {
	return []MatchStat{
		{IsHome: true, Shots: 3, Goals: 1, Rating: ptr(7.8)},
		{IsHome: false, Shots: 1, Assists: 1, Rating: ptr(6.4)},
		{IsHome: true, Shots: 2, Yellow: 1},
		{IsHome: false, Shots: 4, Goals: 2, Rating: ptr(8.1)},
		{IsHome: true, Shots: 0},
		{IsHome: false, Shots: 5, Rating: ptr(7.1)},
	}
}

func TestComputeHitRatesWindowsAndSplits(t *testing.T) {
	t.Parallel()
	rule, line, err := ruleFor(canon.PropShots, 1.5)
	assert.NoError(t, err)

	rates := computeHitRates(newestFirstMatches(), rule, line, []int{3, 10})

	assert.Equal(t, canon.PropShots, rates.StatCode)
	assert.Equal(t, []HitRate{
		{Window: 3, Matches: 3, Hits: 2, Rate: 2.0 / 3},
		{Window: 10, Matches: 6, Hits: 4, Rate: 4.0 / 6},
	}, rates.Windows)
	assert.Equal(t, HitRate{Window: 3, Matches: 3, Hits: 2, Rate: 2.0 / 3}, rates.Home)
	assert.Equal(t, HitRate{Window: 3, Matches: 3, Hits: 2, Rate: 2.0 / 3}, rates.Away)
}

func TestYesNoPropsUseHalfGoalLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		code         canon.MarketCode
		expectedStat canon.MarketCode
		expectedHits int
	}{
		{"anytime scorer maps to goals over half", canon.PropAnytime, canon.PropGoals, 2},
		{"goal or assist counts both", canon.PropGA, canon.PropGA, 3},
		{"to be booked counts cards", canon.PropCard, canon.PropCard, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rule, line, err := ruleFor(tt.code, 7)
			assert.NoError(t, err)
			assert.InDelta(t, 0.5, line, 0)
			rates := computeHitRates(newestFirstMatches(), rule, line, []int{20})
			assert.Equal(t, tt.expectedStat, rates.StatCode)
			assert.Equal(t, tt.expectedHits, rates.Windows[0].Hits)
		})
	}
}

func TestRatingHitRateSkipsUnratedMatches(t *testing.T) {
	t.Parallel()
	rule, line, err := ruleFor(canon.PropRating, 7.0)
	assert.NoError(t, err)
	rates := computeHitRates(newestFirstMatches(), rule, line, []int{20})
	assert.Equal(t, HitRate{Window: 20, Matches: 4, Hits: 3, Rate: 0.75}, rates.Windows[0])
}

func TestRuleForRejectsMatchMarkets(t *testing.T) {
	t.Parallel()
	_, _, err := ruleFor(canon.Code1X2, 0)
	assert.True(t, errors.Is(err, ErrUnsupportedCode))
}

func TestRatingDistribution(t *testing.T) {
	t.Parallel()
	d := ratingDistribution([]float64{7.8, 6.4, 8.1, 5.9})
	assert.InDelta(t, 7.05, d.Mean, 1e-9)
	assert.InDelta(t, 5.9, d.Min, 0)
	assert.InDelta(t, 8.1, d.Max, 0)
	counts := make([]int, len(d.Buckets))
	for i, b := range d.Buckets {
		counts[i] = b.Count
	}
	assert.Equal(t, []int{1, 1, 0, 0, 1, 1}, counts)
	assert.Equal(t, RatingBucket{From: 8, To: 10, Count: 1}, d.Buckets[5])

	empty := ratingDistribution(nil)
	assert.Len(t, empty.Buckets, 6)
	assert.Zero(t, empty.Mean)
}

func TestNextDailyRun(t *testing.T) {
	t.Parallel()
	before := time.Date(2026, 10, 23, 3, 59, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 10, 23, 4, 0, 0, 0, time.UTC), nextDailyRun(before, 4))
	at := time.Date(2026, 10, 23, 4, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 10, 24, 4, 0, 0, 0, time.UTC), nextDailyRun(at, 4))
}

func TestLineupStatusOf(t *testing.T) {
	t.Parallel()
	assert.Equal(t, LineupStarter, lineupStatusOf(false, false))
	assert.Equal(t, LineupBench, lineupStatusOf(true, false))
	assert.Equal(t, LineupOut, lineupStatusOf(false, true))
}
