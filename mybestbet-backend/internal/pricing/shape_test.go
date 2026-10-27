package pricing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

func TestFitsShape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		code     canon.MarketCode
		side     canon.Side
		expected bool
	}{
		{name: "match result draw", code: canon.Code1X2, side: canon.SideDraw, expected: true},
		{name: "moneyline home", code: canon.CodeMoneyline, side: canon.SideHome, expected: true},
		{name: "moneyline has no draw", code: canon.CodeMoneyline, side: canon.SideDraw},
		{name: "qualify has no draw", code: canon.CodeQualify, side: canon.SideDraw},
		{name: "spread away", code: canon.CodeSpread, side: canon.SideAway, expected: true},
		{name: "spread is not a total", code: canon.CodeSpread, side: canon.SideOver},
		{name: "set handicap home", code: canon.CodeSetHcp, side: canon.SideHome, expected: true},
		{name: "map handicap draw", code: canon.CodeMapHcp, side: canon.SideDraw},
		{name: "total under", code: canon.CodeTotal, side: canon.SideUnder, expected: true},
		{name: "total games home", code: canon.CodeTotalGames, side: canon.SideHome},
		{name: "total maps over", code: canon.CodeTotalMaps, side: canon.SideOver, expected: true},
		{name: "points prop over", code: canon.PropPoints, side: canon.SideOver, expected: true},
		{name: "anytime touchdown yes", code: canon.PropAnytimeTD, side: canon.SideYes, expected: true},
		{name: "anytime touchdown over", code: canon.PropAnytimeTD, side: canon.SideOver},
		{name: "double chance label without side", code: canon.CodeDC, side: "", expected: true},
		{name: "team stat bucket", code: canon.TeamShots, side: "", expected: true},
		{name: "team goals over", code: canon.TeamGoals, side: canon.SideOver, expected: true},
		{name: "team goals has no home side", code: canon.TeamGoals, side: canon.SideHome},
		{name: "goalkeeper saves under", code: canon.PropSaves, side: canon.SideUnder, expected: true},
		{name: "player passes yes", code: canon.PropPasses, side: canon.SideYes},
		{name: "unknown code passes", code: "X_UNKNOWN", side: canon.SideDraw, expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, FitsShape(tt.code, tt.side))
		})
	}
}

func TestSettlesAgainstLine(t *testing.T) {
	t.Parallel()
	for _, code := range []canon.MarketCode{canon.CodeAH, canon.CodeOU, canon.CodeSpread, canon.CodeTotal, canon.CodeSetHcp, canon.CodeTotalGames, canon.CodeMapHcp, canon.CodeTotalMaps, canon.PropPoints, canon.PropAces, canon.TeamShots, canon.TeamGoals, canon.PropSaves, canon.PropPasses} {
		assert.True(t, SettlesAgainstLine(code), code)
	}
	for _, code := range []canon.MarketCode{canon.Code1X2, canon.CodeMoneyline, canon.CodeQualify, canon.CodeBTTS, canon.PropAnytimeTD, canon.CodeDC} {
		assert.False(t, SettlesAgainstLine(code), code)
	}
}

func TestDevigMarketTwoWayPerCode(t *testing.T) {
	t.Parallel()
	implied := []float64{1 / 1.3, 1 / 3.6}
	powerByDefault := config.Pricing{}
	multiplicativeForMoneyline := config.Pricing{DefaultDevig: "power", DevigMethod: map[canon.MarketCode]string{canon.CodeMoneyline: "multiplicative"}}

	power, ok := DevigMarket(powerByDefault, canon.CodeMoneyline, implied)
	require.True(t, ok)
	multiplicative, ok := DevigMarket(multiplicativeForMoneyline, canon.CodeMoneyline, implied)
	require.True(t, ok)
	spread, ok := DevigMarket(multiplicativeForMoneyline, canon.CodeSpread, implied)
	require.True(t, ok)

	assert.InDelta(t, 1, power[0]+power[1], tolerance)
	assert.InDelta(t, implied[0]/(implied[0]+implied[1]), multiplicative[0], tolerance)
	assert.Greater(t, power[0], multiplicative[0])
	assert.Equal(t, power, spread)
}

func TestWorstPushAwareReturnWholeTotalsAcrossSports(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     float64
		expected float64
	}{
		{name: "basketball total 220", line: 220, expected: 105},
		{name: "tennis games 22", line: 22, expected: 105},
		{name: "total 44.25 quarter", line: 44.25, expected: 102.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			offset, ok := PushOffset(tt.line, canon.SideOver)
			require.True(t, ok)
			stake := TwoWayStake{Decimal: 2.1, Stake: 50}
			assert.InDelta(t, tt.expected, WorstPushAwareReturn(offset, stake, stake), 1e-6)
		})
	}
}
