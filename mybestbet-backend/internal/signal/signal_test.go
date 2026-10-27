package signal

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

var (
	now      = time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC)
	knobs    = config.Pricing{ArbMinMargin: 0.005, EVMin: 0.03, MaxLegSkew: 15 * time.Second, MinTouchLiquidity: 50}
	matchKey = canon.MarketKey{EventID: "ev1", Code: canon.Code1X2}
)

func book(source string, side canon.Side, decimal float64) Quote {
	return Quote{Outcome: Outcome{Side: side}, Source: source, Price: canon.NewPrice(decimal, canon.FormDecimal, now), Rules: canon.StandardRule}
}

func threeWay(source string, home, draw, away float64) []Quote {
	return []Quote{book(source, canon.SideHome, home), book(source, canon.SideDraw, draw), book(source, canon.SideAway, away)}
}

func arbQuotes() []Quote {
	quotes := threeWay("bet9ja", 2.3, 3.4, 3.2)
	quotes = append(quotes, threeWay("betking", 2.1, 3.9, 3.3)...)
	return append(quotes, threeWay("sportybet", 2.0, 3.5, 4.4)...)
}

func TestFindArbsConstructed(t *testing.T) {
	t.Parallel()
	found := FindArbs(matchKey, arbQuotes(), knobs, now)
	require.Len(t, found, 1)
	arb := found[0]
	overround := 1/2.3 + 1/3.9 + 1/4.4
	assert.InDelta(t, 1-overround, arb.Margin, 1e-9)
	assert.Equal(t, canon.SignalArb, arb.Type)
	assert.Equal(t, "arb|ev1|1X2||||away:=sportybet,draw:=betking,home:=bet9ja", arb.DedupeKey)
	var staked float64
	for _, leg := range arb.Legs {
		assert.Equal(t, matchKey.String(), leg.MarketKey)
		assert.Nil(t, leg.Line)
		staked += leg.Stake
		assert.InDelta(t, StakeBankroll/overround, leg.Stake*leg.Price.Raw, 1e-9)
	}
	assert.InDelta(t, StakeBankroll, staked, 1e-9)
}

func TestFindArbsRejects(t *testing.T) {
	t.Parallel()
	mutateAwayLeg := func(mutate func(*Quote)) []Quote {
		quotes := arbQuotes()
		mutate(&quotes[8])
		return quotes
	}
	tests := []struct {
		name   string
		quotes []Quote
	}{
		{name: "stale leg", quotes: mutateAwayLeg(func(q *Quote) { q.Price.ObservedAt = now.Add(-16 * time.Second) })},
		{name: "suspended leg", quotes: mutateAwayLeg(func(q *Quote) { q.Price.Suspended = true })},
		{name: "derived leg", quotes: mutateAwayLeg(func(q *Quote) { q.Derived = true })},
		{name: "degraded source", quotes: mutateAwayLeg(func(q *Quote) { q.Degraded = true })},
		{name: "quote past its max age", quotes: mutateAwayLeg(func(q *Quote) { q.Stale = true })},
		{name: "settle rule mismatch", quotes: mutateAwayLeg(func(q *Quote) { q.Rules.RegulationOnly = false })},
		{name: "void rule mismatch", quotes: mutateAwayLeg(func(q *Quote) { q.Rules.VoidIfNoStart = true })},
		{name: "line only leg", quotes: mutateAwayLeg(func(q *Quote) { q.Price = canon.Price{Form: canon.FormLineOnly, ObservedAt: now} })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range FindArbs(matchKey, tt.quotes, knobs, now) {
				assert.NotContains(t, s.DedupeKey, "away:=sportybet", "unexpected arb %+v", s)
			}
		})
	}
}

