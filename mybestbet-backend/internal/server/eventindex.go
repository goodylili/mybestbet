package server

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	eventIndexRefresh   = 15 * time.Second
	eventIndexLookback  = 26 * time.Hour
	eventIndexLookahead = maxEventWindow + 7*24*time.Hour
)

type eventIndex struct {
	from   time.Time
	to     time.Time
	events []EventInfo
	byID   map[string]EventInfo
}

func (idx *eventIndex) covers(from, to time.Time) bool {
	return idx != nil && !from.Before(idx.from) && !to.After(idx.to)
}

func (idx *eventIndex) between(filter eventFilter, limit int) []EventInfo {
	start, _ := slices.BinarySearchFunc(idx.events, filter.from, func(e EventInfo, t time.Time) int { return e.Kickoff.Compare(t) })
	var events []EventInfo
	for _, e := range idx.events[start:] {
		if !e.Kickoff.Before(filter.to) || len(events) == limit {
			break
		}
		if filter.league != "" && e.LeagueCode != filter.league {
			continue
		}
		if filter.leagueCodes != nil && !slices.Contains(filter.leagueCodes, e.LeagueCode) {
			continue
		}
		events = append(events, e)
	}
	return events
}

func (s *Server) refreshEventIndex(ctx context.Context) error {
	now := time.Now()
	index := &eventIndex{from: now.Add(-eventIndexLookback), to: now.Add(eventIndexLookahead)}
	rows, err := s.pool.Query(ctx, `SELECT `+eventInfoColumns+` `+eventInfoJoins+` WHERE e.kickoff >= $1 AND e.kickoff < $2 AND e."mergedInto" IS NULL
		ORDER BY e.kickoff, e.id`, index.from, index.to)
	if err != nil {
		return fmt.Errorf("index events: %w", err)
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (EventInfo, error) { return scanEventInfo(row) })
	if err != nil {
		return fmt.Errorf("scan indexed events: %w", err)
	}
	if err := s.attachEventLogos(ctx, events); err != nil {
		return err
	}
	if err := s.attachEventLinks(ctx, s.withSports(events)); err != nil {
		return err
	}
	index.events = events
	index.byID = make(map[string]EventInfo, len(events))
	keys := make([]string, len(events))
	for i, e := range index.events {
		index.byID[e.ID] = e
		keys[i] = marketSelector{}.keyFor(e.ID, e.Sport).String()
	}
	s.events.set(index)
	leagues, err := s.loadLeagues(ctx, index.fixtureCounts(now.Add(-recentKickoffGrace)))
	if err != nil {
		return err
	}
	s.leagues.set(leagues)
	_, err = s.boardCells(ctx, keys)
	return err
}

func (idx *eventIndex) fixtureCounts(since time.Time) map[string]int {
	counts := map[string]int{}
	for _, e := range idx.events {
		if !e.Kickoff.Before(since) && (e.Status == "scheduled" || e.Status == "live") {
			counts[e.LeagueCode]++
		}
	}
	return counts
}

func (idx *eventIndex) dayCounts(filter eventFilter, loc *time.Location) []CalendarDay {
	var days []CalendarDay
	for _, e := range idx.between(eventFilter{league: filter.league, leagueCodes: filter.leagueCodes, from: filter.from, to: filter.to}, -1) {
		day := e.Kickoff.In(loc).Format(time.DateOnly)
		if n := len(days); n > 0 && days[n-1].Date == day {
			days[n-1].Count++
			continue
		}
		days = append(days, CalendarDay{Date: day, Count: 1})
	}
	return days
}

func (s *Server) indexedEventsBetween(ctx context.Context, filter eventFilter) ([]EventInfo, error) {
	if index, _ := s.events.get(); index.covers(filter.from, filter.to) {
		return index.between(filter, maxEventsListed), nil
	}
	return s.eventsBetween(ctx, filter)
}

func (s *Server) indexedDayCounts(ctx context.Context, filter eventFilter, loc *time.Location) ([]CalendarDay, error) {
	if index, _ := s.events.get(); index.covers(filter.from, filter.to) {
		return index.dayCounts(filter, loc), nil
	}
	return s.eventDayCounts(ctx, filter, loc)
}

func (s *Server) indexedEvent(ctx context.Context, id string) (EventInfo, error) {
	if index, _ := s.events.get(); index != nil {
		if event, ok := index.byID[id]; ok {
			return event, nil
		}
	}
	return s.eventByID(ctx, id)
}
