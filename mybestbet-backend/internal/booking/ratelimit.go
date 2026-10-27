package booking

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

type limitRule struct {
	name   string
	max    int64
	window time.Duration
}

var (
	bookPerUser    = limitRule{name: "book", max: 20, window: time.Minute}
	comparePerUser = limitRule{name: "compare", max: 120, window: time.Minute}
)

func (s *Service) allow(ctx context.Context, rule limitRule, userID string) error {
	now := s.now()
	start := now.Truncate(rule.window)
	key := "booking:rl:" + rule.name + ":" + userID + ":" + strconv.FormatInt(start.Unix(), 10)
	pipe := s.rdb.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, rule.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("booking rate limit: %w", err)
	}
	if count.Val() > rule.max {
		return &RateLimitError{RetryAfter: start.Add(rule.window).Sub(now)}
	}
	return nil
}
