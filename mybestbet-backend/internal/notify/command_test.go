package notify

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		text     string
		expected command
		ok       bool
	}{
		{name: "plain command", text: "/arbs 1.0", expected: command{name: "arbs", args: []string{"1.0"}}, ok: true},
		{name: "bot suffix and case", text: "/League@MyBestBetBot epl laliga", expected: command{name: "league", args: []string{"epl", "laliga"}}, ok: true},
		{name: "no args", text: "  /start  ", expected: command{name: "start", args: []string{}}, ok: true},
		{name: "not a command", text: "hello", ok: false},
		{name: "empty", text: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseCommand(tt.text)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestApplyCommand(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	env := commandEnv{now: now, knownLeagues: []string{"EPL", "LALIGA", "SERIEA"}}
	twoHoursLater := now.Add(2 * time.Hour)
	ten := 10.0
	tests := []struct {
		name      string
		text      string
		start     func(*Subscription)
		expect    func(*Subscription)
		wantUsage bool
	}{
		{name: "start keeps defaults", text: "/start", expect: func(*Subscription) {}},
		{name: "arbs threshold", text: "/arbs 1.0", expect: func(s *Subscription) { s.MinArbPercent = 1 }},
		{name: "ev threshold with percent sign", text: "/ev 5%", expect: func(s *Subscription) { s.MinEVPercent = 5 }},
		{name: "ev rejects text", text: "/ev lots", wantUsage: true},
		{name: "arbs rejects missing value", text: "/arbs", wantUsage: true},
		{name: "leagues uppercased and deduplicated", text: "/league epl LALIGA epl", expect: func(s *Subscription) { s.Leagues = []string{"EPL", "LALIGA"} }},
		{name: "league with no codes clears filter", text: "/league", start: func(s *Subscription) { s.Leagues = []string{"EPL"} }, expect: func(s *Subscription) { s.Leagues = []string{} }},
		{name: "unknown league", text: "/league MARS", wantUsage: true},
		{name: "sports lowercased and deduplicated", text: "/sport Basketball tennis basketball", expect: func(s *Subscription) { s.Sports = []string{"basketball", "tennis"} }},
		{name: "american football by canonical name", text: "/sports american-football", expect: func(s *Subscription) { s.Sports = []string{"american-football"} }},
		{name: "sport with no names clears filter", text: "/sport", start: func(s *Subscription) { s.Sports = []string{"esports"} }, expect: func(s *Subscription) { s.Sports = []string{} }},
		{name: "unknown sport", text: "/sport cricket", wantUsage: true},
		{name: "props on", text: "/props on", expect: func(s *Subscription) { s.Props = true }},
		{name: "props invalid", text: "/props maybe", wantUsage: true},
		{name: "move threshold", text: "/move 10", expect: func(s *Subscription) { s.MovePercent = &ten }},
		{name: "move off", text: "/move off", start: func(s *Subscription) { s.MovePercent = &ten }, expect: func(s *Subscription) { s.MovePercent = nil }},
		{name: "mute two hours", text: "/mute 2h", expect: func(s *Subscription) { s.MutedUntil = &twoHoursLater }},
		{name: "mute off", text: "/mute off", start: func(s *Subscription) { s.MutedUntil = &twoHoursLater }, expect: func(s *Subscription) { s.MutedUntil = nil }},
		{name: "mute rejects negative", text: "/mute -2h", wantUsage: true},
		{name: "unknown command shows help", text: "/frobnicate", wantUsage: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			current := defaultSubscription(42)
			if tt.start != nil {
				tt.start(&current)
			}
			cmd, ok := parseCommand(tt.text)
			require.True(t, ok)
			got, reply, err := applyCommand(current, cmd, env)
			if tt.wantUsage {
				var usage usageError
				require.ErrorAs(t, err, &usage)
				assert.Equal(t, current, got)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, reply)
			expected := current
			tt.expect(&expected)
			assert.Equal(t, expected, got)
		})
	}
}
