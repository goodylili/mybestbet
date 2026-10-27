package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestEventIndexBetween(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	event := func(id, league string, offset time.Duration) EventInfo {
		return EventInfo{ID: id, LeagueCode: league, Kickoff: start.Add(offset)}
	}
	index := &eventIndex{from: start, to: start.Add(48 * time.Hour), events: []EventInfo{
		event("a", "EPL", 0), event("b", "NBA", time.Hour), event("c", "EPL", 2*time.Hour), event("d", "EPL", 30*time.Hour),
	}}
	ids := func(events []EventInfo) []string {
		out := []string{}
		for _, e := range events {
			out = append(out, e.ID)
		}
		return out
	}
	assert.Equal(t, []string{"b", "c"}, ids(index.between(eventFilter{from: start.Add(time.Minute), to: start.Add(24 * time.Hour)}, -1)))
	assert.Equal(t, []string{"a", "c"}, ids(index.between(eventFilter{league: "EPL", from: start, to: start.Add(48 * time.Hour)}, 2)))
	assert.Equal(t, []string{"b"}, ids(index.between(eventFilter{leagueCodes: []string{"NBA"}, from: start, to: start.Add(48 * time.Hour)}, -1)))
	assert.True(t, index.covers(start, start.Add(time.Hour)))
	assert.False(t, index.covers(start.Add(-time.Minute), start.Add(time.Hour)))
	assert.False(t, (*eventIndex)(nil).covers(start, start))
}

func TestSignalSnapshotFiltersAndBadges(t *testing.T) {
	t.Parallel()
	view := func(id, event, league string, kind canon.SignalType, margin, ev float64) SignalView {
		return SignalView{Signal: canon.Signal{ID: id, EventID: event, LeagueCode: league, Type: kind, Margin: margin, EV: ev}}
	}
	signals := []SignalView{
		view("1", "e1", "EPL", canon.SignalArb, 0.02, 0),
		view("2", "e1", "EPL", canon.SignalArb, 0.05, 0),
		view("3", "e1", "EPL", canon.SignalEV, 0, 0.04),
		view("4", "e2", "NBA", canon.SignalMove, 0, 0),
	}
	assert.Equal(t, map[string]Badges{"e1": {Arbs: 2, EVs: 1, MaxMargin: 0.05, MaxEV: 0.04}}, badgesOf(signals))
	assert.True(t, signalFilter{signalType: "arb", minimum: 0.03}.matches(signals[1]))
	assert.False(t, signalFilter{signalType: "arb", minimum: 0.03}.matches(signals[0]))
	assert.False(t, signalFilter{signalType: "ev"}.matches(signals[0]))
	assert.False(t, signalFilter{leagueCodes: []string{"NBA"}}.matches(signals[0]))
	assert.True(t, signalFilter{leagueCodes: []string{"NBA"}}.matches(signals[3]))
}

func TestEventIndexCounts(t *testing.T) {
	t.Parallel()
	lagos, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 27, 22, 30, 0, 0, time.UTC)
	index := &eventIndex{from: start.Add(-time.Hour), to: start.Add(72 * time.Hour), events: []EventInfo{
		{ID: "old", LeagueCode: "EPL", Kickoff: start.Add(-30 * time.Minute), Status: "scheduled"},
		{ID: "a", LeagueCode: "EPL", Kickoff: start, Status: "live"},
		{ID: "b", LeagueCode: "EPL", Kickoff: start.Add(time.Hour), Status: "finished"},
		{ID: "c", LeagueCode: "NBA", Kickoff: start.Add(2 * time.Hour), Status: "scheduled"},
	}}
	assert.Equal(t, map[string]int{"EPL": 1, "NBA": 1}, index.fixtureCounts(start))
	assert.Equal(t, []CalendarDay{{Date: "2026-10-27", Count: 2}, {Date: "2026-10-28", Count: 1}},
		index.dayCounts(eventFilter{league: "EPL", from: start.Add(-time.Hour), to: start.Add(48 * time.Hour)}, lagos))
	assert.Equal(t, []CalendarDay{{Date: "2026-10-27", Count: 2}, {Date: "2026-10-28", Count: 2}},
		index.dayCounts(eventFilter{from: start.Add(-time.Hour), to: start.Add(48 * time.Hour)}, lagos))
}
