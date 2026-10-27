package server

import (
	"context"
	"net/http"
	"time"

	"mybestbet/pkg/canon"
)

const (
	recentKickoffGrace  = 2 * time.Hour
	defaultEventWindow  = 7 * 24 * time.Hour
	maxEventWindow      = 31 * 24 * time.Hour
	defaultCalendarSpan = 31 * 24 * time.Hour
	maxCalendarSpan     = 92 * 24 * time.Hour
)

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	from, to, err := kickoffWindow(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	sport, err := sportQuery(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	market, err := marketSelectorQuery(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	events, err := s.indexedEventsBetween(r.Context(), eventFilter{league: r.URL.Query().Get("league"), leagueCodes: s.leagueCodesOf(sport), from: from, to: to})
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	summaries, err := s.summarize(r.Context(), events, market)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, listOf(summaries), eventsMaxAge)
}

func kickoffWindow(r *http.Request) (time.Time, time.Time, error) {
	return timeWindowQuery(r, time.Now().Add(-recentKickoffGrace), defaultEventWindow, maxEventWindow)
}

func (s *Server) eventCalendar(w http.ResponseWriter, r *http.Request) {
	loc, err := locationQuery(r, "tz")
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	sport, err := sportQuery(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	year, month, day := time.Now().In(loc).Date()
	from, to, err := timeWindowQuery(r, time.Date(year, month, day, 0, 0, 0, 0, loc), defaultCalendarSpan, maxCalendarSpan)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	days, err := s.indexedDayCounts(r.Context(), eventFilter{league: r.URL.Query().Get("league"), leagueCodes: s.leagueCodesOf(sport), from: from, to: to}, loc)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, EventCalendar{Days: emptyIfNil(days)}, calendarMaxAge)
}

func (s *Server) summarize(ctx context.Context, events []EventInfo, market marketSelector) ([]EventSummary, error) {
	ids := make([]string, len(events))
	marketKeys := make([]canon.MarketKey, len(events))
	kickoffs := make([]time.Time, len(events))
	for i, e := range events {
		ids[i] = e.ID
		marketKeys[i] = market.keyFor(e.ID, e.Sport)
		kickoffs[i] = e.Kickoff
	}
	coverage, err := s.coverageByEvent(ctx, ids)
	if err != nil {
		return nil, err
	}
	badges, err := s.cachedBadges(ctx, ids)
	if err != nil {
		return nil, err
	}
	bests, err := s.bestOutcomesOf(ctx, marketKeys, kickoffs)
	if err != nil {
		return nil, err
	}
	summaries := make([]EventSummary, len(events))
	for i, e := range events {
		summaries[i] = EventSummary{
			EventInfo: e,
			Sources:   emptyIfNil(coverage[e.ID].sources),
			PropCount: coverage[e.ID].propCount,
			Best:      bests[i],
			Badges:    badges[e.ID],
		}
	}
	return summaries, nil
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	event, cells, err := s.eventWithBoard(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, eventDetail(event, cells), eventMaxAge)
}

func (s *Server) eventWithBoard(r *http.Request) (EventInfo, []canon.BoardCell, error) {
	id, err := uuidPathValue(r, "id")
	if err != nil {
		return EventInfo{}, nil, err
	}
	return s.eventBoard(r.Context(), id)
}

func (s *Server) eventBoard(ctx context.Context, id string) (EventInfo, []canon.BoardCell, error) {
	s.priority.touch(id)
	event, err := s.indexedEvent(ctx, id)
	if err != nil {
		return EventInfo{}, nil, err
	}
	cells, err := s.eventBoardCells(ctx, id)
	return event, pricedSinceKickoff(cells, event.Kickoff, time.Now()), err
}