func TestFindArbsNone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		quotes []Quote
	}{
		{name: "books overlap", quotes: append(threeWay("bet9ja", 2.0, 3.3, 3.5), threeWay("betking", 2.05, 3.2, 3.4)...)},
		{name: "margin below threshold", quotes: append(threeWay("bet9ja", 2.0, 3.6, 4.1), threeWay("betking", 2.1, 3.5, 3.9)...)},
		{name: "single source", quotes: threeWay("bet9ja", 2.0, 3.3, 3.5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, FindArbs(matchKey, tt.quotes, knobs, now))
		})
	}
}

func TestFindArbsSkewIsRelativeToNewestLeg(t *testing.T) {
	t.Parallel()
	quotes := arbQuotes()
	for i := range quotes {
		quotes[i].Price.ObservedAt = now.Add(-time.Minute)
	}
	found := FindArbs(matchKey, quotes, knobs, now)
	require.Len(t, found, 1)
	assert.Equal(t, time.Minute, found[0].OldestLegAge)
}

func TestFindArbsPredictionMarketFeesAndDepth(t *testing.T) {
	t.Parallel()
	key := canon.MarketKey{EventID: "ev1", Code: canon.CodeBTTS}
	yes := Quote{Outcome: Outcome{Side: canon.SideYes}, Source: "kalshi", Rules: canon.StandardRule, FeePerShare: 0.02,
		Price: canon.NewBookPrice(new(0.4), new(0.42), new(500.0), canon.FormShare, now)}
	no := book("bet9ja", canon.SideNo, 1.85)
	noAtKalshi := Quote{Outcome: Outcome{Side: canon.SideNo}, Source: "kalshi", Rules: canon.StandardRule, FeePerShare: 0.02,
		Price: canon.NewBookPrice(new(0.55), new(0.6), new(500.0), canon.FormShare, now)}
	yesAtBook := book("bet9ja", canon.SideYes, 2.2)

	withFee := FindArbs(key, []Quote{yes, no, noAtKalshi, yesAtBook}, knobs, now)
	require.Len(t, withFee, 1)
	assert.InDelta(t, 1-(0.44+1/1.85), withFee[0].Margin, 1e-9)

	thin := yes
	thin.Price.Liquidity = new(20.0)
	assert.Empty(t, FindArbs(key, []Quote{thin, no, noAtKalshi, yesAtBook}, knobs, now))
}

func TestFindArbsNeverDropsAnOutcome(t *testing.T) {
	t.Parallel()
	share := func(side canon.Side, ask, liquidity float64, observedAt time.Time) Quote {
		return Quote{Outcome: Outcome{Side: side}, Source: "polymarket", Rules: canon.StandardRule, Price: canon.NewBookPrice(nil, new(ask), new(liquidity), canon.FormShare, observedAt)}
	}
	tests := []struct {
		name string
		home Quote
	}{
		{name: "home leg older than the skew", home: share(canon.SideHome, 0.56, 3000, now.Add(-20*time.Second))},
		{name: "home leg too thin", home: share(canon.SideHome, 0.47, 22, now)},
		{name: "home leg suspended", home: func() Quote { q := share(canon.SideHome, 0.56, 3000, now); q.Price.Suspended = true; return q }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			quotes := []Quote{tt.home, share(canon.SideDraw, 0.24, 2000, now), share(canon.SideAway, 0.22, 1200, now)}
			assert.Empty(t, FindArbs(matchKey, quotes, knobs, now))
		})
	}
}

func TestFindArbsRequiresCompleteOutcomeSet(t *testing.T) {
	t.Parallel()
	key := canon.MarketKey{EventID: "ev1", Code: canon.CodeCS}
	quotes := []Quote{
		{Outcome: Outcome{Label: "1-0"}, Source: "a", Price: canon.NewPrice(9, canon.FormDecimal, now), Rules: canon.StandardRule},
		{Outcome: Outcome{Label: "0-0"}, Source: "b", Price: canon.NewPrice(9, canon.FormDecimal, now), Rules: canon.StandardRule},
	}
	assert.Empty(t, FindArbs(key, quotes, knobs, now))
}

