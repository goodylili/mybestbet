package resolve

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

func TestPublishDeltasChunksLargeBatchesAndKeepsFreshnessLast(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL is required")
	}
	rdb, err := redis.Open(t.Context(), redisURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	eventID := uuid.NewString()
	deltas := make([]canon.Delta, publishChunk*2+1)
	for i := range deltas {
		deltas[i] = canon.Delta{Source: "publish-test", Market: canon.Market{Key: canon.MarketKey{EventID: eventID, Code: canon.Code1X2}}}
	}
	afterID := "0-0"
	if last, err := rdb.XRevRangeN(t.Context(), redis.StreamOddsDelta, "+", "-", 1).Result(); err == nil && len(last) > 0 {
		afterID = last[0].ID
	}

	require.NoError(t, publishDeltas(t.Context(), rdb, deltas, []canon.Freshness{{Source: "publish-test", EventID: eventID}}))

	entries, err := rdb.XRange(t.Context(), redis.StreamOddsDelta, "("+afterID, "+").Result()
	require.NoError(t, err)
	var fields []string
	for _, e := range entries {
		if e.Values[redis.EventIDField] != eventID {
			continue
		}
		if _, ok := e.Values[redis.FreshnessField]; ok {
			fields = append(fields, redis.FreshnessField)
		} else {
			fields = append(fields, redis.StreamField)
		}
	}
	require.Len(t, fields, len(deltas)+1)
	assert.Equal(t, redis.FreshnessField, fields[len(fields)-1])
}
