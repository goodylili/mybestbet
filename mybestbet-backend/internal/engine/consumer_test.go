package engine

import (
	"log/slog"
	"testing"
	"time"

	"github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

func TestDecodeMessageRoutesFreshnessAndDeltas(t *testing.T) {
	t.Parallel()
	fresh, err := json.Marshal(canon.Freshness{Source: "betking", EventID: "ev1", ObservedAt: testNow})
	require.NoError(t, err)
	delta, err := json.Marshal(matchResultDelta("betking", 2, 3, 4, testNow))
	require.NoError(t, err)

	f, err := decodeMessage(goredis.XMessage{ID: "1-0", Values: map[string]any{redis.FreshnessField: string(fresh)}})
	require.NoError(t, err)
	require.NotNil(t, f.freshness)
	assert.Equal(t, "ev1", f.eventID())
	assert.True(t, f.freshness.ObservedAt.Equal(testNow))

	d, err := decodeMessage(goredis.XMessage{ID: "2-0", Values: map[string]any{redis.StreamField: string(delta)}})
	require.NoError(t, err)
	assert.Nil(t, d.freshness)
	assert.Equal(t, matchResult.EventID, d.eventID())

	_, err = decodeMessage(goredis.XMessage{ID: "3-0", Values: map[string]any{redis.FreshnessField: `{"observedAt":"` + time.Now().Format(time.RFC3339) + `"}`}})
	require.Error(t, err)
}

func TestRoutableDefersDecodingWhenEventIDIsInTheMessage(t *testing.T) {
	t.Parallel()
	delta, err := json.Marshal(matchResultDelta("betking", 2, 3, 4, testNow))
	require.NoError(t, err)

	fast, err := routable(goredis.XMessage{ID: "1-0", Values: map[string]any{redis.StreamField: string(delta), redis.EventIDField: matchResult.EventID}})
	require.NoError(t, err)
	assert.Equal(t, matchResult.EventID, fast.eventID)
	require.NotNil(t, fast.message.undecoded)

	legacy, err := routable(goredis.XMessage{ID: "2-0", Values: map[string]any{redis.StreamField: string(delta)}})
	require.NoError(t, err)
	assert.Equal(t, matchResult.EventID, legacy.eventID)
	assert.Nil(t, legacy.message.undecoded)

	b := newTestBoard()
	s := &shard{board: b, log: slog.New(slog.DiscardHandler)}
	cs := s.stageDelta(fast.message, testNow)
	assert.NotEmpty(t, cs.cells)
	assert.Empty(t, s.stageDelta(streamDelta{messageID: "3-0", undecoded: &goredis.XMessage{ID: "3-0", Values: map[string]any{redis.StreamField: "{"}}}, testNow).cells)
}
