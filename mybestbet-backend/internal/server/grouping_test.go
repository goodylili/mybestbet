package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func oneXTwoKey(eventID string) string {
	return canon.MarketKey{EventID: eventID, Code: canon.Code1X2}.String()
}

func decimalCell(marketKey, source string, side canon.Side, label string, odds float64) canon.BoardCell {
	return canon.BoardCell{MarketKey: marketKey, Source: source, Side: side, Label: label, Price: canon.NewPrice(odds, canon.FormDecimal, time.Time{})}
}

func TestSelectionRowsPicksBestBettableSource(t *testing.T) {
	t.Parallel()
	key := oneXTwoKey("e1")
	fair := 0.48
	suspended := decimalCell(key, "betking", canon.SideHome, "", 3.5)
	suspended.Price.Suspended = true
	derived := decimalCell(key, "bet9ja", canon.SideHome, "", 9)
	derived.Derived = true
	cells := []canon.BoardCell{
		decimalCell(key, "betano", canon.SideAway, "", 3.1),
		decimalCell(key, "sportybet", canon.SideHome, "", 2.05),
		decimalCell(key, "betway", canon.SideHome, "", 2.15),
		suspended,
		derived,
		{MarketKey: key, Source: consensusSource, Side: canon.SideHome, Fair: &fair},
		decimalCell(key, "betway", canon.SideDraw, "", 3.3),
	}
	rows := selectionRows(cells)
	require.Len(t, rows, 3)
	assert.Equal(t, []string{"home", "draw", "away"}, []string{rows[0].Side, rows[1].Side, rows[2].Side})
	home := rows[0]
	assert.Equal(t, "betway", home.BestSource)
	assert.Len(t, home.Cells, 4)
	require.NotNil(t, home.Consensus)
	assert.InDelta(t, 0.48, *home.Consensus.Fair, 1e-9)
	assert.NotContains(t, home.Cells, consensusSource)
}

func TestSelectionRowsOrdersDoubleChanceLabels(t *testing.T) {
	t.Parallel()
	key := canon.MarketKey{EventID: "e1", Code: canon.CodeDC}.String()
	rows := selectionRows([]canon.BoardCell{
		decimalCell(key, "a", "", "X2", 1.5),
		decimalCell(key, "a", "", "12", 1.3),
		decimalCell(key, "a", "", "1X", 1.2),
	})
	assert.Equal(t, []string{"1X", "12", "X2"}, []string{rows[0].Label, rows[1].Label, rows[2].Label})
}

func TestEventDetailGroupsMatchMarketsByCodeAndLine(t *testing.T) {
	t.Parallel()
	ou35 := canon.MarketKey{EventID: "e1", Code: canon.CodeOU, Line: canon.Line(3.5)}.String()
	ou25 := canon.MarketKey{EventID: "e1", Code: canon.CodeOU, Line: canon.Line(2.5)}.String()
	prop := canon.MarketKey{EventID: "e1", Code: canon.PropShots, Line: canon.Line(1.5), PlayerID: "p1"}.String()
	cells := []canon.BoardCell{
		decimalCell(ou35, "betano", canon.SideOver, "", 2.9),
		decimalCell(ou25, "betano", canon.SideOver, "", 1.9),
		decimalCell(ou25, "kalshi", canon.SideUnder, "", 2.0),
		decimalCell(oneXTwoKey("e1"), "bet9ja", canon.SideHome, "", 2.0),
		decimalCell(prop, "prizepicks", canon.SideOver, "", 1.8),
		{MarketKey: "garbage", Source: "x"},
	}
	detail := eventDetail(EventInfo{ID: "e1"}, cells)
	require.Equal(t, []canon.MarketCode{canon.Code1X2, canon.CodeOU}, tabCodes(detail.Markets))
	assert.Equal(t, canon.ShapeThreeWay, detail.Markets[0].Shape)
	overUnder := detail.Markets[1]
	assert.Equal(t, canon.ShapeOverUnder, overUnder.Shape)
	require.Len(t, overUnder.Periods, 1)
	boards := overUnder.Periods[0].Boards
	require.Len(t, boards, 2)
	assert.Equal(t, ou25, boards[0].KeyString)
	assert.Equal(t, ou35, boards[1].KeyString)
	assert.Equal(t, []string{"bet9ja", "betano", "kalshi"}, detail.Sources)
}

