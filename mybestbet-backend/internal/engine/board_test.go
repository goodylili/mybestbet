package engine

import (
	"fmt"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/signal"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const testMoveHold = 10 * time.Second

var (
	testNow     = time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC)
	matchResult = canon.MarketKey{EventID: "ev1", Code: canon.Code1X2}
)

func testConfig() config.Config {
	return config.Config{
		Pricing: config.Pricing{
			DefaultDevig:      "power",
			DevigMethod:       map[canon.MarketCode]string{canon.CodeOU: "multiplicative"},
			MinFairSources:    3,
			ArbMinMargin:      0.005,
			EVMin:             0.03,
			MaxLegSkew:        15 * time.Second,
			MinTouchLiquidity: 50,
			DeepBookLiquidity: 1000,
			DeepBookWeight:    3,
			MaxOverround:      0.15,
			EVProbRange:       config.ProbRange{Min: 0.05, Max: 0.95},
			MinMoveHold:       testMoveHold,
		},
		Feeds:    map[string]string{"22bet": "onexbet", "paripesa": "onexbet", "betwinner": "onexbet"},
		Telegram: config.Telegram{MoveWindow: 15 * time.Minute},
		Sources:  map[string]sources.Config{"bayse": {FeePerShare: 0}},
	}
}

func newTestBoard(degraded ...string) *board {
	isDegraded := map[string]bool{}
	for _, s := range degraded {
		isDegraded[s] = true
	}
	return newBoard(testConfig(), func(source string) bool { return isDegraded[source] })
}

func decimalDelta(source string, key canon.MarketKey, at time.Time, selections ...canon.Selection) canon.Delta {
	return canon.Delta{Source: source, Market: canon.Market{Key: key, Selections: selections, Rules: canon.StandardRule}, FetchedAt: at}
}

func sel(side canon.Side, label string, decimal float64, at time.Time) canon.Selection {
	return canon.Selection{Side: side, Label: label, Price: canon.NewPrice(decimal, canon.FormDecimal, at)}
}

func matchResultDelta(source string, home, draw, away float64, at time.Time) canon.Delta {
	d := decimalDelta(source, matchResult, at, sel(canon.SideHome, "", home, at), sel(canon.SideDraw, "", draw, at), sel(canon.SideAway, "", away, at))
	d.Market.SourceKey = "1x2"
	return d
}

func signalsOf(cs changeSet, kind canon.SignalType, status canon.SignalStatus) []canon.Signal {
	var out []canon.Signal
	for _, s := range cs.signals {
		if (kind == "" || s.Type == kind) && s.Status == status {
			out = append(out, s)
		}
	}
	return out
}

func cellsFrom(cs changeSet, marketKey, source string) []canon.BoardCell {
	var out []canon.BoardCell
	for _, c := range cs.cells {
		if c.MarketKey == marketKey && c.Source == source {
			out = append(out, c)
		}
	}
	return out
}

func TestBoardRescansStagedBatchOnce(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	b.stage(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), testNow)
	b.stage(matchResultDelta("betking", 2.1, 3.9, 3.3, testNow), testNow)
	staged := b.stage(matchResultDelta("sportybet", 2.0, 3.5, 4.4, testNow), testNow)
	require.Empty(t, staged.signals)

	var cs changeSet
	b.rescanStale(&cs, testNow)
	assert.Len(t, signalsOf(cs, canon.SignalArb, canon.SignalOpen), 1)
	assert.Empty(t, b.stale)

	var again changeSet
	b.rescanStale(&again, testNow)
	assert.Empty(t, again.signals)
}

func TestBoardOpensAndClosesArb(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	b.apply(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), testNow)
	b.apply(matchResultDelta("betking", 2.1, 3.9, 3.3, testNow), testNow)
	cs := b.apply(matchResultDelta("sportybet", 2.0, 3.5, 4.4, testNow), testNow)

	opened := signalsOf(cs, canon.SignalArb, canon.SignalOpen)
	require.Len(t, opened, 1)
	assert.NotEmpty(t, opened[0].ID)
	assert.Len(t, cellsFrom(cs, matchResult.String(), consensusSource), 3)
	for _, c := range cellsFrom(cs, matchResult.String(), "sportybet") {
		require.NotNil(t, c.Fair)
	}

	later := testNow.Add(time.Second)
	unchanged := b.apply(matchResultDelta("sportybet", 2.0, 3.5, 4.4, later), later)
	assert.Empty(t, signalsOf(unchanged, canon.SignalArb, canon.SignalOpen))

	gone := b.apply(matchResultDelta("sportybet", 2.0, 3.5, 3.0, later), later)
	closed := signalsOf(gone, canon.SignalArb, canon.SignalClosed)
	require.Len(t, closed, 1)
	assert.Equal(t, opened[0].ID, closed[0].ID)
	require.NotNil(t, closed[0].ClosedAt)
}

