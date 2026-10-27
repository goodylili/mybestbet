package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
)

const (
	healthRefreshEvery  = 2 * time.Second
	degradedFlag        = "1"
	healthFieldDegraded = "degraded"
)

type sourceHealth struct {
	rdb      *goredis.Client
	sources  []string
	degraded atomic.Pointer[map[string]bool]
	log      *slog.Logger
}

func newSourceHealth(rdb *goredis.Client, sources []string, log *slog.Logger) *sourceHealth {
	h := &sourceHealth{rdb: rdb, sources: sources, log: log}
	h.degraded.Store(&map[string]bool{})
	return h
}

func (h *sourceHealth) isDegraded(source string) bool { return (*h.degraded.Load())[source] }

func (h *sourceHealth) refresh(ctx context.Context) error {
	pipe := h.rdb.Pipeline()
	flags := make([]*goredis.StringCmd, len(h.sources))
	for i, source := range h.sources {
		flags[i] = pipe.HGet(ctx, redis.SourceHealthKey(source), healthFieldDegraded)
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
		return fmt.Errorf("read source health: %w", err)
	}
	degraded := make(map[string]bool, len(h.sources))
	for i, source := range h.sources {
		degraded[source] = flags[i].Val() == degradedFlag
	}
	h.degraded.Store(&degraded)
	return nil
}

func (h *sourceHealth) refreshUntilDone(ctx context.Context) error {
	ticker := time.NewTicker(healthRefreshEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := h.refresh(ctx); err != nil {
				h.log.Warn("refresh source health", "err", err)
			}
		}
	}
}
