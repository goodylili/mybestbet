package server

import (
	"context"
	"fmt"
	"slices"
	"time"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const boardReadWorkers = 4

func (s *Server) eventBoardCells(ctx context.Context, eventID string) ([]canon.BoardCell, error) {
	marketKeys, ok := s.hub.board.eventMarkets(eventID, time.Now())
	if !ok {
		loaded, err := s.loadEventMarkets(ctx, []string{eventID})
		if err != nil {
			return nil, err
		}
		marketKeys = loaded[eventID]
	}
	return s.boardCells(ctx, marketKeys)
}

func (s *Server) loadEventMarkets(ctx context.Context, eventIDs []string) (map[string][]string, error) {
	pipe := s.rdb.Pipeline()
	members := make([]*goredis.StringSliceCmd, len(eventIDs))
	for i, eventID := range eventIDs {
		members[i] = pipe.SMembers(ctx, redis.EventMarketsKey(eventID))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("read event markets: %w", err)
	}
	now := time.Now()
	loaded := make(map[string][]string, len(eventIDs))
	for i, eventID := range eventIDs {
		loaded[eventID] = members[i].Val()
		s.hub.board.storeEventMarkets(eventID, loaded[eventID], now)
	}
	return loaded, nil
}

func (s *Server) boardCells(ctx context.Context, marketKeys []string) ([]canon.BoardCell, error) {
	if len(marketKeys) == 0 {
		return nil, nil
	}
	hits, misses := s.hub.board.lookup(marketKeys, time.Now())
	loaded, err := s.loadBoardHashes(ctx, misses)
	if err != nil {
		return nil, err
	}
	var cells []canon.BoardCell
	for _, key := range marketKeys {
		if found, ok := hits[key]; ok {
			cells = append(cells, found...)
			continue
		}
		cells = append(cells, loaded[key]...)
	}
	return cells, nil
}

func (s *Server) loadBoardHashes(ctx context.Context, misses []boardCacheMiss) (map[string][]canon.BoardCell, error) {
	batches := slices.Collect(slices.Chunk(misses, coverageReadBatch))
	decoded := make([][][]canon.BoardCell, len(batches))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(boardReadWorkers)
	for i, batch := range batches {
		group.Go(func() error {
			cells, err := s.loadBoardBatch(groupCtx, batch)
			decoded[i] = cells
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	loaded := make(map[string][]canon.BoardCell, len(misses))
	for i, batch := range batches {
		for j, miss := range batch {
			loaded[miss.marketKey] = decoded[i][j]
		}
	}
	return loaded, nil
}

func (s *Server) loadBoardBatch(ctx context.Context, batch []boardCacheMiss) ([][]canon.BoardCell, error) {
	pipe := s.rdb.Pipeline()
	hashes := make([]*goredis.MapStringStringCmd, len(batch))
	for i, miss := range batch {
		hashes[i] = pipe.HGetAll(ctx, redis.BoardKey(miss.marketKey))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("read board hashes: %w", err)
	}
	now := time.Now()
	cells := make([][]canon.BoardCell, len(batch))
	for i, miss := range batch {
		cells[i] = s.decodeBoardHash(ctx, miss.marketKey, hashes[i].Val())
		s.hub.board.store(miss, cells[i], now)
	}
	return cells, nil
}

func (s *Server) refreshBoard(ctx context.Context) error {
	misses, events := s.hub.board.takeDirty(time.Now())
	if len(events) > 0 {
		if _, err := s.loadEventMarkets(ctx, events); err != nil {
			return err
		}
	}
	_, err := s.loadBoardHashes(ctx, misses)
	return err
}

func (s *Server) decodeBoardHash(ctx context.Context, marketKey string, hash map[string]string) []canon.BoardCell {
	cells := make([]canon.BoardCell, 0, len(hash))
	for field, encoded := range hash {
		var cell canon.BoardCell
		if err := json.Unmarshal([]byte(encoded), &cell); err != nil {
			s.log.WarnContext(ctx, "skip undecodable board cell", "market_key", marketKey, "field", field, "err", err)
			continue
		}
		if cell.MarketKey == "" {
			cell.MarketKey = marketKey
		}
		cells = append(cells, cell)
	}
	return cells
}

type marketSelector struct {
	code   canon.MarketCode
	line   *float64
	period string
}

func (m marketSelector) keyFor(eventID string, sport canon.Sport) canon.MarketKey {
	code := m.code
	if code == "" {
		code = mainMarketOf(sport)
	}
	return canon.MarketKey{EventID: eventID, Code: code, Line: m.line, Period: m.period}
}

func pricedSinceKickoff(cells []canon.BoardCell, kickoff, now time.Time) []canon.BoardCell {
	if kickoff.IsZero() || now.Before(kickoff) {
		return cells
	}
	return slices.DeleteFunc(cells, func(c canon.BoardCell) bool { return c.Price.ObservedAt.Before(kickoff) })
}

func (s *Server) bestOutcomesOf(ctx context.Context, keys []canon.MarketKey, kickoffs []time.Time) ([]BestOutcomes, error) {
	keyStrings := make([]string, len(keys))
	for i, key := range keys {
		keyStrings[i] = key.String()
	}
	cells, err := s.boardCells(ctx, keyStrings)
	if err != nil {
		return nil, err
	}
	cellsByMarket := map[string][]canon.BoardCell{}
	for _, cell := range cells {
		cellsByMarket[cell.MarketKey] = append(cellsByMarket[cell.MarketKey], cell)
	}
	bests := make([]BestOutcomes, len(keys))
	now := time.Now()
	for i, key := range keys {
		cells := cellsByMarket[keyStrings[i]]
		if i < len(kickoffs) {
			cells = pricedSinceKickoff(cells, kickoffs[i], now)
		}
		bests[i] = bestOutcomes(key, cells)
	}
	return bests, nil
}