func TestEventDetailSplitsBasketballMarketsIntoPeriods(t *testing.T) {
	t.Parallel()
	spreadKey := func(line float64, period string) string {
		return canon.MarketKey{EventID: "e2", Code: canon.CodeSpread, Line: canon.Line(line), Period: period}.String()
	}
	moneyline := canon.MarketKey{EventID: "e2", Code: canon.CodeMoneyline}.String()
	cells := []canon.BoardCell{
		decimalCell(spreadKey(-1.5, canon.PeriodQuarter1), "betway", canon.SideHome, "", 1.9),
		decimalCell(spreadKey(-5.5, canon.PeriodFullTime), "betway", canon.SideHome, "", 1.91),
		decimalCell(spreadKey(-3.5, canon.PeriodFirstHalf), "betking", canon.SideAway, "", 1.88),
		decimalCell(spreadKey(-4.5, canon.PeriodFullTime), "betking", canon.SideAway, "", 1.95),
		decimalCell(moneyline, "betway", canon.SideAway, "", 2.4),
	}
	detail := eventDetail(EventInfo{ID: "e2", Sport: canon.SportBasketball}, cells)
	require.Equal(t, []canon.MarketCode{canon.CodeMoneyline, canon.CodeSpread}, tabCodes(detail.Markets))
	spread := detail.Markets[1]
	assert.Equal(t, canon.ShapeHandicap, spread.Shape)
	assert.Equal(t, canon.KindMatch, spread.Kind)
	periods := make([]string, len(spread.Periods))
	for i, p := range spread.Periods {
		periods[i] = p.Period
	}
	assert.Equal(t, []string{canon.PeriodFullTime, canon.PeriodFirstHalf, canon.PeriodQuarter1}, periods)
	require.Len(t, spread.Periods[0].Boards, 2)
	assert.Equal(t, spreadKey(-5.5, canon.PeriodFullTime), spread.Periods[0].Boards[0].KeyString)
}

func tabCodes(tabs []MarketTab) []canon.MarketCode {
	codes := make([]canon.MarketCode, len(tabs))
	for i, tab := range tabs {
		codes[i] = tab.Code
	}
	return codes
}

func TestBestOutcomes(t *testing.T) {
	t.Parallel()
	key := oneXTwoKey("e1")
	best := bestOutcomes(canon.MarketKey{EventID: "e1", Code: canon.Code1X2}, []canon.BoardCell{
		decimalCell(key, "a", canon.SideHome, "", 2.0),
		decimalCell(key, "b", canon.SideHome, "", 2.2),
		decimalCell(key, "a", canon.SideAway, "", 4.0),
	})
	assert.Equal(t, canon.Code1X2, best.Market)
	require.NotNil(t, best.Home)
	assert.Equal(t, "b", best.Home.Source)
	assert.Nil(t, best.Draw)
	require.NotNil(t, best.Away)
	assert.Equal(t, "a", best.Away.Source)
}

func TestEventDetailKeepsTeamMarketsApartByTeam(t *testing.T) {
	t.Parallel()
	teamShots := func(teamID string) string {
		return canon.MarketKey{EventID: "e3", Code: canon.TeamShots, Line: canon.Line(11.5), TeamID: teamID}.String()
	}
	cells := []canon.BoardCell{
		decimalCell(teamShots("t2"), "betano", canon.SideOver, "", 1.9),
		decimalCell(teamShots("t1"), "betano", canon.SideOver, "", 2.1),
	}
	detail := eventDetail(EventInfo{ID: "e3"}, cells)
	require.Len(t, detail.Markets, 1)
	boards := detail.Markets[0].Periods[0].Boards
	require.Len(t, boards, 2)
	assert.Equal(t, "t1", boards[0].Key.TeamID)
	assert.Equal(t, teamShots("t1"), boards[0].KeyString)
	assert.Equal(t, "t2", boards[1].Key.TeamID)
}

func TestBestOutcomesOverUnderEchoesLineAndPeriod(t *testing.T) {
	t.Parallel()
	marketKey := canon.MarketKey{EventID: "e1", Code: canon.CodeOU, Line: canon.Line(2.5), Period: canon.PeriodFirstHalf}
	key := marketKey.String()
	best := bestOutcomes(marketKey, []canon.BoardCell{
		decimalCell(key, "a", canon.SideOver, "", 1.9),
		decimalCell(key, "b", canon.SideOver, "", 2.05),
		decimalCell(key, "a", canon.SideUnder, "", 1.8),
	})
	assert.Equal(t, canon.CodeOU, best.Market)
	require.NotNil(t, best.Line)
	assert.InDelta(t, 2.5, *best.Line, 0)
	assert.Equal(t, canon.PeriodFirstHalf, best.Period)
	require.NotNil(t, best.Over)
	assert.Equal(t, "b", best.Over.Source)
	require.NotNil(t, best.Under)
	assert.Equal(t, "a", best.Under.Source)
	assert.Nil(t, best.Home)
	assert.Nil(t, best.Yes)
}
