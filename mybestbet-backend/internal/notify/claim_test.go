package notify

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlertLimiterDedupesAndRealertsOnGrowth(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	limiter := &alertLimiter{rdb: rdb, maxPerHour: 30, realertGrowth: 0.5, dedupeTTL: time.Minute, rateWindow: time.Hour, clock: time.Now}
	dedupe := "test:" + uuid.NewString()
	chatID := time.Now().UnixNano()

	steps := []struct {
		name     string
		strength float64
		expected claimOutcome
	}{
		{name: "first alert", strength: 0.010, expected: claimGranted},
		{name: "same margin is a repeat", strength: 0.010, expected: claimDuplicate},
		{name: "growth below half again", strength: 0.014, expected: claimDuplicate},
		{name: "growth of half again re-alerts", strength: 0.015, expected: claimGranted},
		{name: "baseline moves to latest alert", strength: 0.020, expected: claimDuplicate},
	}
	for _, step := range steps {
		outcome, _, err := limiter.claim(ctx, dedupe, chatID, step.strength)
		require.NoError(t, err, step.name)
		assert.Equal(t, step.expected, outcome, step.name)
	}
}

func TestAlertLimiterThrottlesPerChat(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	limiter := &alertLimiter{rdb: rdb, maxPerHour: 3, realertGrowth: 0.5, dedupeTTL: time.Minute, rateWindow: time.Hour, clock: time.Now}
	chatID := time.Now().UnixNano()
	for i := range 3 {
		outcome, _, err := limiter.claim(ctx, "test:"+uuid.NewString(), chatID, 0.01)
		require.NoError(t, err)
		assert.Equal(t, claimGranted, outcome, "alert %d", i)
	}
	throttledKey := "test:" + uuid.NewString()
	outcome, _, err := limiter.claim(ctx, throttledKey, chatID, 0.01)
	require.NoError(t, err)
	assert.Equal(t, claimThrottled, outcome)

	outcome, _, err = limiter.claim(ctx, throttledKey, chatID+1, 0.01)
	require.NoError(t, err)
	assert.Equal(t, claimGranted, outcome, "other chats keep their own budget")
}

func TestAlertLimiterReleaseRestoresPreviousClaim(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	limiter := &alertLimiter{rdb: rdb, maxPerHour: 2, realertGrowth: 0.5, dedupeTTL: time.Minute, rateWindow: time.Hour, clock: time.Now}
	dedupe := "test:" + uuid.NewString()
	chatID := time.Now().UnixNano()

	_, _, err := limiter.claim(ctx, dedupe, chatID, 0.01)
	require.NoError(t, err)
	outcome, claim, err := limiter.claim(ctx, dedupe, chatID, 0.02)
	require.NoError(t, err)
	require.Equal(t, claimGranted, outcome)
	require.NoError(t, limiter.release(ctx, claim))

	outcome, _, err = limiter.claim(ctx, dedupe, chatID, 0.014)
	require.NoError(t, err)
	assert.Equal(t, claimDuplicate, outcome, "previous margin 0.01 is the baseline again")
	outcome, _, err = limiter.claim(ctx, dedupe, chatID, 0.02)
	require.NoError(t, err)
	assert.Equal(t, claimGranted, outcome, "released claim gave its rate slot back")
}
