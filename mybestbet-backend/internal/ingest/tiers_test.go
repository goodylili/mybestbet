package ingest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
)

var testTiers = []config.Tier{
	{Name: "hot", Within: 2 * time.Hour, Interval: 5 * time.Second, Jitter: 0.2},
	{Name: "warm", Within: 24 * time.Hour, Interval: 30 * time.Second, Jitter: 0.1},
	{Name: "cold", Within: 168 * time.Hour, Interval: 3 * time.Minute, Jitter: 0.1},
}

func TestTierFor(t *testing.T) {
	tests := []struct {
		name         string
		untilKickoff time.Duration
		expected     int
	}{
		{name: "already kicked off", untilKickoff: -time.Minute, expected: noTier},
		{name: "kickoff now is hot", untilKickoff: 0, expected: 0},
		{name: "two hours is hot", untilKickoff: 2 * time.Hour, expected: 0},
		{name: "just past two hours is warm", untilKickoff: 2*time.Hour + time.Second, expected: 1},
		{name: "three days is cold", untilKickoff: 72 * time.Hour, expected: 2},
		{name: "beyond a week is unscheduled", untilKickoff: 169 * time.Hour, expected: noTier},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tierFor(testTiers, tt.untilKickoff))
		})
	}
}

func TestSortedTiersOrdersByWindow(t *testing.T) {
	shuffled := []config.Tier{testTiers[2], testTiers[0], testTiers[1]}
	assert.Equal(t, testTiers, sortedTiers(shuffled))
}

func TestStretchToCeiling(t *testing.T) {
	tests := []struct {
		name          string
		eventsPerTier []int
		ceilingRPS    float64
		expected      []time.Duration
	}{
		{
			name:          "under ceiling keeps base intervals",
			eventsPerTier: []int{10, 30, 100},
			ceilingRPS:    8,
			expected:      []time.Duration{5 * time.Second, 30 * time.Second, 3 * time.Minute},
		},
		{
			name:          "no ceiling configured",
			eventsPerTier: []int{100, 300, 1000},
			ceilingRPS:    0,
			expected:      []time.Duration{5 * time.Second, 30 * time.Second, 3 * time.Minute},
		},
		{
			name:          "cold stretches first",
			eventsPerTier: []int{20, 30, 720},
			ceilingRPS:    8,
			expected:      []time.Duration{5 * time.Second, 30 * time.Second, 240 * time.Second},
		},
		{
			name:          "warm stretches once cold hits its cap",
			eventsPerTier: []int{20, 120, 3600},
			ceilingRPS:    8,
			expected:      []time.Duration{5 * time.Second, 60 * time.Second, maxStretchedInterval},
		},
		{
			name:          "hot keeps at most its share of the ceiling",
			eventsPerTier: []int{60, 60, 60},
			ceilingRPS:    8,
			expected:      []time.Duration{12500 * time.Millisecond, 30 * time.Second, 3 * time.Minute},
		},
		{
			name:          "hot within its share is never stretched",
			eventsPerTier: []int{24, 0, 0},
			ceilingRPS:    8,
			expected:      []time.Duration{5 * time.Second, 30 * time.Second, 3 * time.Minute},
		},
		{
			name:          "empty cold tier is skipped",
			eventsPerTier: []int{20, 150, 0},
			ceilingRPS:    8,
			expected:      []time.Duration{5 * time.Second, 37500 * time.Millisecond, 3 * time.Minute},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stretchToCeiling(testTiers, tt.eventsPerTier, tt.ceilingRPS)
			require.Len(t, got, len(tt.expected))
			for i := range got {
				assert.InDelta(t, tt.expected[i].Seconds(), got[i].Seconds(), 0.001, "tier %s", testTiers[i].Name)
			}
			if tt.ceilingRPS > 0 && tt.eventsPerTier[0] <= int(tt.ceilingRPS*testTiers[0].Interval.Seconds()) {
				assert.LessOrEqual(t, requestRate(tt.eventsPerTier, got), tt.ceilingRPS+0.001)
			}
		})
	}
}

func TestJitteredStaysWithinBand(t *testing.T) {
	tests := []struct {
		name       string
		unitRandom float64
		expected   time.Duration
	}{
		{name: "lowest draw", unitRandom: 0, expected: 4 * time.Second},
		{name: "middle draw", unitRandom: 0.5, expected: 5 * time.Second},
		{name: "highest draw", unitRandom: 1, expected: 6 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, jittered(5*time.Second, 0.2, tt.unitRandom))
		})
	}
}

func TestFetchTimeoutPerTier(t *testing.T) {
	assert.Equal(t, 5*time.Second, fetchTimeoutFor(testTiers[0].Interval))
	assert.Equal(t, 20*time.Second, fetchTimeoutFor(testTiers[1].Interval))
	assert.Equal(t, 20*time.Second, fetchTimeoutFor(testTiers[2].Interval))
}

func TestEventCeilingFollowsRequestCost(t *testing.T) {
	r := &runner{ceilingRPS: 8}
	assert.InDelta(t, 8, r.eventCeiling(), 1e-9)
	r.recordRequestCost(12, 4)
	assert.InDelta(t, 8.0/3, r.eventCeiling(), 1e-9)
	r.recordRequestCost(4, 4)
	assert.InDelta(t, 8/(3+0.2*(1-3)), r.eventCeiling(), 1e-9)
	r.recordRequestCost(0, 4)
	assert.InDelta(t, 8/(3+0.2*(1-3)), r.eventCeiling(), 1e-9)
}
