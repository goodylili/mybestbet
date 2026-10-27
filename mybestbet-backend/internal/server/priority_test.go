package server

import (
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/redis"
)

func TestFlushPriorityKeepsLatestExpiry(t *testing.T) {
	_, rdb := integrationDeps(t)
	s := &Server{rdb: rdb, log: slog.Default()}
	opened, expired := uuid.NewString(), uuid.NewString()
	now := time.Now()
	require.NoError(t, rdb.ZAdd(t.Context(), redis.PriorityEventsKey, goredis.Z{Score: float64(now.Add(-time.Second).UnixMilli()), Member: expired}).Err())
	t.Cleanup(func() { _ = rdb.ZRem(t.Context(), redis.PriorityEventsKey, opened, expired).Err() })

	s.priority.touch(opened)
	s.priority.touch(opened)
	require.NoError(t, s.flushPriority(t.Context(), now))
	s.priority.touch(opened)
	require.NoError(t, s.flushPriority(t.Context(), now.Add(-time.Minute)))
	require.NoError(t, s.flushPriority(t.Context(), now), "an empty queue writes nothing")

	score, err := rdb.ZScore(t.Context(), redis.PriorityEventsKey, opened).Result()
	require.NoError(t, err)
	assert.InDelta(t, float64(now.Add(priorityLifetime).UnixMilli()), score, 0, "an older touch never shortens the expiry")
	_, err = rdb.ZScore(t.Context(), redis.PriorityEventsKey, expired).Result()
	assert.Error(t, err, "expired members are trimmed")
}
