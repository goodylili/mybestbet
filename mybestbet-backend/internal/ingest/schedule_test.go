package ingest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestScheduleTakesDueEventsPerTier(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s := newSchedule()
	changed := s.replace([]canon.SourceEvent{
		{SourceID: "hot", Kickoff: now.Add(time.Hour)},
		{SourceID: "warm", Kickoff: now.Add(5 * time.Hour)},
		{SourceID: "cold", Kickoff: now.Add(72 * time.Hour)},
		{SourceID: "started", Kickoff: now.Add(-time.Minute)},
	})
	assert.True(t, changed)
	assert.Equal(t, []int{1, 1, 1}, s.eventsPerTier(testTiers, now))

	midpoint := func() float64 { return 0.5 }
	intervals := baseIntervals(testTiers)
	assert.Equal(t, [][]string{{"hot"}, {"warm"}, {"cold"}}, s.takeDue(now, testTiers, intervals, midpoint, coolChunk, nil))
	assert.Equal(t, [][]string{nil, nil, nil}, s.takeDue(now.Add(4*time.Second), testTiers, intervals, midpoint, coolChunk, nil))
	assert.Equal(t, [][]string{{"hot"}, nil, nil}, s.takeDue(now.Add(5*time.Second), testTiers, intervals, midpoint, coolChunk, nil))

	s.pollAllNow()
	assert.Equal(t, [][]string{{"hot"}, {"warm"}, {"cold"}}, s.takeDue(now.Add(6*time.Second), testTiers, intervals, midpoint, coolChunk, nil))
}

func TestScheduleBudgetsCoolTiersPerTake(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s := newSchedule()
	s.replace([]canon.SourceEvent{
		{SourceID: "hot-a", Kickoff: now.Add(time.Hour)},
		{SourceID: "hot-b", Kickoff: now.Add(time.Hour)},
		{SourceID: "warm", Kickoff: now.Add(5 * time.Hour)},
		{SourceID: "cold-a", Kickoff: now.Add(72 * time.Hour)},
		{SourceID: "cold-b", Kickoff: now.Add(96 * time.Hour)},
	})
	midpoint := func() float64 { return 0.5 }
	intervals := baseIntervals(testTiers)

	assert.Equal(t, [][]string{{"hot-a", "hot-b"}, {"warm"}, {"cold-a"}}, s.takeDue(now, testTiers, intervals, midpoint, 2, nil))
	assert.Equal(t, [][]string{nil, nil, {"cold-b"}}, s.takeDue(now, testTiers, intervals, midpoint, 2, nil))
	assert.Equal(t, [][]string{nil, nil, nil}, s.takeDue(now, testTiers, intervals, midpoint, 2, nil))
}

func TestScheduleReplaceKeepsPollStateAndReportsChanges(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s := newSchedule()
	events := []canon.SourceEvent{{SourceID: "a", Kickoff: now.Add(time.Hour)}}
	s.replace(events)
	s.takeDue(now, testTiers, baseIntervals(testTiers), func() float64 { return 0.5 }, coolChunk, nil)

	assert.False(t, s.replace(events))
	assert.Equal(t, [][]string{nil, nil, nil}, s.takeDue(now.Add(time.Second), testTiers, baseIntervals(testTiers), func() float64 { return 0.5 }, coolChunk, nil))
	assert.True(t, s.replace(append(events, canon.SourceEvent{SourceID: "b", Kickoff: now.Add(time.Hour)})))
	assert.Equal(t, []string{"a", "b"}, s.ids())
}

func TestDiscoveryTimeoutScalesWithLeagues(t *testing.T) {
	assert.Equal(t, 2*time.Minute, discoveryTimeoutFor(10, 10*time.Minute))
	assert.Equal(t, 216*time.Second, discoveryTimeoutFor(72, 10*time.Minute))
	assert.Equal(t, 3*time.Minute, discoveryTimeoutFor(72, 3*time.Minute))
	assert.Equal(t, 2*time.Minute, discoveryTimeoutFor(72, time.Minute))
}

func TestScheduleTreatsFocusedEventsAsHot(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s := newSchedule()
	s.replace([]canon.SourceEvent{
		{SourceID: "cold", Kickoff: now.Add(72 * time.Hour)},
		{SourceID: "watched", Kickoff: now.Add(96 * time.Hour)},
		{SourceID: "started", Kickoff: now.Add(-time.Minute)},
	})
	midpoint := func() float64 { return 0.5 }
	intervals := baseIntervals(testTiers)
	watched := func(id string) bool { return id == "watched" || id == "started" }

	assert.Equal(t, [][]string{{"watched"}, nil, {"cold"}}, s.takeDue(now, testTiers, intervals, midpoint, coolChunk, watched))
	assert.Equal(t, [][]string{nil, nil, nil}, s.takeDue(now.Add(4*time.Second), testTiers, intervals, midpoint, coolChunk, watched))
	assert.Equal(t, [][]string{{"watched"}, nil, nil}, s.takeDue(now.Add(5*time.Second), testTiers, intervals, midpoint, coolChunk, watched))
}

func TestScheduleFocusPullsAColdPollForward(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s := newSchedule()
	s.replace([]canon.SourceEvent{{SourceID: "cold", Kickoff: now.Add(72 * time.Hour)}})
	midpoint := func() float64 { return 0.5 }
	intervals := baseIntervals(testTiers)
	s.takeDue(now, testTiers, intervals, midpoint, coolChunk, nil)

	focused := func(string) bool { return true }
	assert.Equal(t, [][]string{nil, nil, nil}, s.takeDue(now.Add(time.Second), testTiers, intervals, midpoint, coolChunk, focused))
	assert.Equal(t, [][]string{{"cold"}, nil, nil}, s.takeDue(now.Add(6*time.Second), testTiers, intervals, midpoint, coolChunk, focused))
}

func TestScheduleLanesTakeOnlyTheirOwnTiers(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s := newSchedule()
	s.replace([]canon.SourceEvent{
		{SourceID: "hot", Kickoff: now.Add(time.Hour)},
		{SourceID: "warm", Kickoff: now.Add(5 * time.Hour)},
		{SourceID: "cold", Kickoff: now.Add(72 * time.Hour)},
	})
	midpoint := func() float64 { return 0.5 }
	intervals := baseIntervals(testTiers)

	assert.Equal(t, [][]string{nil, {"warm"}, {"cold"}}, s.takeCool(now, testTiers, intervals, midpoint, coolChunk, nil))
	assert.Equal(t, []string{"hot"}, s.takeHot(now, testTiers, intervals, midpoint, nil))
	assert.Empty(t, s.takeHot(now, testTiers, intervals, midpoint, nil))
	assert.Equal(t, [][]string{nil, nil, nil}, s.takeCool(now, testTiers, intervals, midpoint, coolChunk, nil))
}
