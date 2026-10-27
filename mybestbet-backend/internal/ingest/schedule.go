package ingest

import (
	"cmp"
	"math"
	"slices"
	"sync"
	"time"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

type scheduledEvent struct {
	kickoff  time.Time
	nextPoll time.Time
}

type schedule struct {
	mu     sync.Mutex
	events map[string]*scheduledEvent
}

func newSchedule() *schedule { return &schedule{events: map[string]*scheduledEvent{}} }

func (s *schedule) replace(events []canon.SourceEvent) (idsChanged bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]*scheduledEvent, len(events))
	for _, ev := range events {
		entry := &scheduledEvent{kickoff: ev.Kickoff}
		if previous, known := s.events[ev.SourceID]; known {
			entry.nextPoll = previous.nextPoll
		}
		next[ev.SourceID] = entry
	}
	idsChanged = len(next) != len(s.events)
	for id := range next {
		if _, known := s.events[id]; !known {
			idsChanged = true
		}
	}
	s.events = next
	return idsChanged
}

func (s *schedule) ids() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.events))
	for id := range s.events {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (s *schedule) eventsPerTier(tiers []config.Tier, now time.Time) []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := make([]int, len(tiers))
	for _, ev := range s.events {
		if tier := tierFor(tiers, ev.kickoff.Sub(now)); tier != noTier {
			counts[tier]++
		}
	}
	return counts
}

func (s *schedule) takeDue(now time.Time, tiers []config.Tier, intervals []time.Duration, unitRandom func() float64, coolBudget int, isFocused func(string) bool) [][]string {
	return s.take(now, tiers, intervals, unitRandom, true, coolBudget, isFocused)
}

func (s *schedule) takeHot(now time.Time, tiers []config.Tier, intervals []time.Duration, unitRandom func() float64, isFocused func(string) bool) []string {
	return s.take(now, tiers, intervals, unitRandom, true, 0, isFocused)[hotTier]
}

func (s *schedule) takeCool(now time.Time, tiers []config.Tier, intervals []time.Duration, unitRandom func() float64, budget int, isFocused func(string) bool) [][]string {
	return s.take(now, tiers, intervals, unitRandom, false, budget, isFocused)
}

func (s *schedule) take(now time.Time, tiers []config.Tier, intervals []time.Duration, unitRandom func() float64, includeHot bool, coolBudget int, isFocused func(string) bool) [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	due := s.dueByTier(now, tiers, isFocused)
	if !includeHot {
		due[hotTier] = nil
	}
	s.budgetCool(due, now, intervals, coolBudget)
	for tier, ids := range due {
		for _, id := range ids {
			s.events[id].nextPoll = now.Add(jittered(intervals[tier], tiers[tier].Jitter, unitRandom()))
		}
		slices.Sort(ids)
		due[tier] = ids
	}
	return due
}

func (s *schedule) dueByTier(now time.Time, tiers []config.Tier, isFocused func(string) bool) [][]string {
	due := make([][]string, len(tiers))
	for id, ev := range s.events {
		untilKickoff := ev.kickoff.Sub(now)
		tier := tierFor(tiers, untilKickoff)
		if untilKickoff >= 0 && isFocused != nil && isFocused(id) {
			tier = hotTier
			if soonest := now.Add(tiers[hotTier].Interval); ev.nextPoll.After(soonest) {
				ev.nextPoll = soonest
			}
		}
		if tier == noTier || now.Before(ev.nextPoll) {
			continue
		}
		due[tier] = append(due[tier], id)
	}
	for _, ids := range due {
		slices.SortFunc(ids, func(a, b string) int {
			return cmp.Or(s.events[a].nextPoll.Compare(s.events[b].nextPoll), cmp.Compare(a, b))
		})
	}
	return due
}

type coolCandidate struct {
	id      string
	tier    int
	urgency float64
}

func (s *schedule) budgetCool(due [][]string, now time.Time, intervals []time.Duration, budget int) {
	var candidates []coolCandidate
	for tier := hotTier + 1; tier < len(due); tier++ {
		for _, id := range due[tier] {
			candidates = append(candidates, coolCandidate{id: id, tier: tier, urgency: s.urgency(id, now, intervals[tier])})
		}
		due[tier] = nil
	}
	slices.SortFunc(candidates, func(a, b coolCandidate) int {
		return cmp.Or(cmp.Compare(b.urgency, a.urgency), cmp.Compare(a.tier, b.tier), cmp.Compare(a.id, b.id))
	})
	for _, c := range candidates[:min(len(candidates), budget)] {
		due[c.tier] = append(due[c.tier], c.id)
	}
}

func (s *schedule) urgency(id string, now time.Time, interval time.Duration) float64 {
	ev := s.events[id]
	if ev.nextPoll.IsZero() || interval <= 0 {
		return math.Inf(1)
	}
	return float64(now.Sub(ev.nextPoll)) / float64(interval)
}

func (s *schedule) pollAllNow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range s.events {
		ev.nextPoll = time.Time{}
	}
}
