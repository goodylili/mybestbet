package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestPropCellDropsWhatTheClientDerives(t *testing.T) {
	t.Parallel()
	observed := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	decimal := propCell(canon.BoardCell{MarketKey: "k", Source: "betking", Side: canon.SideOver, Price: canon.NewPrice(2, canon.FormDecimal, observed), DeepLink: "https://book.test"})
	assert.Equal(t, PropCell{Raw: 2, DeepLink: "https://book.test"}, decimal)

	share := propCell(canon.BoardCell{Source: "kalshi", Price: canon.NewPrice(0.4, canon.FormShare, observed), Derived: true})
	assert.Equal(t, PropCell{Raw: 0.4, Form: canon.FormShare, Derived: true}, share)

	devigged := canon.NewPrice(2, canon.FormDecimal, observed)
	devigged.Prob = 0.48
	cell := propCell(canon.BoardCell{Price: devigged})
	if assert.NotNil(t, cell.Prob) {
		assert.InDelta(t, 0.48, *cell.Prob, 0)
	}
}

func TestPropSelectionsKeepBestAndConsensus(t *testing.T) {
	t.Parallel()
	observed := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	cells := []canon.BoardCell{
		{Source: "betking", Side: canon.SideYes, Label: "Yes", Price: canon.NewPrice(2.1, canon.FormDecimal, observed)},
		{Source: "bet9ja", Side: canon.SideYes, Label: "Yes", Price: canon.NewPrice(2.3, canon.FormDecimal, observed)},
		{Source: consensusSource, Side: canon.SideYes, Label: "Yes", Price: canon.NewPrice(2.2, canon.FormDecimal, observed)},
	}
	got := propSelections(selectionRows(cells))
	assert.Len(t, got, 1)
	assert.Equal(t, "bet9ja", got[0].BestSource)
	assert.Equal(t, map[string]PropCell{"betking": {Raw: 2.1}, "bet9ja": {Raw: 2.3}}, got[0].Cells)
	assert.Equal(t, &PropCell{Raw: 2.2}, got[0].Consensus)
}
