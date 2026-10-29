package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"

	"mybestbet/internal/auth"
	"mybestbet/pkg/canon"
)

const (
	coalesceWindow    = 250 * time.Millisecond
	heartbeatInterval = 15 * time.Second
	clientRetryMillis = 2000
	boardStreamScope  = "board"
	maxStreamsPerUser = 16
	guestStreamPrefix = "guest:"
)

var (
	errClientTooSlow = errors.New("client too slow, buffer overflowed")
	errSessionEnded  = errors.New("session revoked")
)

type pendingUpdate struct {
	eventID string
	cells   map[string]canon.BoardCell
	signals map[string]canon.Signal
	score   *canon.EventScore
}

func newPendingUpdate(eventID string) *pendingUpdate {
	return &pendingUpdate{eventID: eventID, cells: map[string]canon.BoardCell{}, signals: map[string]canon.Signal{}}
}

func (p *pendingUpdate) merge(update canon.BoardUpdate) {
	for _, cell := range update.Cells {
		p.cells[cell.MarketKey+"|"+cell.Source+"|"+string(cell.Side)+"|"+cell.Label] = cell
	}
	for _, signal := range update.Signals {
		p.signals[signal.ID] = signal
	}
	if update.Score != nil {
		p.score = update.Score
	}
}

func (p *pendingUpdate) isEmpty() bool {
	return len(p.cells) == 0 && len(p.signals) == 0 && p.score == nil
}

func (p *pendingUpdate) drain(now time.Time) canon.BoardUpdate {
	update := canon.BoardUpdate{EventID: p.eventID, Score: p.score, SentAt: now}
	p.score = nil
	for _, cell := range p.cells {
		update.Cells = append(update.Cells, cell)
	}
	for _, signal := range p.signals {
		update.Signals = append(update.Signals, signal)
	}
	slices.SortFunc(update.Cells, func(a, b canon.BoardCell) int { return cmp.Compare(a.MarketKey+a.Source, b.MarketKey+b.Source) })
	slices.SortFunc(update.Signals, func(a, b canon.Signal) int { return cmp.Compare(a.ID, b.ID) })
	clear(p.cells)
	clear(p.signals)
	return update
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	principal, signedIn := auth.PrincipalFrom(r.Context())
	holder := guestStreamPrefix + s.client(r).IP
	if signedIn {
		holder = principal.UserID
	}
	release, ok := s.streams.acquire(holder)
	if !ok {
		s.writeFailure(w, r, &auth.RateLimitError{RetryAfter: clientRetryMillis * time.Millisecond})
		return
	}
	defer release()
	if signedIn {
		ctx, cancel := context.WithDeadline(r.Context(), principal.ExpiresAt)
		defer cancel()
		r = r.WithContext(ctx)
	}
	scope, err := enumQuery(r, "scope", boardStreamScope)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	if scope == boardStreamScope {
		s.streamBoard(w, r)
		return
	}
	eventID := r.URL.Query().Get("event")
	if eventID != "" && uuid.Validate(eventID) != nil {
		s.writeFailure(w, r, invalidInput("event", "must be a uuid"))
		return
	}
	sub, err := s.hub.subscribe(r.Context(), eventID)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	defer s.hub.unsubscribe(sub)
	var snapshot *EventDetail
	if eventID != "" {
		event, cells, err := s.eventBoard(r.Context(), eventID)
		if err != nil {
			s.writeFailure(w, r, err)
			return
		}
		detail := eventDetail(event, cells)
		snapshot = &detail
	}
	if err := s.pumpEvents(r.Context(), openEventStream(w), sub, snapshot); err != nil {
		s.log.DebugContext(r.Context(), "stream closed", "event_id", eventID, "err", err)
	}
}

