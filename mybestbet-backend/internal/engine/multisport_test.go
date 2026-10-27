package engine

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func defaultRuleDelta(source string, key canon.MarketKey, selections ...canon.Selection) canon.Delta {
	d := decimalDelta(source, key, testNow, selections...)
	d.Market.Rules = key.Code.DefaultRule()
	return d
}

func twoWayDelta(source string, key canon.MarketKey, first, second canon.Side, firstPrice, secondPrice float64) canon.Delta {
	return defaultRuleDelta(source, key, sel(first, "", firstPrice, testNow), sel(second, "", secondPrice, testNow))
}

func TestBoardMoneylineArbWithoutDerivedMarkets(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	moneyline := canon.MarketKey{EventID: "nba1", Code: canon.CodeMoneyline}
	b.apply(twoWayDelta("bet9ja", moneyline, canon.SideHome, canon.SideAway, 2.1, 1.7), testNow)
	b.apply(twoWayDelta("sportybet", moneyline, canon.SideHome, canon.SideAway, 1.8, 1.95), testNow)
	cs := b.apply(twoWayDelta("betking", moneyline, canon.SideHome, canon.SideAway, 1.75, 2.05), testNow)

	arbs := signalsOf(cs, canon.SignalArb, canon.SignalOpen)
	require.Len(t, arbs, 1)
	assert.InDelta(t, 1-(1/2.1+1/2.05), arbs[0].Margin, 1e-9)
	ms := b.markets[moneyline.String()]
	require.Len(t, ms.consensus, 2)
	for _, c := range ms.sources["sportybet"].cells {
		require.NotNil(t, c.Fair)
	}
	for _, other := range b.byEvent["nba1"] {
		assert.Equal(t, canon.CodeMoneyline, other.key.Code)
	}
}

func TestBoardNeverMixesPeriods(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	fullGame := canon.MarketKey{EventID: "nba1", Code: canon.CodeSpread, Line: canon.Line(-3.5)}
	firstQuarter := fullGame
	firstQuarter.Period = canon.PeriodQuarter1
	firstHalf := fullGame
	firstHalf.Period = canon.PeriodFirstHalf
	b.apply(twoWayDelta("bet9ja", fullGame, canon.SideHome, canon.SideAway, 2.1, 1.7), testNow)
	b.apply(twoWayDelta("betking", firstQuarter, canon.SideHome, canon.SideAway, 1.7, 2.1), testNow)
	cs := b.apply(twoWayDelta("sportybet", firstHalf, canon.SideHome, canon.SideAway, 1.7, 2.1), testNow)

	assert.Empty(t, signalsOf(cs, canon.SignalArb, canon.SignalOpen))
	assert.Len(t, b.markets, 3)
	for _, ms := range b.markets {
		assert.Len(t, ms.sources, 1, ms.keyString)
	}
}

func TestBoardKeepsQualifyApartFromMatchResult(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	qualify := canon.MarketKey{EventID: "cup1", Code: canon.CodeQualify}
	result := canon.MarketKey{EventID: "cup1", Code: canon.Code1X2}
	b.apply(twoWayDelta("bet9ja", qualify, canon.SideHome, canon.SideAway, 2.1, 1.7), testNow)
	b.apply(defaultRuleDelta("betking", result, sel(canon.SideHome, "", 1.7, testNow), sel(canon.SideDraw, "", 3.6, testNow), sel(canon.SideAway, "", 2.1, testNow)), testNow)
	regulationQualify := twoWayDelta("sportybet", qualify, canon.SideHome, canon.SideAway, 1.7, 2.1)
	regulationQualify.Market.Rules = canon.StandardRule
	cs := b.apply(regulationQualify, testNow)

	assert.Empty(t, signalsOf(cs, canon.SignalArb, canon.SignalOpen))
	assert.Len(t, b.markets[qualify.String()].sources, 2)
	drawNoBet := b.markets[canon.MarketKey{EventID: "cup1", Code: canon.CodeDNB}.String()]
	require.NotNil(t, drawNoBet)
	assert.Contains(t, drawNoBet.sources, "betking")
	assert.NotContains(t, drawNoBet.sources, "bet9ja")

	agreeing := twoWayDelta("sportybet", qualify, canon.SideHome, canon.SideAway, 1.7, 2.1)
	opened := signalsOf(b.apply(agreeing, testNow), canon.SignalArb, canon.SignalOpen)
	require.Len(t, opened, 1)
	for _, leg := range opened[0].Legs {
		assert.NotEqual(t, "betking", leg.Source)
	}
}

func TestBoardDropsSelectionsOutsideTheMarketShape(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	moneyline := canon.MarketKey{EventID: "nba1", Code: canon.CodeMoneyline}
	before := misshapenDropped(t, "threewaybook", canon.CodeMoneyline)
	cs := b.apply(defaultRuleDelta("threewaybook", moneyline, sel(canon.SideHome, "", 2.4, testNow), sel(canon.SideDraw, "", 12, testNow), sel(canon.SideAway, "", 1.9, testNow)), testNow)

	cells := cellsFrom(cs, moneyline.String(), "threewaybook")
	require.Len(t, cells, 2)
	for _, c := range cells {
		assert.NotEqual(t, canon.SideDraw, c.Side)
	}
	assert.InDelta(t, 1, misshapenDropped(t, "threewaybook", canon.CodeMoneyline)-before, 1e-9)
}

func misshapenDropped(t *testing.T, source string, code canon.MarketCode) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, misshapenCellsDropped.WithLabelValues(source, string(code)).Write(&m))
	return m.GetCounter().GetValue()
}

func TestBoardFlagsLineGapsOnPropsOfEverySport(t *testing.T) {
	t.Parallel()
	for _, code := range []canon.MarketCode{canon.PropPoints, canon.PropPassYards, canon.PropAces, canon.PropMapKills} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			b := newTestBoard()
			at := func(line float64) canon.MarketKey {
				return canon.MarketKey{EventID: "ev1", Code: code, PlayerID: "p1", Line: canon.Line(line)}
			}
			b.apply(twoWayDelta("bet9ja", at(20.5), canon.SideOver, canon.SideUnder, 1.9, 1.9), testNow)
			b.apply(twoWayDelta("bet9ja", at(22.5), canon.SideOver, canon.SideUnder, 2.6, 1.5), testNow)
			lineOnly := canon.Price{Form: canon.FormLineOnly, ObservedAt: testNow}
			pick := canon.Delta{Source: "prizepicks", Market: canon.Market{Key: at(22.5), Rules: code.DefaultRule(), Selections: []canon.Selection{{Side: canon.SideOver, Price: lineOnly}, {Side: canon.SideUnder, Price: lineOnly}}}}
			gaps := signalsOf(b.apply(pick, testNow), canon.SignalLine, canon.SignalOpen)
			require.Len(t, gaps, 1)
			assert.InDelta(t, 2.0, gaps[0].Margin, 1e-9)
		})
	}
}
