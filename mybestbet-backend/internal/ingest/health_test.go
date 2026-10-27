package ingest

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/sources"
)

func TestBackoffAfter(t *testing.T) {
	tests := []struct {
		name     string
		failures int
		expected time.Duration
	}{
		{name: "first failure", failures: 1, expected: 5 * time.Second},
		{name: "second failure doubles", failures: 2, expected: 10 * time.Second},
		{name: "fourth failure", failures: 4, expected: 40 * time.Second},
		{name: "capped", failures: 20, expected: backoffCap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, backoffAfter(tt.failures))
		})
	}
}

func TestHealthBacksOffOnThrottleAndDegradesAfterThreeFailures(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	throttled := fmt.Errorf("price: %w", &sources.StatusError{URL: "https://book/x", Status: 429})
	h := &health{}

	h.recordFailure(now, time.Second, throttled)
	assert.True(t, h.isPaused(now.Add(4*time.Second)))
	assert.False(t, h.isPaused(now.Add(5*time.Second)))
	assert.False(t, h.isDegraded())

	h.recordFailure(now, time.Second, errors.New("connection reset"))
	assert.False(t, h.isPaused(now.Add(6*time.Second)), "plain errors do not extend backoff")
	h.recordFailure(now, time.Second, fmt.Errorf("discover: %w", sources.ErrForbidden))
	assert.True(t, h.isDegraded())
	assert.True(t, h.isPaused(now.Add(19*time.Second)))

	fields := h.redisFields(now)
	assert.Equal(t, "1", fields["degraded"])
	assert.Equal(t, 3, fields["consecutiveFailures"])
	assert.Equal(t, int64(3), fields["errors"])

	h.recordSuccess(now.Add(time.Minute), 200*time.Millisecond, now.Add(50*time.Second))
	assert.False(t, h.isDegraded())
	assert.False(t, h.isPaused(now.Add(time.Minute)))
	fields = h.redisFields(now.Add(time.Minute))
	assert.Equal(t, "0", fields["degraded"])
	assert.Equal(t, int64(10000), fields["freshnessMs"])
	assert.Equal(t, int64(200), fields["lastFetchMs"])
	assert.Equal(t, now.Add(time.Minute).Format(time.RFC3339Nano), fields["lastSuccess"])
}
