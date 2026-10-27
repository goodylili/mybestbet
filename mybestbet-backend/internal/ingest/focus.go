package ingest

import (
	"context"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
)

const (
	focusRefreshEvery   = 2 * time.Second
	focusRefreshTimeout = time.Second
)

type focusSet struct {
	rdb    *goredis.Client
	log    *slog.Logger
	now    func() time.Time
	events atomic.Pointer[map[string]bool]
}

func newFocusSet(rdb *goredis.Client, log *slog.Logger) *focusSet {
	return &focusSet{rdb: rdb, log: log, now: time.Now}
}

func (f *focusSet) has(eventID string) bool {
	if f == nil {
		return false
	}
	events := f.events.Load()
	return events != nil && (*events)[eventID]
}

func (f *focusSet) isEmpty() bool {
	events := f.events.Load()
	return events == nil || len(*events) == 0
}

func (f *focusSet) refreshUntilDone(ctx context.Context) error {
	ticker := time.NewTicker(focusRefreshEvery)
	defer ticker.Stop()
	for {
		f.refresh(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (f *focusSet) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, focusRefreshTimeout)
	defer cancel()
	ids, err := f.rdb.ZRangeArgs(ctx, goredis.ZRangeArgs{Key: redis.PriorityEventsKey, Start: strconv.FormatInt(f.now().UnixMilli(), 10), Stop: "+inf", ByScore: true}).Result()
	if err != nil {
		if ctx.Err() == nil {
			f.log.Warn("read priority events", "err", err)
		}
		return
	}
	events := make(map[string]bool, len(ids))
	for _, id := range ids {
		events[id] = true
	}
	f.events.Store(&events)
	focusedEvents.Set(float64(len(events)))
}