func TestFindArbsRejectsThreeWayMissingAnOutcome(t *testing.T) {
	t.Parallel()
	key := canon.MarketKey{EventID: "ev1", Code: canon.Code1H1X2, Period: canon.PeriodFirstHalf}
	quotes := []Quote{book("onexbet", canon.SideHome, 17), book("onexbet", canon.SideDraw, 10)}
	assert.Empty(t, FindArbs(key, quotes, knobs, now))
	assert.Empty(t, FindArbs(matchKey, quotes, knobs, now))
}

func TestFindArbsAsianHandicapPushAware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     float64
		expected float64
		isArb    bool
	}{
		{name: "half line keeps full margin", line: -0.5, expected: 1 - (1/2.1 + 1/2.1), isArb: true},
		{name: "whole line ignores refund scenario", line: 0, expected: 1 - (1/2.1 + 1/2.1), isArb: true},
		{name: "quarter line margin is reduced", line: -0.25, expected: 1 - 100/102.5, isArb: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key := canon.MarketKey{EventID: "ev1", Code: canon.CodeAH, Line: canon.Line(tt.line)}
			quotes := []Quote{book("a", canon.SideHome, 2.1), book("a", canon.SideAway, 1.7), book("b", canon.SideHome, 1.7), book("b", canon.SideAway, 2.1)}
			found := FindArbs(key, quotes, knobs, now)
			require.Len(t, found, 1)
			assert.InDelta(t, tt.expected, found[0].Margin, 1e-9)
		})
	}
}

func TestFindValue(t *testing.T) {
	t.Parallel()
	fair := map[Outcome]float64{{Side: canon.SideHome}: 0.5, {Side: canon.SideAway}: 0.3}
	tests := []struct {
		name    string
		quote   Quote
		expects bool
		ev      float64
	}{
		{name: "above threshold", quote: book("bet9ja", canon.SideHome, 2.1), expects: true, ev: 0.05},
		{name: "just above threshold", quote: book("bet9ja", canon.SideHome, 2.07), expects: true, ev: 0.035},
		{name: "below threshold", quote: book("bet9ja", canon.SideHome, 2.05)},
		{name: "no fair price for outcome", quote: book("bet9ja", canon.SideDraw, 9)},
		{name: "derived price", quote: func() Quote { q := book("bet9ja", canon.SideHome, 2.5); q.Derived = true; return q }()},
		{name: "suspended price", quote: func() Quote { q := book("bet9ja", canon.SideHome, 2.5); q.Price.Suspended = true; return q }()},
		{name: "stale price", quote: func() Quote { q := book("bet9ja", canon.SideHome, 2.5); q.Stale = true; return q }()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			found := FindValue(matchKey, []Quote{tt.quote}, fair, knobs, now)
			if !tt.expects {
				assert.Empty(t, found)
				return
			}
			require.Len(t, found, 1)
			assert.InDelta(t, tt.ev, found[0].EV, 1e-9)
			assert.Equal(t, "ev|ev1|1X2||||bet9ja|home:", found[0].DedupeKey)
		})
	}
}

