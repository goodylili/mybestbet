package server

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	coverageTTL            = 3 * time.Minute
	coverageRefreshTimeout = 2 * time.Minute
	coverageReadBatch      = 250
)

type eventCoverage struct {
	sources   []string
	propCount int
}

type coverageSnapshot struct {
	byEvent  map[string]eventCoverage
	bySource map[string]SourceCoverage
	loadedAt time.Time
}

type coverageCache struct {
	mu       sync.Mutex
	snapshot *coverageSnapshot
	loading  chan struct{}
}

func (s *Server) sourceCoverage(ctx context.Context) (map[string]SourceCoverage, error) {
	return s.currentCoverage(ctx).bySource, nil
}

func (s *Server) coverageByEvent(ctx context.Context, eventIDs []string) (map[string]eventCoverage, error) {
	snapshot := s.currentCoverage(ctx)
	coverage := make(map[string]eventCoverage, len(eventIDs))
	for _, id := range eventIDs {
		if c, ok := snapshot.byEvent[id]; ok {
			coverage[id] = c
		}
	}
	return coverage, nil
}

func (s *Server) currentCoverage(ctx context.Context) *coverageSnapshot {
	c := &s.coverage
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loading == nil && (c.snapshot == nil || time.Since(c.snapshot.loadedAt) >= coverageTTL) {
		c.loading = make(chan struct{})
		go s.refreshCoverage(context.WithoutCancel(ctx), c.loading)
	}
	if c.snapshot == nil {
		return &coverageSnapshot{}
	}
	return c.snapshot
}

func (s *Server) refreshCoverage(ctx context.Context, done chan struct{}) {
	ctx, cancel := context.WithTimeout(ctx, coverageRefreshTimeout)
	defer cancel()
	snapshot, err := s.loadCoverage(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "refresh coverage", "err", err)
	}
	c := &s.coverage
	c.mu.Lock()
	if snapshot != nil {
		c.snapshot = snapshot
	}
	c.loading = nil
	c.mu.Unlock()
	close(done)
}

func (s *Server) loadCoverage(ctx context.Context) (*coverageSnapshot, error) {
	leagues, err := s.openEventLeagues(ctx)
	if err != nil {
		return nil, err
	}
	eventIDs := slices.Collect(maps.Keys(leagues))
	marketsByEvent, err := readInBatches(ctx, s.rdb, eventIDs, func(pipe goredis.Pipeliner, id string) *goredis.StringSliceCmd {
		return pipe.SMembers(ctx, redis.EventMarketsKey(id))
	})
	if err != nil {
		return nil, fmt.Errorf("read event markets: %w", err)
	}
	var marketKeys, owners []string
	for i, id := range eventIDs {
		marketKeys = append(marketKeys, marketsByEvent[i]...)
		for range marketsByEvent[i] {
			owners = append(owners, id)
		}
	}
	fields, err := readInBatches(ctx, s.rdb, marketKeys, func(pipe goredis.Pipeliner, key string) *goredis.StringSliceCmd {
		return pipe.HKeys(ctx, redis.BoardKey(key))
	})
	if err != nil {
		return nil, fmt.Errorf("read board fields: %w", err)
	}
	builder := newCoverageBuilder()
	for i, key := range marketKeys {
		builder.add(owners[i], leagues[owners[i]], key, fields[i])
	}
	return builder.snapshot(), nil
}

func readInBatches(ctx context.Context, rdb *goredis.Client, keys []string, queue func(goredis.Pipeliner, string) *goredis.StringSliceCmd) ([][]string, error) {
	results := make([][]string, 0, len(keys))
	for batch := range slices.Chunk(keys, coverageReadBatch) {
		pipe := rdb.Pipeline()
		cmds := make([]*goredis.StringSliceCmd, len(batch))
		for i, key := range batch {
			cmds[i] = queue(pipe, key)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, err
		}
		for _, cmd := range cmds {
			results = append(results, cmd.Val())
		}
	}
	return results, nil
}

func (s *Server) openEventLeagues(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, "leagueCode" FROM events WHERE status IN ('scheduled', 'live') AND "mergedInto" IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list open events: %w", err)
	}
	leagues := map[string]string{}
	var eventID, league string
	_, err = pgx.ForEachRow(rows, []any{&eventID, &league}, func() error {
		leagues[eventID] = league
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan open events: %w", err)
	}
	return leagues, nil
}

type coverageBuilder struct {
	eventSources  map[string]map[string]struct{}
	propCounts    map[string]int
	sourceLeagues map[string]map[string]struct{}
	sourceMarkets map[string]map[canon.MarketCode]struct{}
}

func newCoverageBuilder() *coverageBuilder {
	return &coverageBuilder{
		eventSources:  map[string]map[string]struct{}{},
		propCounts:    map[string]int{},
		sourceLeagues: map[string]map[string]struct{}{},
		sourceMarkets: map[string]map[canon.MarketCode]struct{}{},
	}
}

func (b *coverageBuilder) add(eventID, league, marketKey string, fields []string) {
	key, err := canon.ParseMarketKey(marketKey)
	if err != nil {
		return
	}
	priced := false
	for _, field := range fields {
		source, _, _ := strings.Cut(field, "|")
		if source == consensusSource {
			continue
		}
		priced = true
		addTo(b.eventSources, eventID, source)
		addTo(b.sourceLeagues, source, league)
		addTo(b.sourceMarkets, source, key.Code)
	}
	if priced && key.PlayerID != "" {
		b.propCounts[eventID]++
	}
}

func addTo[K comparable](sets map[string]map[K]struct{}, owner string, member K) {
	set := sets[owner]
	if set == nil {
		set = map[K]struct{}{}
		sets[owner] = set
	}
	set[member] = struct{}{}
}

func (b *coverageBuilder) snapshot() *coverageSnapshot {
	snapshot := &coverageSnapshot{
		byEvent:  make(map[string]eventCoverage, len(b.eventSources)),
		bySource: make(map[string]SourceCoverage, len(b.sourceLeagues)),
		loadedAt: time.Now(),
	}
	for eventID, sources := range b.eventSources {
		snapshot.byEvent[eventID] = eventCoverage{sources: slices.Sorted(maps.Keys(sources)), propCount: b.propCounts[eventID]}
	}
	for source, leagues := range b.sourceLeagues {
		snapshot.bySource[source] = SourceCoverage{Leagues: slices.Sorted(maps.Keys(leagues)), Markets: slices.Sorted(maps.Keys(b.sourceMarkets[source]))}
	}
	return snapshot
}
