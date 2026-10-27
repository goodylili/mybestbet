package notify

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
)

const (
	alertDedupeTTL  = 24 * time.Hour
	alertRateWindow = time.Hour
)

type claimOutcome int64

const (
	claimDuplicate claimOutcome = iota
	claimGranted
	claimThrottled
)

var claimAlertScript = goredis.NewScript(`
local previous = redis.call('GET', KEYS[1])
if previous and tonumber(ARGV[1]) < tonumber(previous) * (1 + tonumber(ARGV[2])) then
  return {0, false}
end
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', tonumber(ARGV[4]) - tonumber(ARGV[5]))
if redis.call('ZCARD', KEYS[2]) >= tonumber(ARGV[6]) then
  return {2, false}
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
redis.call('ZADD', KEYS[2], ARGV[4], ARGV[7])
redis.call('PEXPIRE', KEYS[2], ARGV[5])
return {1, previous}
`)

var releaseAlertScript = goredis.NewScript(`
redis.call('ZREM', KEYS[2], ARGV[2])
if ARGV[1] == '' then
  return redis.call('DEL', KEYS[1])
end
return redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')
`)

type alertClaim struct {
	dedupeKey     string
	rateKey       string
	rateMember    string
	previousValue string
}

type alertLimiter struct {
	rdb           *goredis.Client
	maxPerHour    int
	realertGrowth float64
	dedupeTTL     time.Duration
	rateWindow    time.Duration
	clock         func() time.Time
}

func alertRateKey(chatID int64) string { return "alert:rate:" + strconv.FormatInt(chatID, 10) }

func (l *alertLimiter) claim(ctx context.Context, signalDedupeKey string, chatID int64, strength float64) (claimOutcome, alertClaim, error) {
	now := l.clock()
	c := alertClaim{
		dedupeKey:  redis.AlertDedupeKey(signalDedupeKey, chatID),
		rateKey:    alertRateKey(chatID),
		rateMember: signalDedupeKey + "@" + strconv.FormatInt(now.UnixNano(), 10),
	}
	reply, err := claimAlertScript.Run(ctx, l.rdb, []string{c.dedupeKey, c.rateKey},
		strconv.FormatFloat(strength, 'g', -1, 64), l.realertGrowth, l.dedupeTTL.Milliseconds(),
		now.UnixMilli(), l.rateWindow.Milliseconds(), l.maxPerHour, c.rateMember).Slice()
	if err != nil {
		return claimDuplicate, c, fmt.Errorf("claim alert: %w", err)
	}
	outcome, ok := reply[0].(int64)
	if !ok {
		return claimDuplicate, c, fmt.Errorf("claim alert: unexpected reply %v", reply)
	}
	if previous, ok := reply[1].(string); ok {
		c.previousValue = previous
	}
	return claimOutcome(outcome), c, nil
}

func (l *alertLimiter) release(ctx context.Context, c alertClaim) error {
	if err := releaseAlertScript.Run(ctx, l.rdb, []string{c.dedupeKey, c.rateKey}, c.previousValue, c.rateMember).Err(); err != nil {
		return fmt.Errorf("release alert claim: %w", err)
	}
	return nil
}