func (s *Server) streamBoard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("event") != "" {
		s.writeFailure(w, r, invalidInput("event", "cannot be combined with scope=board"))
		return
	}
	sport, err := sportQuery(r)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	sub, err := s.hub.subscribeBoard(r.Context(), boardScope{league: r.URL.Query().Get("league"), leagues: s.leagueCodesOf(sport)})
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	defer s.hub.unsubscribeBoard(sub)
	if err := s.pumpBoard(r.Context(), openEventStream(w), sub); err != nil {
		s.log.DebugContext(r.Context(), "board stream closed", "err", err)
	}
}

type eventStream struct {
	w       http.ResponseWriter
	flusher *http.ResponseController
}

func openEventStream(w http.ResponseWriter) eventStream {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return eventStream{w: w, flusher: http.NewResponseController(w)}
}

func (es eventStream) raw(text string) error {
	if _, err := io.WriteString(es.w, text); err != nil {
		return err
	}
	return es.flusher.Flush()
}

func (es eventStream) connected() error {
	return es.raw(fmt.Sprintf("retry: %d\n: connected\n\n", clientRetryMillis))
}

func (es eventStream) heartbeat() error { return es.raw(": ping\n\n") }

func (s *Server) beat(ctx context.Context, es eventStream) error {
	if principal, ok := auth.PrincipalFrom(ctx); ok {
		if s.auth == nil {
			return errSessionEnded
		}
		active, err := s.auth.SessionActive(ctx, principal.SessionID)
		if err != nil {
			return fmt.Errorf("check stream session: %w", err)
		}
		if !active {
			return errSessionEnded
		}
	}
	return es.heartbeat()
}

func sendEvents[T any](es eventStream, name string, payloads ...T) error {
	var frames strings.Builder
	for _, payload := range payloads {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode %s: %w", name, err)
		}
		fmt.Fprintf(&frames, "event: %s\ndata: %s\n\n", name, encoded)
	}
	return es.raw(frames.String())
}

func (s *Server) pumpEvents(ctx context.Context, es eventStream, sub *subscriber, snapshot *EventDetail) error {
	if err := es.connected(); err != nil {
		return err
	}
	if snapshot != nil {
		if err := sendEvents(es, "snapshot", snapshot); err != nil {
			return err
		}
	}
	coalesce := time.NewTicker(coalesceWindow)
	defer coalesce.Stop()
	heartbeat := time.NewTicker(cmp.Or(s.heartbeat, heartbeatInterval))
	defer heartbeat.Stop()
	pending := newPendingUpdate(sub.eventID)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.hub.stopped:
			return nil
		case <-sub.lagging:
			return errClientTooSlow
		case update := <-sub.items:
			pending.merge(update)
		case now := <-coalesce.C:
			if pending.isEmpty() {
				continue
			}
			if err := sendEvents(es, "update", pending.drain(now)); err != nil {
				return err
			}
		case <-heartbeat.C:
			if sub.eventID != "" {
				s.priority.touch(sub.eventID)
			}
			if err := s.beat(ctx, es); err != nil {
				return err
			}
		}
	}
}

func (s *Server) pumpBoard(ctx context.Context, es eventStream, sub *boardSubscriber) error {
	if err := es.connected(); err != nil {
		return err
	}
	heartbeat := time.NewTicker(cmp.Or(s.heartbeat, heartbeatInterval))
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.hub.stopped:
			return nil
		case <-sub.lagging:
			return errClientTooSlow
		case updates := <-sub.items:
			if err := sendEvents(es, "best", updates...); err != nil {
				return err
			}
		case <-heartbeat.C:
			if err := s.beat(ctx, es); err != nil {
				return err
			}
		}
	}
}

type streamQuota struct {
	mu     sync.Mutex
	active map[string]int
}

func (q *streamQuota) acquire(userID string) (func(), bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active[userID] >= maxStreamsPerUser {
		return nil, false
	}
	if q.active == nil {
		q.active = map[string]int{}
	}
	q.active[userID]++
	return sync.OnceFunc(func() { q.release(userID) }), true
}

func (q *streamQuota) release(userID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active[userID]--; q.active[userID] <= 0 {
		delete(q.active, userID)
	}
}
