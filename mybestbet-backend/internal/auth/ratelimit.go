package auth

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type rule struct {
	name   string
	max    int64
	window time.Duration
}

var (
	otpPerEmail   = rule{name: "otp:email", max: 5, window: time.Hour}
	otpPerIP      = rule{name: "otp:ip", max: 20, window: time.Hour}
	loginPerIP    = rule{name: "login:ip", max: 10, window: time.Minute}
	verifyPerPair = rule{name: "verify", max: 10, window: 10 * time.Minute}
	refreshPerIP  = rule{name: "refresh:ip", max: 60, window: time.Minute}
	googlePerIP   = rule{name: "google:ip", max: 20, window: time.Minute}
)

const ipv6PrefixBits = 64

func networkOf(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, ipv6PrefixBits).Masked().String()
}

type counter interface {
	incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

type redisCounter struct {
	rdb *goredis.Client
}

func (c redisCounter) incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := c.rdb.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("rate limit %s: %w", key, err)
	}
	return count.Val(), nil
}

type limiter struct {
	counter counter
	now     func() time.Time
}

func (l limiter) allow(ctx context.Context, r rule, subject string) error {
	now := l.now()
	start := now.Truncate(r.window)
	key := "auth:rl:" + r.name + ":" + subject + ":" + strconv.FormatInt(start.Unix(), 10)
	count, err := l.counter.incr(ctx, key, r.window)
	if err != nil {
		return err
	}
	if count > r.max {
		return &RateLimitError{RetryAfter: start.Add(r.window).Sub(now)}
	}
	return nil
}