func TestBoardPricesAndArbsTeamGoalsPerTeam(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	home := canon.MarketKey{EventID: "ev1", Code: canon.TeamGoals, Line: canon.Line(1.5), TeamID: "home-team"}
	away := home
	away.TeamID = "away-team"
	quote := func(source string, key canon.MarketKey, over, under float64) changeSet {
		return b.apply(decimalDelta(source, key, testNow, sel(canon.SideOver, "", over, testNow), sel(canon.SideUnder, "", under, testNow)), testNow)
	}
	quote("bet9ja", home, 1.9, 1.9)
	quote("betking", home, 1.85, 1.95)
	quote("bet9ja", away, 1.5, 2.5)
	quote("betking", away, 1.55, 2.4)
	cs := quote("sportybet", home, 2.05, 1.75)

	assert.Len(t, cellsFrom(cs, home.String(), consensusSource), 2)
	for _, c := range cellsFrom(cs, home.String(), "sportybet") {
		require.NotNil(t, c.Fair)
	}
	assert.Empty(t, signalsOf(cs, canon.SignalArb, canon.SignalOpen))

	opened := signalsOf(quote("betway", home, 1.6, 2.3), canon.SignalArb, canon.SignalOpen)
	require.Len(t, opened, 1)
	assert.Equal(t, home, opened[0].Market)
	assert.Empty(t, signalsOf(quote("betway", away, 1.5, 2.45), canon.SignalArb, canon.SignalOpen))
}

func TestBoardConfirmedUnchangedPricesRejoinArbs(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	firstSeen := testNow.Add(-time.Minute)
	b.apply(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, firstSeen), firstSeen)
	b.apply(matchResultDelta("betking", 2.1, 3.9, 3.3, firstSeen), firstSeen)
	stale := b.apply(matchResultDelta("sportybet", 2.0, 3.5, 4.4, testNow), testNow)
	require.Empty(t, signalsOf(stale, canon.SignalArb, canon.SignalOpen))

	b.confirmUnchanged(canon.Freshness{Source: "bet9ja", EventID: matchResult.EventID, ObservedAt: testNow, SourceKeys: []string{"1x2"}}, testNow)
	cs := b.confirmUnchanged(canon.Freshness{Source: "betking", EventID: matchResult.EventID, ObservedAt: testNow, SourceKeys: []string{"1x2"}}, testNow)

	require.Len(t, signalsOf(cs, canon.SignalArb, canon.SignalOpen), 1)
	assert.Empty(t, cs.rows)
}

func TestBoardConfirmsOnlyTheMarketsASourceStillQuotes(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	firstSeen := testNow.Add(-time.Minute)
	spread := canon.MarketKey{EventID: matchResult.EventID, Code: canon.CodeSpread, Line: new(-1.5)}
	withdrawn := decimalDelta("betway", spread, firstSeen, sel(canon.SideHome, "", 1.9, firstSeen), sel(canon.SideAway, "", 1.97, firstSeen))
	withdrawn.Market.SourceKey = "hcp-1.5"
	b.apply(withdrawn, firstSeen)
	b.apply(matchResultDelta("betway", 2.3, 3.4, 3.2, firstSeen), firstSeen)
	offer := decimalDelta("onexbet", spread, testNow, sel(canon.SideHome, "", 7.0, testNow), sel(canon.SideAway, "", 1.1, testNow))
	offer.Market.SourceKey = "onexbet-hcp"
	b.apply(offer, testNow)
	b.confirmUnchanged(canon.Freshness{Source: "betway", EventID: matchResult.EventID, ObservedAt: testNow, SourceKeys: []string{"1x2", "hcp-1.5"}}, testNow)
	require.Len(t, b.openByScope[spread.String()], 1)

	later := testNow.Add(5 * time.Second)
	cs := b.confirmUnchanged(canon.Freshness{Source: "betway", EventID: matchResult.EventID, ObservedAt: later, SourceKeys: []string{"1x2"}, Complete: true}, later)

	suspended := cellsFrom(cs, spread.String(), "betway")
	require.Len(t, suspended, 2)
	for _, c := range suspended {
		assert.True(t, c.Price.Suspended)
		assert.Equal(t, testNow, c.Price.ObservedAt)
	}
	require.Len(t, cs.rows, 2)
	assert.Len(t, signalsOf(cs, canon.SignalArb, canon.SignalClosed), 1)
	assert.Empty(t, b.openByScope[spread.String()])
	for _, c := range b.markets[matchResult.String()].sources["betway"].cells {
		assert.Equal(t, later, c.Price.ObservedAt)
	}

	again := b.confirmUnchanged(canon.Freshness{Source: "betway", EventID: matchResult.EventID, ObservedAt: later, SourceKeys: []string{"1x2"}, Complete: true}, later)
	assert.Empty(t, cellsFrom(again, spread.String(), "betway"))
	assert.Empty(t, again.rows)
	assert.Empty(t, again.signals)

	partial := b.confirmUnchanged(canon.Freshness{Source: "onexbet", EventID: matchResult.EventID, ObservedAt: later, SourceKeys: []string{"other"}}, later)
	assert.Empty(t, cellsFrom(partial, spread.String(), "onexbet"))
	assert.False(t, b.markets[spread.String()].sources["onexbet"].cells[0].Price.Suspended)
}

