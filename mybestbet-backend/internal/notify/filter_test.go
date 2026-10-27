package notify

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestSubscriptionWants(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)
	five := 5.0
	arb := func(margin float64) canon.Signal {
		return canon.Signal{Type: canon.SignalArb, Status: canon.SignalOpen, LeagueCode: "EPL", Margin: margin, Market: canon.MarketKey{Code: canon.Code1X2}}
	}
	tests := []struct {
		name     string
		sub      func(*Subscription)
		sport    canon.Sport
		signal   canon.Signal
		expected bool
	}{
		{name: "arb above default threshold", signal: arb(0.006), expected: true},
		{name: "arb below default threshold", signal: arb(0.004), expected: false},
		{name: "arb below raised threshold", sub: func(s *Subscription) { s.MinArbPercent = 1 }, signal: arb(0.009), expected: false},
		{name: "closed signal ignored", signal: canon.Signal{Type: canon.SignalArb, Status: canon.SignalClosed, Margin: 0.05}, expected: false},
		{name: "muted chat", sub: func(s *Subscription) { s.MutedUntil = &later }, signal: arb(0.05), expected: false},
		{name: "mute expired", sub: func(s *Subscription) { s.MutedUntil = &earlier }, signal: arb(0.05), expected: true},
		{name: "league filter excludes", sub: func(s *Subscription) { s.Leagues = []string{"LALIGA"} }, signal: arb(0.05), expected: false},
		{name: "league filter includes", sub: func(s *Subscription) { s.Leagues = []string{"EPL", "LALIGA"} }, signal: arb(0.05), expected: true},
		{name: "ev at threshold", signal: canon.Signal{Type: canon.SignalEV, Status: canon.SignalOpen, EV: 0.03}, expected: true},
		{name: "ev below threshold", signal: canon.Signal{Type: canon.SignalEV, Status: canon.SignalOpen, EV: 0.029}, expected: false},
		{name: "prop needs props on", signal: canon.Signal{Type: canon.SignalEV, Status: canon.SignalOpen, EV: 0.1, Market: canon.MarketKey{Code: canon.PropShots}}, expected: false},
		{name: "prop with props on", sub: func(s *Subscription) { s.Props = true }, signal: canon.Signal{Type: canon.SignalEV, Status: canon.SignalOpen, EV: 0.1, Market: canon.MarketKey{Code: canon.PropShots}}, expected: true},
		{name: "move disabled by default", signal: canon.Signal{Type: canon.SignalMove, Status: canon.SignalOpen, Margin: 0.5}, expected: false},
		{name: "move above threshold", sub: func(s *Subscription) { s.MovePercent = &five }, signal: canon.Signal{Type: canon.SignalMove, Status: canon.SignalOpen, Margin: 0.06}, expected: true},
		{name: "sport filter includes", sub: func(s *Subscription) { s.Sports = []string{"basketball"} }, sport: canon.SportBasketball, signal: arb(0.05), expected: true},
		{name: "sport filter excludes", sub: func(s *Subscription) { s.Sports = []string{"basketball", "tennis"} }, sport: canon.SportFootball, signal: arb(0.05), expected: false},
		{name: "sport filter excludes unknown sport", sub: func(s *Subscription) { s.Sports = []string{"basketball"} }, signal: arb(0.05), expected: false},
		{name: "basketball prop with props on", sub: func(s *Subscription) { s.Props = true }, sport: canon.SportBasketball, signal: canon.Signal{Type: canon.SignalEV, Status: canon.SignalOpen, EV: 0.1, Market: canon.MarketKey{Code: canon.PropPoints}}, expected: true},
		{name: "esports prop needs props on", sport: canon.SportEsports, signal: canon.Signal{Type: canon.SignalEV, Status: canon.SignalOpen, EV: 0.1, Market: canon.MarketKey{Code: canon.PropMapKills}}, expected: false},
		{name: "tennis line gap with props on", sub: func(s *Subscription) { s.Props = true }, sport: canon.SportTennis, signal: canon.Signal{Type: canon.SignalLine, Status: canon.SignalOpen, Margin: 1, Market: canon.MarketKey{Code: canon.PropAces}}, expected: true},
		{name: "line gap only for props", sub: func(s *Subscription) { s.Props = true }, signal: canon.Signal{Type: canon.SignalLine, Status: canon.SignalOpen, Margin: 1, Market: canon.MarketKey{Code: canon.PropShots}}, expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sub := defaultSubscription(1)
			if tt.sub != nil {
				tt.sub(&sub)
			}
			assert.Equal(t, tt.expected, sub.wants(tt.signal, tt.sport, now))
		})
	}
}

func TestSportOf(t *testing.T) {
	t.Parallel()
	leagueSports := map[string]canon.Sport{"NBA": canon.SportBasketball, "EPL": canon.SportFootball}
	tests := []struct {
		name     string
		signal   canon.Signal
		expected canon.Sport
	}{
		{name: "from league", signal: canon.Signal{LeagueCode: "NBA", Market: canon.MarketKey{Code: canon.CodeMoneyline}}, expected: canon.SportBasketball},
		{name: "single sport market without league", signal: canon.Signal{Market: canon.MarketKey{Code: canon.PropAces}}, expected: canon.SportTennis},
		{name: "multi sport market without league", signal: canon.Signal{Market: canon.MarketKey{Code: canon.CodeMoneyline}}, expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, sportOf(tt.signal, leagueSports))
		})
	}
}
