package engine

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/redis"
)

func TestDropDeadConsumersReleasesTheirPendingDeltas(t *testing.T) {
	_, rdb := integrationDeps(t)
	ctx := context.Background()
	group := "engine-test-" + uuid.NewString()
	require.NoError(t, ensureConsumerGroup(ctx, rdb, group))
	t.Cleanup(func() { _ = rdb.XGroupDestroy(ctx, redis.StreamOddsDelta, group).Err() })
	require.NoError(t, rdb.XAdd(ctx, &goredis.XAddArgs{Stream: redis.StreamOddsDelta, Values: []any{redis.StreamField, "{}"}}).Err())
	require.NoError(t, rdb.XReadGroup(ctx, &goredis.XReadGroupArgs{Group: group, Consumer: "engine-gone", Streams: []string{redis.StreamOddsDelta, newMessagesCursor}, Count: 1}).Err())
	require.NoError(t, rdb.XGroupCreateConsumer(ctx, redis.StreamOddsDelta, group, "engine-live").Err())

	e := &Engine{rdb: rdb, group: group, consumer: "engine-live", log: slog.New(slog.DiscardHandler)}
	require.NoError(t, e.dropDeadConsumers(ctx, 0))

	consumers, err := rdb.XInfoConsumers(ctx, redis.StreamOddsDelta, group).Result()
	require.NoError(t, err)
	require.Len(t, consumers, 1)
	assert.Equal(t, "engine-live", consumers[0].Name)
	pending, err := rdb.XPending(ctx, redis.StreamOddsDelta, group).Result()
	require.NoError(t, err)
	assert.Zero(t, pending.Count)
}