func TestBoardLeavesRestoredMarketsUnconfirmed(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	restoredAt := testNow.Add(-time.Hour)
	restored := matchResultDelta("betway", 2.3, 3.4, 3.2, restoredAt)
	restored.Market.SourceKey = ""
	b.restore(restored, testNow)

	cs := b.confirmUnchanged(canon.Freshness{Source: "betway", EventID: matchResult.EventID, ObservedAt: testNow, SourceKeys: []string{"other"}, Complete: true}, testNow)

	assert.Empty(t, cellsFrom(cs, matchResult.String(), "betway"))
	for _, c := range b.markets[matchResult.String()].sources["betway"].cells {
		assert.False(t, c.Price.Suspended)
		assert.Equal(t, restoredAt, c.Price.ObservedAt)
	}
}

func TestBoardExcludesDegradedSourceFromArbs(t *testing.T) {
	t.Parallel()
	b := newTestBoard("sportybet")
	b.apply(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), testNow)
	b.apply(matchResultDelta("betking", 2.1, 3.9, 3.3, testNow), testNow)
	cs := b.apply(matchResultDelta("sportybet", 2.0, 3.5, 4.4, testNow), testNow)
	for _, s := range signalsOf(cs, canon.SignalArb, canon.SignalOpen) {
		for _, leg := range s.Legs {
			assert.NotEqual(t, "sportybet", leg.Source)
		}
	}
}

func TestBoardHistoryRowsOnlyWhenRawChanges(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	first := b.apply(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), testNow)
	require.Len(t, first.rows, 3)
	for _, r := range first.rows {
		assert.True(t, r.isRawChanged)
	}
	later := testNow.Add(5 * time.Second)
	second := b.apply(matchResultDelta("bet9ja", 2.3, 3.5, 3.2, later), later)
	changed := map[canon.Side]bool{}
	for _, r := range second.rows {
		changed[r.cell.Side] = r.isRawChanged
	}
	assert.Equal(t, map[canon.Side]bool{canon.SideHome: false, canon.SideDraw: true, canon.SideAway: false}, changed)
}

func TestBoardTombstonesRemovedSelections(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	b.apply(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), testNow)
	cs := b.apply(decimalDelta("bet9ja", matchResult, testNow, sel(canon.SideHome, "", 2.3, testNow), sel(canon.SideAway, "", 3.2, testNow)), testNow)
	var drawRemoved bool
	for _, c := range cs.removed {
		drawRemoved = drawRemoved || (c.Source == "bet9ja" && c.Side == canon.SideDraw && c.MarketKey == matchResult.String() && c.Price.Suspended)
	}
	assert.True(t, drawRemoved)
}

func TestBoardDerivesDoubleChanceAndDrawNoBet(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	cs := b.apply(matchResultDelta("bet9ja", 2.0, 3.5, 4.0, testNow), testNow)
	dc := cellsFrom(cs, canon.MarketKey{EventID: "ev1", Code: canon.CodeDC}.String(), "bet9ja")
	dnb := cellsFrom(cs, canon.MarketKey{EventID: "ev1", Code: canon.CodeDNB}.String(), "bet9ja")
	require.Len(t, dc, 3)
	require.Len(t, dnb, 2)
	for _, c := range append(dc, dnb...) {
		assert.True(t, c.Derived)
		require.NotNil(t, c.Fair)
	}
	assert.Equal(t, "1X", dc[0].Label)
	assert.InDelta(t, 1/(1/2.0+1/3.5), dc[0].Price.Raw, 1e-9)
}

