package server

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
)

const (
	priorityLifetime   = 5 * time.Minute
	priorityFlushEvery = time.Second
)

type priorityQueue struct {
	mu     sync.Mutex
	events map[string]struct{}
}

func (q *priorityQueue) touch(eventID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.events == nil {
		q.events = map[string]struct{}{}
	}
	q.events[eventID] = struct{}{}
}

func (q *priorityQueue) take() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	events := make([]string, 0, len(q.events))
	for eventID := range q.events {
		events = append(events, eventID)
	}
	clear(q.events)
	return events
}

func (s *Server) runPriorityFlush(ctx context.Context) error {
	ticker := time.NewTicker(priorityFlushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.flushPriority(ctx, time.Now()); err != nil {
				s.log.WarnContext(ctx, "flush priority events", "err", err)
			}
		}
	}
}

func (s *Server) flushPriority(ctx context.Context, now time.Time) error {
	events := s.priority.take()
	if len(events) == 0 {
		return nil
	}
	expiry := float64(now.Add(priorityLifetime).UnixMilli())
	members := make([]goredis.Z, len(events))
	for i, eventID := range events {
		members[i] = goredis.Z{Score: expiry, Member: eventID}
	}
	pipe := s.rdb.Pipeline()
	pipe.ZAddGT(ctx, redis.PriorityEventsKey, members...)
	pipe.ZRemRangeByScore(ctx, redis.PriorityEventsKey, "-inf", "("+strconv.FormatInt(now.UnixMilli(), 10))
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write priority events: %w", err)
	}
	return nil
}
