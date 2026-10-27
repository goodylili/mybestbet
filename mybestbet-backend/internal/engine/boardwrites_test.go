package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestBoardSkipsRewritingUnchangedCells(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		after   time.Duration
		home    float64
		written bool
	}{
		{name: "same prices seconds later", after: 5 * time.Second, home: 2.1},
		{name: "changed price", after: 5 * time.Second, home: 2.2, written: true},
		{name: "same prices past the refresh interval", after: boardRefreshAfter, home: 2.1, written: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBoard()
			first := b.apply(matchResultDelta("betking", 2.1, 3.4, 3.6, testNow), testNow)
			require.Len(t, cellsFrom(first, matchResult.String(), "betking"), 3)

			later := testNow.Add(tt.after)
			cs := b.apply(matchResultDelta("betking", tt.home, 3.4, 3.6, later), later)
			assert.Equal(t, tt.written, len(cellsFrom(cs, matchResult.String(), "betking")) == 3)
			assert.Len(t, cs.rows, 3)
		})
	}
}

func TestBoardSkipsRewritingUnchangedDerivedCells(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	b.apply(matchResultDelta("betking", 2.1, 3.4, 3.6, testNow), testNow)
	later := testNow.Add(5 * time.Second)
	cs := b.apply(matchResultDelta("betking", 2.1, 3.4, 3.6, later), later)
	assert.Empty(t, cs.cells)
}

func TestMergeBoardWritesGroupsCellsByTheirEvent(t *testing.T) {
	t.Parallel()
	rescanned := changeSet{cells: []canon.BoardCell{
		{MarketKey: canon.MarketKey{EventID: "ev1", Code: canon.Code1X2}.String(), Source: consensusSource, Side: canon.SideHome},
		{MarketKey: canon.MarketKey{EventID: "ev2", Code: canon.Code1X2}.String(), Source: consensusSource, Side: canon.SideAway},
	}}
	writes := mergeBoardWrites([]changeSet{rescanned})
	require.Len(t, writes, 2)
	assert.Len(t, writes["ev1"].published, 1)
	assert.Len(t, writes["ev2"].published, 1)
	assert.NotContains(t, writes, "")
}