func TestBoardRealDoubleChanceBeatsDerived(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	dcKey := canon.MarketKey{EventID: "ev1", Code: canon.CodeDC}
	b.apply(decimalDelta("bet9ja", dcKey, testNow, sel("", "1X", 1.3, testNow), sel("", "12", 1.3, testNow), sel("", "X2", 1.9, testNow)), testNow)
	cs := b.apply(matchResultDelta("bet9ja", 2.0, 3.5, 4.0, testNow), testNow)
	assert.Empty(t, cellsFrom(cs, dcKey.String(), "bet9ja"))
	assert.False(t, b.markets[dcKey.String()].sources["bet9ja"].isComputed)
}

func TestBoardDerivedNeverBecomesArbLeg(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	dcKey := canon.MarketKey{EventID: "ev1", Code: canon.CodeDC}
	b.apply(matchResultDelta("bet9ja", 1.5, 6.0, 9.0, testNow), testNow)
	cs := b.apply(decimalDelta("betking", dcKey, testNow, sel("", "1X", 1.05, testNow), sel("", "12", 1.05, testNow), sel("", "X2", 9.0, testNow)), testNow)
	for _, s := range signalsOf(cs, canon.SignalArb, canon.SignalOpen) {
		for _, leg := range s.Legs {
			assert.NotEqual(t, "bet9ja", leg.Source, "derived leg used in %s", s.DedupeKey)
		}
	}
}

func TestBoardLaddersBucketsAgainstBookLine(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	shots := canon.MarketKey{EventID: "ev1", Code: canon.TeamShots}
	bucket := func(label string, naira float64) canon.Selection {
		return canon.Selection{Label: label, Price: canon.NewBookPrice(nil, new(naira), new(400.0), canon.FormNaira, testNow)}
	}
	cs := b.apply(canon.Delta{Source: "bayse", Market: canon.Market{Key: shots, Rules: canon.StandardRule, Selections: []canon.Selection{bucket("<24", 30), bucket("24-31", 50), bucket("32+", 25)}}}, testNow)
	rungKey := canon.MarketKey{EventID: "ev1", Code: canon.TeamShots, Line: canon.Line(23.5)}
	rung := cellsFrom(cs, rungKey.String(), "bayse")
	require.Len(t, rung, 2)
	assert.InDelta(t, 0.75, rung[0].Price.Prob, 1e-9)
	assert.False(t, rung[0].Derived)
	require.NotNil(t, rung[0].Fair)

	arb := b.apply(decimalDelta("betano", rungKey, testNow, sel(canon.SideOver, "", 1.25, testNow), sel(canon.SideUnder, "", 4.5, testNow)), testNow)
	opened := signalsOf(arb, canon.SignalArb, canon.SignalOpen)
	require.Len(t, opened, 1)
	assert.InDelta(t, 1-(0.75+1/4.5), opened[0].Margin, 1e-9)
	assert.Equal(t, "bayse", opened[0].Legs[0].Source)
}

func TestBoardFlagsPrizePicksLineGap(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	at := func(line float64) canon.MarketKey {
		return canon.MarketKey{EventID: "ev1", Code: canon.PropShots, PlayerID: "p1", Line: canon.Line(line)}
	}
	b.apply(decimalDelta("bet9ja", at(1.5), testNow, sel(canon.SideOver, "", 1.9, testNow), sel(canon.SideUnder, "", 1.9, testNow)), testNow)
	b.apply(decimalDelta("bet9ja", at(2.5), testNow, sel(canon.SideOver, "", 3.4, testNow), sel(canon.SideUnder, "", 1.3, testNow)), testNow)
	lineOnly := canon.Price{Form: canon.FormLineOnly, ObservedAt: testNow}
	cs := b.apply(canon.Delta{Source: "prizepicks", Market: canon.Market{Key: at(2.5), Rules: canon.StandardRule, Selections: []canon.Selection{{Side: canon.SideOver, Price: lineOnly}, {Side: canon.SideUnder, Price: lineOnly}}}}, testNow)
	gaps := signalsOf(cs, canon.SignalLine, canon.SignalOpen)
	require.Len(t, gaps, 1)
	assert.InDelta(t, 1.0, gaps[0].Margin, 1e-9)
	assert.Equal(t, at(2.5).String(), gaps[0].Market.String())
}

