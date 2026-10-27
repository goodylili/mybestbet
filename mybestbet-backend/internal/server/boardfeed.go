package server

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
)

const (
	boardFlushTimeout     = 2 * time.Second
	maxCachedEventLeagues = 50_000
)

var mainMarketMarkers = []string{mainMarketMarker(canon.Code1X2), mainMarketMarker(canon.CodeMoneyline)}

func mainMarketMarker(code canon.MarketCode) string {
	return canon.MarketKey{Code: code}.String() + `"`
}

func touchesMainMarket(payload string) bool {
	return slices.ContainsFunc(mainMarketMarkers, func(marker string) bool { return strings.Contains(payload, marker) })
}

type boardChanges struct {
	mu        sync.Mutex
	moved     map[string]struct{}
	published map[string]EventBest
}

func newBoardChanges() *boardChanges {
	return &boardChanges{moved: map[string]struct{}{}, published: map[string]EventBest{}}
}

func (c *boardChanges) markMoved(eventID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.moved[eventID] = struct{}{}
}

func (c *boardChanges) merge(update EventBest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published[update.EventID] = mergeEventBest(c.published[update.EventID], update)
}

func mergeEventBest(current, update EventBest) EventBest {
	current.EventID = update.EventID
	if update.Best != nil {
		current.Best = update.Best
	}
	if update.Score != nil {
		current.Score = update.Score
	}
	return current
}

func (c *boardChanges) take() ([]string, map[string]EventBest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	moved := slices.Collect(maps.Keys(c.moved))
	published := c.published
	clear(c.moved)
	c.published = map[string]EventBest{}
	return moved, published
}

type boardScope struct {
	league  string
	leagues []string
}

func (b boardScope) includes(league string) bool {
	return league != "" && (b.league == "" || b.league == league) && (b.leagues == nil || slices.Contains(b.leagues, league))
}

type boardSubscriber struct {
	mailbox[[]EventBest]
	scope boardScope
}

func (h *streamHub) subscribeBoard(ctx context.Context, scope boardScope) (*boardSubscriber, error) {
	if err := h.awaitReady(ctx); err != nil {
		return nil, err
	}
	sub := &boardSubscriber{scope: scope}
	sub.open(subscriberBuffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.boardSubscribers[sub] = struct{}{}
	return sub, nil
}

func (h *streamHub) unsubscribeBoard(sub *boardSubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.boardSubscribers, sub)
}

func (h *streamHub) boardScopes() []boardScope {
	h.mu.Lock()
	defer h.mu.Unlock()
	scopes := make([]boardScope, 0, len(h.boardSubscribers))
	for sub := range h.boardSubscribers {
		scopes = append(scopes, sub.scope)
	}
	return scopes
}

func (h *streamHub) publishBoard(updates []EventBest, leagueOf map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.boardSubscribers {
		var inScope []EventBest
		for _, update := range updates {
			if sub.scope.includes(leagueOf[update.EventID]) {
				inScope = append(inScope, update)
			}
		}
		if len(inScope) > 0 {
			sub.offer(inScope)
		}
	}
}

func (s *Server) RunStreamHub(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error { return s.hub.run(ctx) })
	group.Go(func() error { return s.runBoardFeed(ctx) })
	group.Go(func() error {
		s.currentCoverage(ctx)
		return nil
	})
	group.Go(func() error { return s.keepRefreshed(ctx, "events", eventIndexRefresh, nil, s.refreshEventIndex) })
	group.Go(func() error {
		return s.keepRefreshed(ctx, "signals", signalSnapshotRefresh, s.hub.signaled, s.refreshSignals)
	})
	group.Go(func() error { return s.runPriorityFlush(ctx) })
	group.Go(func() error { return s.keepRefreshed(ctx, "sources", sourceSnapshotRefresh, nil, s.refreshSources) })
	group.Go(func() error {
		return s.keepRefreshed(ctx, "leagueSports", leagueSportRefresh, nil, s.refreshLeagueSports)
	})
	return group.Wait()
}

func (s *Server) runBoardFeed(ctx context.Context) error {
	ticker := time.NewTicker(coalesceWindow)
	defer ticker.Stop()
	leagues := map[string]string{}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.hub.stopped:
			return nil
		case <-ticker.C:
			s.syncBoard(ctx, leagues)
		}
	}
}

func (s *Server) syncBoard(ctx context.Context, leagues map[string]string) {
	moved, published := s.hub.changes.take()
	ctx, cancel := context.WithTimeout(ctx, boardFlushTimeout)
	defer cancel()
	if err := s.refreshBoard(ctx); err != nil {
		s.log.WarnContext(ctx, "refresh board cache", "err", err)
	}
	if err := s.flushBoardFeed(ctx, leagues, moved, published); err != nil {
		s.log.WarnContext(ctx, "flush board stream", "err", err)
	}
}

func (s *Server) flushBoardFeed(ctx context.Context, leagues map[string]string, moved []string, published map[string]EventBest) error {
	scopes := s.hub.boardScopes()
	if len(scopes) == 0 || len(moved)+len(published) == 0 {
		return nil
	}
	if err := s.cacheEventLeagues(ctx, leagues, append(moved, slices.Collect(maps.Keys(published))...)); err != nil {
		return err
	}
	inScope := func(eventID string) bool {
		return slices.ContainsFunc(scopes, func(scope boardScope) bool { return scope.includes(leagues[eventID]) })
	}
	var keys []canon.MarketKey
	for _, eventID := range moved {
		if inScope(eventID) {
			keys = append(keys, marketSelector{}.keyFor(eventID, s.sportOf(leagues[eventID])))
		}
	}
	bests, err := s.bestOutcomesOf(ctx, keys, nil)
	if err != nil {
		return err
	}
	for i, key := range keys {
		published[key.EventID] = mergeEventBest(published[key.EventID], EventBest{EventID: key.EventID, Best: &bests[i]})
	}
	updates := make([]EventBest, 0, len(published))
	for eventID, update := range published {
		if inScope(eventID) {
			updates = append(updates, update)
		}
	}
	slices.SortFunc(updates, func(a, b EventBest) int { return cmp.Compare(a.EventID, b.EventID) })
	s.hub.publishBoard(updates, leagues)
	return nil
}

func (s *Server) cacheEventLeagues(ctx context.Context, leagues map[string]string, eventIDs []string) error {
	if len(leagues) > maxCachedEventLeagues {
		clear(leagues)
	}
	var unknown []string
	for _, eventID := range eventIDs {
		if _, cached := leagues[eventID]; !cached && uuid.Validate(eventID) == nil {
			unknown = append(unknown, eventID)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	found, err := s.eventLeagues(ctx, unknown)
	if err != nil {
		return err
	}
	maps.Copy(leagues, found)
	return nil
}
