package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/canon"
)

func TestMatchStatValuesKeyedByStatField(t *testing.T) {
	t.Parallel()
	rating := 7.4
	tests := []struct {
		name     string
		match    MatchStat
		expected map[string]float64
	}{
		{
			name:  "football mirrors columns",
			match: MatchStat{Sport: canon.SportFootball, Goals: 1, Assists: 2, Shots: 4, ShotsOnTarget: 3, Fouls: 1, Tackles: 5, Yellow: 1, Rating: &rating},
			expected: map[string]float64{
				"goals": 1, "assists": 2, "shots": 4, "shotsOnTarget": 3, "fouls": 1, "tackles": 5, "yellow": 1, "red": 0, "rating": 7.4,
			},
		},
		{
			name:     "basketball keeps stored values and ignores football columns",
			match:    MatchStat{Sport: canon.SportBasketball, Assists: 9, Stats: statkey.Values{statkey.Points: 21, statkey.Rebounds: 7, statkey.Assists: 4, statkey.Threes: 3}},
			expected: map[string]float64{"points": 21, "rebounds": 7, "assists": 4, "threes": 3},
		},
		{
			name:     "esports kills read per map like hit rates",
			match:    MatchStat{Sport: canon.SportEsports, Stats: statkey.Values{statkey.Kills: 42, statkey.Maps: 2}},
			expected: map[string]float64{"kills": 21, "maps": 2},
		},
		{
			name:     "tennis passes stored values through",
			match:    MatchStat{Sport: canon.SportTennis, Stats: statkey.Values{statkey.Aces: 11, statkey.DoubleFaults: 2, statkey.Sets: 3}},
			expected: map[string]float64{"aces": 11, "doubleFaults": 2, "sets": 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.match.StatValues())
		})
	}
}

func TestMatchStatValuesAgreeWithHitRateReadings(t *testing.T) {
	t.Parallel()
	rating := 6.9
	matches := map[canon.Sport]MatchStat{
		canon.SportFootball:         {Sport: canon.SportFootball, Goals: 2, Assists: 1, Shots: 5, ShotsOnTarget: 3, Fouls: 2, Tackles: 1, Rating: &rating},
		canon.SportBasketball:       {Sport: canon.SportBasketball, Stats: statkey.Values{statkey.Points: 30, statkey.Rebounds: 8, statkey.Assists: 6, statkey.Threes: 4}},
		canon.SportAmericanFootball: {Sport: canon.SportAmericanFootball, Stats: statkey.Values{statkey.PassYards: 280, statkey.RushYards: 12, statkey.RecYards: 0, statkey.Receptions: 0, statkey.PassTDs: 2}},
		canon.SportTennis:           {Sport: canon.SportTennis, Stats: statkey.Values{statkey.Aces: 9, statkey.DoubleFaults: 3}},
		canon.SportEsports:          {Sport: canon.SportEsports, Stats: statkey.Values{statkey.Kills: 51, statkey.Maps: 3}},
	}
	for code, rule := range statRules {
		spec, _ := canon.SpecOf(code)
		if spec.StatField == "" {
			continue
		}
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			match := matches[spec.Sports[0]]
			want, ok := rule.read(match)
			assert.True(t, ok)
			assert.InDelta(t, want, match.StatValues()[spec.StatField], 1e-9)
		})
	}
}
