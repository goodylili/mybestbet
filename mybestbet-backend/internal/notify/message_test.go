package notify

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestFormatAlertArb(t *testing.T) {
	t.Parallel()
	signal := canon.Signal{
		Type:       canon.SignalArb,
		EventID:    "ev-1",
		LeagueCode: "EPL",
		Margin:     0.0213,
		Market:     canon.MarketKey{Code: canon.CodeOU, Line: canon.Line(2.5)},
		Legs: []canon.Leg{
			{Source: "bet9ja", Side: canon.SideOver, Price: canon.NewPrice(2.1, canon.FormDecimal, time.Time{}), DeepLink: "https://bet9ja.example/e?a=1&b=2"},
			{Source: "kalshi", Side: canon.SideUnder, Price: canon.NewPrice(45, canon.FormCents, time.Time{})},
		},
	}
	details := alertContext{Home: "Brighton & Hove", Away: "Chelsea", Kickoff: time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC)}
	got := formatAlert(signal, canon.SportFootball, details, "https://odds.example/")
	assert.Contains(t, got, "<b>ARB 2.13%</b> · EPL · football\n")
	assert.Contains(t, got, "Brighton &amp; Hove vs Chelsea")
	assert.Contains(t, got, "Market: OU 2.5 (full game)")
	assert.Contains(t, got, "• over @ bet9ja 2.10 → stake 51.41")
	assert.Contains(t, got, "• under @ kalshi 45¢ (2.22) → stake 48.59")
	assert.Contains(t, got, `href="https://bet9ja.example/e?a=1&amp;b=2"`)
	assert.Contains(t, got, `href="https://odds.example/event/ev-1"`)
}

func TestArbStakesPreferEngineStakes(t *testing.T) {
	t.Parallel()
	signal := canon.Signal{Type: canon.SignalArb, Legs: []canon.Leg{{Stake: 60, Price: canon.Price{Prob: 0.5}}, {Stake: 40, Price: canon.Price{Prob: 0.4}}}}
	assert.Equal(t, []float64{60, 40}, arbStakes(signal))
}

func TestFormatAlertEV(t *testing.T) {
	t.Parallel()
	signal := canon.Signal{Type: canon.SignalEV, EV: 0.045, FairProb: 0.52, Market: canon.MarketKey{Code: canon.PropShots, Line: canon.Line(1.5), PlayerID: "p"},
		Legs: []canon.Leg{{Source: "betano", Side: canon.SideOver, Price: canon.NewPrice(2.01, canon.FormDecimal, time.Time{})}}}
	got := formatAlert(signal, "", alertContext{Home: "A", Away: "B", PlayerName: "Bukayo Saka"}, "")
	assert.Contains(t, got, "+EV 4.50% (fair 52.00%)")
	assert.Contains(t, got, "Market: P_SHOTS 1.5 (full game) · Bukayo Saka")
	assert.NotContains(t, got, "stake")
	assert.NotContains(t, got, "Open board")
}

func TestFormatAlertShowsSportAndPeriod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		league   string
		sport    canon.Sport
		market   canon.MarketKey
		header   string
		expected string
	}{
		{name: "basketball first quarter spread", league: "NBA", sport: canon.SportBasketball, market: canon.MarketKey{Code: canon.CodeSpread, Line: canon.Line(-1.5), Period: canon.PeriodQuarter1}, header: " · NBA · basketball\n", expected: "Market: SPREAD -1.5 (1st quarter)"},
		{name: "tennis first set winner", league: "ATP", sport: canon.SportTennis, market: canon.MarketKey{Code: canon.CodeMoneyline, Period: canon.PeriodFirstSet}, header: " · ATP · tennis\n", expected: "Market: ML (1st set)"},
		{name: "esports map one", league: "CS2", sport: canon.SportEsports, market: canon.MarketKey{Code: canon.CodeMoneyline, Period: canon.PeriodFirstMap}, header: " · CS2 · esports\n", expected: "Market: ML (map 1)"},
		{name: "nfl full game total", league: "NFL", sport: canon.SportAmericanFootball, market: canon.MarketKey{Code: canon.CodeTotal, Line: canon.Line(44.5)}, header: " · NFL · american-football\n", expected: "Market: TOTAL 44.5 (full game)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			signal := canon.Signal{Type: canon.SignalArb, LeagueCode: tt.league, Margin: 0.01, Market: tt.market}
			got := formatAlert(signal, tt.sport, alertContext{Home: "A", Away: "B"}, "")
			assert.Contains(t, got, "<b>ARB 1.00%</b>"+tt.header)
			assert.Contains(t, got, tt.expected)
		})
	}
}