func TestFindLineGaps(t *testing.T) {
	t.Parallel()
	family := canon.MarketKey{EventID: "ev1", Code: canon.PropShots, PlayerID: "p1"}
	at := func(line float64) canon.MarketKey { k := family; k.Line = canon.Line(line); return k }
	offer := func(source string, line, chance float64) LineOffer {
		q := book(source, canon.SideOver, 1/chance)
		return LineOffer{Market: at(line), Over: q, OverChance: chance}
	}
	pick := LineOffer{Market: at(2.5), Over: Quote{Outcome: Outcome{Side: canon.SideOver}, Source: "prizepicks", Price: canon.Price{Form: canon.FormLineOnly, ObservedAt: now}}}

	books := []LineOffer{offer("bet9ja", 1.5, 0.52), offer("bet9ja", 2.5, 0.3), offer("betking", 1.5, 0.55), offer("betano", 2.5, 0.48)}
	found := FindLineGaps(append(books, pick), now)
	require.Len(t, found, 1)
	assert.Equal(t, canon.SignalLine, found[0].Type)
	assert.InDelta(t, 1.0, found[0].Margin, 1e-9)
	require.Len(t, found[0].Legs, 4)
	for _, leg := range found[0].Legs {
		require.NotNil(t, leg.Line)
		assert.Equal(t, at(*leg.Line).String(), leg.MarketKey)
	}
	assert.InDelta(t, 2.5, *found[0].Legs[0].Line, 0)
	assert.Equal(t, "prizepicks", found[0].Legs[0].Source)

	agreeing := []LineOffer{offer("bet9ja", 2.5, 0.5), offer("betking", 2.5, 0.52)}
	assert.Empty(t, FindLineGaps(append(agreeing, pick), now))
	assert.Empty(t, FindLineGaps([]LineOffer{pick}, now))
}

func TestFindValueFairProbRange(t *testing.T) {
	t.Parallel()
	ranged := knobs
	ranged.EVProbRange = config.ProbRange{Min: 0.05, Max: 0.95}
	under := Outcome{Side: canon.SideUnder}
	tests := []struct {
		name    string
		fair    float64
		decimal float64
		expects bool
	}{
		{name: "fair inside range", fair: 0.3, decimal: 3.6, expects: true},
		{name: "fair below range", fair: 0.04, decimal: 40},
		{name: "fair above range", fair: 0.97, decimal: 1.1},
		{name: "quote priced below range", fair: 0.08, decimal: 63.7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			found := FindValue(matchKey, []Quote{book("bet9ja", canon.SideUnder, tt.decimal)}, map[Outcome]float64{under: tt.fair}, ranged, now)
			assert.Equal(t, tt.expects, len(found) == 1)
		})
	}
}

func TestFindValueOneSignalPerFeed(t *testing.T) {
	t.Parallel()
	fair := map[Outcome]float64{{Side: canon.SideHome}: 0.5}
	brand := func(source, feed string, decimal float64) Quote {
		q := book(source, canon.SideHome, decimal)
		q.Feed = feed
		return q
	}
	quotes := []Quote{brand("paripesa", "onexbet", 2.2), brand("22bet", "onexbet", 2.2), brand("betwinner", "onexbet", 2.3), brand("betking", "betking", 2.2)}
	found := FindValue(matchKey, quotes, fair, knobs, now)
	require.Len(t, found, 2)
	assert.Equal(t, "betwinner", found[0].Legs[0].Source)
	assert.Equal(t, "betking", found[1].Legs[0].Source)
}

func movePoint(decimal float64, observedAt, seenAt time.Time) MovePoint {
	return NewMovePoint(canon.NewPrice(decimal, canon.FormDecimal, observedAt), seenAt)
}

