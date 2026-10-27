package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/canon"
)

func TestEveryPlayerPropHasAStatRule(t *testing.T) {
	t.Parallel()
	for _, sport := range canon.Sports {
		for _, code := range canon.MarketsFor(sport, canon.KindPlayer) {
			_, _, err := ruleFor(code, 1.5)
			assert.NoError(t, err, "%s %s", sport, code)
		}
	}
}

func TestSportHitRates(t *testing.T) {
	t.Parallel()
	matches := []MatchStat{
		{IsHome: true, Stats: statkey.Values{statkey.Points: 20, statkey.Rebounds: 8, statkey.Assists: 5, statkey.RushTDs: 0, statkey.RecTDs: 1, statkey.Kills: 30, statkey.Maps: 2}},
		{IsHome: false, Stats: statkey.Values{statkey.Points: 10, statkey.Rebounds: 2, statkey.Assists: 1, statkey.RushTDs: 0, statkey.RecTDs: 0, statkey.Kills: 40, statkey.Maps: 3}},
		{IsHome: true, Stats: statkey.Values{}},
	}
	tests := []struct {
		code     canon.MarketCode
		line     float64
		expected HitRate
	}{
		{canon.PropPtsRebAst, 30.5, HitRate{Window: 5, Matches: 2, Hits: 1, Rate: 0.5}},
		{canon.PropPoints, 9.5, HitRate{Window: 5, Matches: 2, Hits: 2, Rate: 1}},
		{canon.PropAnytimeTD, 0, HitRate{Window: 5, Matches: 2, Hits: 1, Rate: 0.5}},
		{canon.PropMapKills, 14.5, HitRate{Window: 5, Matches: 2, Hits: 1, Rate: 0.5}},
		{canon.PropAces, 4.5, HitRate{Window: 5}},
	}
	for _, tt := range tests {
		rule, line, err := ruleFor(tt.code, tt.line)
		assert.NoError(t, err)
		rates := computeHitRates(matches, rule, line, []int{5})
		assert.Equal(t, tt.expected, rates.Windows[0], tt.code)
	}
}

func TestQualifyingMinutes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 45, QualifyingMinutes(canon.PropShots))
	assert.Equal(t, 10, QualifyingMinutes(canon.PropPoints))
	assert.Equal(t, 0, QualifyingMinutes(canon.PropPassYards))
	assert.Equal(t, 0, QualifyingMinutes(canon.PropMapKills))
}
