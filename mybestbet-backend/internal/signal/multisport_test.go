package signal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func twoBooksCrossed(first, second canon.Side, best, worst float64) []Quote {
	return []Quote{book("a", first, best), book("a", second, worst), book("b", first, worst), book("b", second, best)}
}

func withRule(q Quote, rule canon.SettleRule) Quote {
	q.Rules = rule
	return q
}

func TestFindArbsAcrossSportsAndShapes(t *testing.T) {
	t.Parallel()
	crossedMargin := 1 - 2/2.1
	quarterMargin := 1 - 100/102.5
	regulation := canon.StandardRule
	includingOvertime := canon.SettleRule{}
	tests := []struct {
		name     string
		key      canon.MarketKey
		quotes   []Quote
		isArb    bool
		expected float64
	}{
		{name: "basketball moneyline arb", key: canon.MarketKey{EventID: "nba", Code: canon.CodeMoneyline}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "basketball moneyline overlap", key: canon.MarketKey{EventID: "nba", Code: canon.CodeMoneyline}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 1.95, 1.85)},
		{name: "tennis moneyline arb", key: canon.MarketKey{EventID: "atp", Code: canon.CodeMoneyline}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "esports first map winner arb", key: canon.MarketKey{EventID: "cs2", Code: canon.CodeMoneyline, Period: canon.PeriodFirstMap}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "spread half line", key: canon.MarketKey{EventID: "nba", Code: canon.CodeSpread, Line: canon.Line(-3.5)}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "spread whole line skips the refund", key: canon.MarketKey{EventID: "nba", Code: canon.CodeSpread, Line: canon.Line(-3)}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "spread quarter line reduced", key: canon.MarketKey{EventID: "nfl", Code: canon.CodeSpread, Line: canon.Line(-3.25)}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: quarterMargin},
		{name: "spread whole line overlap", key: canon.MarketKey{EventID: "nba", Code: canon.CodeSpread, Line: canon.Line(-3)}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 1.92, 1.9)},
		{name: "total whole line skips the refund", key: canon.MarketKey{EventID: "nba", Code: canon.CodeTotal, Line: canon.Line(220)}, quotes: twoBooksCrossed(canon.SideOver, canon.SideUnder, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "total quarter line reduced", key: canon.MarketKey{EventID: "nfl", Code: canon.CodeTotal, Line: canon.Line(44.25)}, quotes: twoBooksCrossed(canon.SideOver, canon.SideUnder, 2.1, 1.7), isArb: true, expected: quarterMargin},
		{name: "first quarter total", key: canon.MarketKey{EventID: "nba", Code: canon.CodeTotal, Line: canon.Line(55), Period: canon.PeriodQuarter1}, quotes: twoBooksCrossed(canon.SideOver, canon.SideUnder, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "tennis set handicap", key: canon.MarketKey{EventID: "atp", Code: canon.CodeSetHcp, Line: canon.Line(-1.5)}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "tennis total games whole line", key: canon.MarketKey{EventID: "atp", Code: canon.CodeTotalGames, Line: canon.Line(22)}, quotes: twoBooksCrossed(canon.SideOver, canon.SideUnder, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "esports map handicap", key: canon.MarketKey{EventID: "cs2", Code: canon.CodeMapHcp, Line: canon.Line(1.5)}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "esports total maps overlap", key: canon.MarketKey{EventID: "cs2", Code: canon.CodeTotalMaps, Line: canon.Line(2.5)}, quotes: twoBooksCrossed(canon.SideOver, canon.SideUnder, 1.9, 1.85)},
		{name: "basketball points prop whole line", key: canon.MarketKey{EventID: "nba", Code: canon.PropPoints, Line: canon.Line(25), PlayerID: "p1"}, quotes: twoBooksCrossed(canon.SideOver, canon.SideUnder, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "receptions prop quarter line reduced", key: canon.MarketKey{EventID: "nfl", Code: canon.PropReceptions, Line: canon.Line(4.25), PlayerID: "p1"}, quotes: twoBooksCrossed(canon.SideOver, canon.SideUnder, 2.1, 1.7), isArb: true, expected: quarterMargin},
		{name: "anytime touchdown yes no", key: canon.MarketKey{EventID: "nfl", Code: canon.PropAnytimeTD, PlayerID: "p1"}, quotes: twoBooksCrossed(canon.SideYes, canon.SideNo, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{name: "qualify arb", key: canon.MarketKey{EventID: "cup", Code: canon.CodeQualify}, quotes: twoBooksCrossed(canon.SideHome, canon.SideAway, 2.1, 1.7), isArb: true, expected: crossedMargin},
		{
			name: "moneyline never pairs with a regulation only leg",
			key:  canon.MarketKey{EventID: "nba", Code: canon.CodeMoneyline},
			quotes: []Quote{
				withRule(book("a", canon.SideHome, 2.1), includingOvertime), withRule(book("a", canon.SideAway, 1.7), includingOvertime),
				withRule(book("b", canon.SideHome, 1.7), regulation), withRule(book("b", canon.SideAway, 2.1), regulation),
			},
		},
		{
			name: "qualify never pairs with a regulation only leg",
			key:  canon.MarketKey{EventID: "cup", Code: canon.CodeQualify},
			quotes: []Quote{
				withRule(book("a", canon.SideHome, 2.1), includingOvertime), withRule(book("a", canon.SideAway, 1.7), includingOvertime),
				withRule(book("b", canon.SideHome, 1.7), regulation), withRule(book("b", canon.SideAway, 2.1), regulation),
			},
		},
		{
			name: "moneyline pairs legs that share the overtime rule",
			key:  canon.MarketKey{EventID: "nba", Code: canon.CodeMoneyline},
			quotes: []Quote{
				withRule(book("a", canon.SideHome, 2.1), includingOvertime), withRule(book("a", canon.SideAway, 1.7), includingOvertime),
				withRule(book("b", canon.SideHome, 1.7), includingOvertime), withRule(book("b", canon.SideAway, 2.1), includingOvertime),
			},
			isArb: true, expected: crossedMargin,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			found := FindArbs(tt.key, tt.quotes, knobs, now)
			if !tt.isArb {
				assert.Empty(t, found)
				return
			}
			require.Len(t, found, 1)
			assert.InDelta(t, tt.expected, found[0].Margin, 1e-9)
			assert.Len(t, found[0].Legs, 2)
			assert.Equal(t, tt.key, found[0].Market)
		})
	}
}

func TestFindValueOnPropsOfEverySport(t *testing.T) {
	t.Parallel()
	over := Outcome{Side: canon.SideOver}
	for _, code := range []canon.MarketCode{canon.PropShots, canon.PropPoints, canon.PropPassYards, canon.PropAces, canon.PropMapKills} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			key := canon.MarketKey{EventID: "ev1", Code: code, Line: canon.Line(20.5), PlayerID: "p1"}
			found := FindValue(key, []Quote{book("bet9ja", canon.SideOver, 2.1)}, map[Outcome]float64{over: 0.5}, knobs, now)
			require.Len(t, found, 1)
			assert.InDelta(t, 0.05, found[0].EV, 1e-9)
			assert.Equal(t, key, found[0].Market)
			assert.Equal(t, key.String(), found[0].Legs[0].MarketKey)
		})
	}
}