func TestDetectMove(t *testing.T) {
	t.Parallel()
	earlier, recent := now.Add(-10*time.Minute), now.Add(-5*time.Minute)
	tests := []struct {
		name       string
		history    []MovePoint
		current    float64
		minHold    time.Duration
		isDegraded bool
		expects    bool
		contains   string
	}{
		{name: "shortening beyond threshold", history: []MovePoint{movePoint(2.0, earlier, earlier), movePoint(1.8, recent, recent)}, current: 1.8, expects: true, contains: "|up"},
		{name: "drifting beyond threshold", history: []MovePoint{movePoint(2.0, earlier, earlier), movePoint(2.3, recent, recent)}, current: 2.3, expects: true, contains: "|down"},
		{name: "small move", history: []MovePoint{movePoint(2.0, earlier, earlier), movePoint(1.97, recent, recent)}, current: 1.97},
		{name: "same observed at is one snapshot not a move", history: []MovePoint{movePoint(61, recent, recent), movePoint(11.5, recent, recent)}, current: 11.5},
		{name: "start replaced within its own snapshot is skipped", history: []MovePoint{movePoint(11.5, earlier, earlier), movePoint(61, recent, recent), movePoint(11.5, recent, recent)}, current: 11.5},
		{name: "new price not yet held long enough", history: []MovePoint{movePoint(2.0, earlier, earlier), movePoint(1.8, recent, now.Add(-3*time.Second))}, current: 1.8, minHold: 10 * time.Second},
		{name: "new price held long enough", history: []MovePoint{movePoint(2.0, earlier, earlier), movePoint(1.8, recent, now.Add(-30*time.Second))}, current: 1.8, minHold: 10 * time.Second, expects: true, contains: "|up"},
		{name: "degraded or over margin quote", history: []MovePoint{movePoint(2.0, earlier, earlier), movePoint(1.8, recent, recent)}, current: 1.8, isDegraded: true},
		{name: "current no longer the latest point", history: []MovePoint{movePoint(2.0, earlier, earlier), movePoint(1.8, recent, recent)}, current: 2.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			current := book("bet9ja", canon.SideHome, tt.current)
			current.Degraded = tt.isDegraded
			s, ok := DetectMove(matchKey, current, tt.history, tt.minHold, now)
			require.Equal(t, tt.expects, ok)
			if ok {
				assert.Contains(t, s.DedupeKey, tt.contains)
				assert.GreaterOrEqual(t, s.Margin, MinMoveFraction)
				require.Len(t, s.Legs, 2)
				require.NotNil(t, s.FromPrice)
				require.NotNil(t, s.ToPrice)
				assert.Equal(t, s.Legs[0].Price, *s.FromPrice)
				assert.InDelta(t, 1/tt.current, s.ToPrice.Prob, 1e-9)
				assert.InDelta(t, s.ToPrice.Prob-s.FromPrice.Prob, s.Change, 1e-12)
				assert.InDelta(t, s.Margin, math.Abs(s.Change)/s.FromPrice.Prob, 1e-9)
				assert.Equal(t, matchKey.String(), s.Legs[1].MarketKey)
			}
		})
	}
}

func TestTrimMoveWindow(t *testing.T) {
	t.Parallel()
	points := []MovePoint{movePoint(2, now.Add(-20*time.Minute), now), movePoint(2, now.Add(-10*time.Minute), now), movePoint(2, now, now)}
	assert.Len(t, TrimMoveWindow(points, now.Add(-15*time.Minute)), 2)
}

func TestDescribeMove(t *testing.T) {
	t.Parallel()
	from, to := canon.NewPrice(2.0, canon.FormDecimal, now), canon.NewPrice(2.5, canon.FormDecimal, now)
	tests := []struct {
		name      string
		signal    canon.Signal
		isFilled  bool
		changeWas float64
	}{
		{name: "stored move regains its prices", signal: canon.Signal{Type: canon.SignalMove, Legs: []canon.Leg{{Price: from}, {Price: to}}}, isFilled: true, changeWas: 0.4 - 0.5},
		{name: "arb is left alone", signal: canon.Signal{Type: canon.SignalArb, Legs: []canon.Leg{{Price: from}, {Price: to}}}},
		{name: "move without two legs is left alone", signal: canon.Signal{Type: canon.SignalMove, Legs: []canon.Leg{{Price: from}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := tt.signal
			DescribeMove(&s)
			if !tt.isFilled {
				assert.Nil(t, s.FromPrice)
				assert.Nil(t, s.ToPrice)
				assert.Zero(t, s.Change)
				return
			}
			assert.Equal(t, from, *s.FromPrice)
			assert.Equal(t, to, *s.ToPrice)
			assert.InDelta(t, tt.changeWas, s.Change, 1e-12)
		})
	}
}