func TestBoardSignalsPriceMove(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	b.apply(matchResultDelta("bet9ja", 2.0, 3.5, 4.0, testNow), testNow)
	later := testNow.Add(5 * time.Minute)
	assert.Empty(t, signalsOf(b.apply(matchResultDelta("bet9ja", 1.8, 3.6, 4.4, later), later), canon.SignalMove, canon.SignalOpen))
	held := later.Add(testMoveHold)
	cs := b.apply(matchResultDelta("betking", 2.1, 3.4, 3.6, held), held)
	moves := signalsOf(cs, canon.SignalMove, canon.SignalOpen)
	require.NotEmpty(t, moves)
	afterWindow := later.Add(20 * time.Minute)
	closed := b.apply(matchResultDelta("bet9ja", 1.8, 3.6, 4.4, afterWindow), afterWindow)
	assert.Len(t, signalsOf(closed, canon.SignalMove, canon.SignalClosed), len(moves))
}

func TestBoardConsensusCountsDistinctFeeds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		books   []string
		expects bool
	}{
		{name: "three brands of one feed give no fair price", books: []string{"22bet", "paripesa", "betwinner"}},
		{name: "brands plus one independent book is still two feeds", books: []string{"22bet", "paripesa", "betwinner", "betking"}},
		{name: "three independent feeds give a fair price", books: []string{"22bet", "paripesa", "betking", "betway"}, expects: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBoard()
			for _, source := range tt.books {
				b.apply(matchResultDelta(source, 2.1, 3.4, 3.6, testNow), testNow)
			}
			assert.Equal(t, tt.expects, len(b.markets[matchResult.String()].consensus) == 3)
		})
	}
}

func TestBoardExcludesOverroundMarketFromConsensusAndSignals(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	totals := canon.MarketKey{EventID: "ev1", Code: canon.CodeOU, Line: canon.Line(2.5)}
	for _, source := range []string{"betking", "betway", "bet9ja"} {
		b.apply(decimalDelta(source, totals, testNow, sel(canon.SideOver, "", 1.9, testNow), sel(canon.SideUnder, "", 1.9, testNow)), testNow)
	}
	ask := func(side canon.Side, price float64) canon.Selection {
		return canon.Selection{Side: side, Price: canon.NewBookPrice(nil, new(price), new(500.0), canon.FormShare, testNow)}
	}
	cs := b.apply(canon.Delta{Source: "polymarket", Market: canon.Market{Key: totals, Rules: canon.StandardRule, Selections: []canon.Selection{ask(canon.SideOver, 0.9), ask(canon.SideUnder, 0.98)}}}, testNow)
	assert.Len(t, cellsFrom(cs, totals.String(), "polymarket"), 2)
	assert.Empty(t, cs.signals)
	ms := b.markets[totals.String()]
	assert.True(t, ms.sources["polymarket"].hasExcessOverround)
	require.Len(t, ms.consensus, 2)
	assert.InDelta(t, 0.5, *ms.consensus[signal.Outcome{Side: canon.SideOver}].Fair, 1e-9)
}

func TestBoardDropsDuplicateCellsWithinDelta(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	before := droppedDuplicates(t, "dupebook")
	cs := b.apply(decimalDelta("dupebook", matchResult, testNow, sel(canon.SideHome, "", 2.0, testNow), sel(canon.SideHome, "", 9.0, testNow), sel(canon.SideAway, "", 3.0, testNow)), testNow)
	home := cellsFrom(cs, matchResult.String(), "dupebook")
	require.Len(t, home, 2)
	assert.InDelta(t, 2.0, home[0].Price.Raw, 1e-9)
	assert.InDelta(t, 1, droppedDuplicates(t, "dupebook")-before, 1e-9)
}

func droppedDuplicates(t *testing.T, source string) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, duplicateCellsDropped.WithLabelValues(source).Write(&m))
	return m.GetCounter().GetValue()
}

