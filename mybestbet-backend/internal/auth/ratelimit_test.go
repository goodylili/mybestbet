package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryCounter map[string]int64

func (m memoryCounter) incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	m[key]++
	return m[key], nil
}

func TestLimiterFixedWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 26, 12, 0, 20, 0, time.UTC)
	l := limiter{counter: memoryCounter{}, now: func() time.Time { return now }}
	r := rule{name: "test", max: 3, window: time.Minute}
	ctx := context.Background()
	for range 3 {
		require.NoError(t, l.allow(ctx, r, "subject"))
	}
	err := l.allow(ctx, r, "subject")
	var limited *RateLimitError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, 40*time.Second, limited.RetryAfter)
	assert.NoError(t, l.allow(ctx, r, "other subject"), "subjects are counted apart")
	now = now.Add(time.Minute)
	assert.NoError(t, l.allow(ctx, r, "subject"), "a new window resets the count")
}

func TestNetworkOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ip       string
		expected string
	}{
		{name: "ipv4 is kept", ip: "203.0.113.7", expected: "203.0.113.7"},
		{name: "ipv6 collapses to its /64", ip: "2001:db8:1:2:aaaa:bbbb:cccc:dddd", expected: "2001:db8:1:2::/64"},
		{name: "same /64 shares a bucket", ip: "2001:db8:1:2::1", expected: "2001:db8:1:2::/64"},
		{name: "ipv4 mapped", ip: "::ffff:203.0.113.7", expected: "203.0.113.7"},
		{name: "unparseable is kept", ip: "unknown", expected: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, networkOf(tt.ip))
		})
	}
}
