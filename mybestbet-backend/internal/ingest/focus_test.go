package ingest

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
)

func TestFocusSetReadsUnexpiredPriorityEvents(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	ctx := context.Background()
	rdb, err := redis.Open(ctx, redisURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	require.NoError(t, rdb.Del(ctx, redis.PriorityEventsKey).Err())
	t.Cleanup(func() { _ = rdb.Del(ctx, redis.PriorityEventsKey).Err() })
	require.NoError(t, rdb.ZAdd(ctx, redis.PriorityEventsKey,
		goredis.Z{Member: "open", Score: float64(now.Add(time.Minute).UnixMilli())},
		goredis.Z{Member: "expired", Score: float64(now.Add(-time.Second).UnixMilli())},
	).Err())

	f := newFocusSet(rdb, slog.New(slog.DiscardHandler))
	f.now = func() time.Time { return now }
	assert.True(t, f.isEmpty())
	f.refresh(ctx)

	assert.False(t, f.isEmpty())
	assert.True(t, f.has("open"))
	assert.False(t, f.has("expired"))
	var unset *focusSet
	assert.False(t, unset.has("open"))
}