func TestBoardIgnoresMovesFromCollidingSnapshots(t *testing.T) {
	t.Parallel()
	shotsKey := canon.MarketKey{EventID: "ev1", Code: canon.PropShots, PlayerID: "p1", Line: canon.Line(1.5)}
	delta := func(sourceMarket string, over float64, at time.Time) canon.Delta {
		d := decimalDelta("footballcom", shotsKey, at, sel(canon.SideOver, "", over, at), sel(canon.SideUnder, "", 1.6, at))
		d.Market.SourceKey = sourceMarket
		return d
	}
	later := testNow.Add(time.Minute)
	tests := []struct {
		name   string
		deltas []canon.Delta
	}{
		{name: "two prices with the same observed at", deltas: []canon.Delta{delta("shots", 1.31, testNow), delta("shots", 2.16, testNow)}},
		{name: "flip flop between two snapshots", deltas: []canon.Delta{delta("shots", 1.31, testNow), delta("shots", 2.16, testNow), delta("shots", 1.31, later), delta("shots", 2.16, later)}},
		{name: "different source market behind the same key", deltas: []canon.Delta{delta("shots", 1.31, testNow), delta("shots-on-target", 2.16, later)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBoard()
			var moves []canon.Signal
			for _, d := range tt.deltas {
				moves = append(moves, signalsOf(b.apply(d, d.FetchedAt), canon.SignalMove, canon.SignalOpen)...)
			}
			afterHold := later.Add(3 * testMoveHold)
			rescan := decimalDelta("betking", shotsKey, afterHold, sel(canon.SideOver, "", 1.5, afterHold), sel(canon.SideUnder, "", 2.5, afterHold))
			moves = append(moves, signalsOf(b.apply(rescan, afterHold), canon.SignalMove, canon.SignalOpen)...)
			assert.Empty(t, moves)
		})
	}
}

func TestBoardRestoredPricesNeverStartAMove(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	b.restore(matchResultDelta("bet9ja", 44, 3.5, 1.2, testNow), testNow)
	later := testNow.Add(time.Minute)
	b.apply(matchResultDelta("bet9ja", 13, 3.5, 1.2, later), later)
	held := later.Add(testMoveHold)
	cs := b.apply(matchResultDelta("betking", 13, 3.5, 1.2, held), held)
	assert.Empty(t, signalsOf(cs, canon.SignalMove, canon.SignalOpen))
}

func TestBoardRestoreReconcilesAdoptedSignals(t *testing.T) {
	t.Parallel()
	live := newTestBoard()
	live.apply(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), testNow)
	live.apply(matchResultDelta("betking", 2.1, 3.9, 3.3, testNow), testNow)
	opened := signalsOf(live.apply(matchResultDelta("sportybet", 2.0, 3.5, 4.4, testNow), testNow), canon.SignalArb, canon.SignalOpen)
	require.Len(t, opened, 1)
	stale := canon.Signal{ID: "stale", DedupeKey: "arb|gone", Type: canon.SignalArb, Status: canon.SignalOpen, EventID: "ev1", Market: canon.MarketKey{EventID: "ev1", Code: canon.CodeBTTS}}

	restarted := newTestBoard()
	restarted.adoptOpenSignal(opened[0])
	restarted.adoptOpenSignal(stale)
	for _, d := range []canon.Delta{matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), matchResultDelta("betking", 2.1, 3.9, 3.3, testNow), matchResultDelta("sportybet", 2.0, 3.5, 4.4, testNow)} {
		restarted.restore(d, testNow)
	}
	cs := restarted.rescanEvent("ev1", testNow)
	assert.Empty(t, signalsOf(cs, canon.SignalArb, canon.SignalOpen))
	assert.NotEmpty(t, cellsFrom(cs, matchResult.String(), "bet9ja"))
	orphans := restarted.closeOrphanedSignals(testNow)
	require.Len(t, orphans.signals, 1)
	assert.Equal(t, "stale", orphans.signals[0].ID)
	assert.Equal(t, canon.SignalClosed, orphans.signals[0].Status)
}

func BenchmarkBoardApplyChangedMarket(b *testing.B) {
	const books = 15
	board := newTestBoard()
	for i := range books {
		board.apply(matchResultDelta(fmt.Sprintf("book%02d", i), 2.0+float64(i)*0.01, 3.4, 3.9, testNow), testNow)
	}
	b.ReportAllocs()
	tick := 0
	for b.Loop() {
		at := testNow.Add(time.Duration(tick) * time.Second)
		home := 2.0 + float64(tick%7)*0.01
		board.apply(matchResultDelta(fmt.Sprintf("book%02d", tick%books), home, 3.4, 3.9, at), at)
		tick++
	}
}