func TestFindLineGapsOnPropsOfEverySport(t *testing.T) {
	t.Parallel()
	for _, code := range []canon.MarketCode{canon.PropPoints, canon.PropRebounds, canon.PropRushYards, canon.PropDoubleFlt, canon.PropMapKills} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			at := func(line float64) canon.MarketKey {
				return canon.MarketKey{EventID: "ev1", Code: code, PlayerID: "p1", Line: canon.Line(line)}
			}
			offer := func(source string, line, chance float64) LineOffer {
				return LineOffer{Market: at(line), Over: book(source, canon.SideOver, 1/chance), OverChance: chance}
			}
			pick := LineOffer{Market: at(24.5), Over: Quote{Outcome: Outcome{Side: canon.SideOver}, Source: "prizepicks", Price: canon.Price{Form: canon.FormLineOnly, ObservedAt: now}}}
			found := FindLineGaps([]LineOffer{offer("bet9ja", 22.5, 0.5), offer("betking", 22.5, 0.52), offer("betking", 24.5, 0.4), pick}, now)
			require.Len(t, found, 1)
			assert.InDelta(t, 2.0, found[0].Margin, 1e-9)
			assert.Equal(t, at(24.5), found[0].Market)
		})
	}
}

func TestDetectMoveOnPeriodMarket(t *testing.T) {
	t.Parallel()
	key := canon.MarketKey{EventID: "nba", Code: canon.CodeSpread, Line: canon.Line(-1.5), Period: canon.PeriodQuarter1}
	earlier, recent := now.Add(-10*time.Minute), now.Add(-5*time.Minute)
	history := []MovePoint{movePoint(2.0, earlier, earlier), movePoint(1.8, recent, recent)}
	s, ok := DetectMove(key, book("bet9ja", canon.SideHome, 1.8), history, 0, now)
	require.True(t, ok)
	assert.Equal(t, "move|nba|SPREAD|-1.5||Q1|bet9ja|home:|up", s.DedupeKey)
	for _, leg := range s.Legs {
		assert.Equal(t, key.String(), leg.MarketKey)
		assert.InDelta(t, -1.5, *leg.Line, 0)
	}
	assert.Positive(t, s.Change)
}
