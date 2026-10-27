package loadtest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/redis"
)

const (
	PathRedis         = "redis-pubsub"
	PathSSE           = "sse"
	drainPollInterval = 50 * time.Millisecond
)

type Config struct {
	RedisURL       string
	APIURL         string
	TestdataDir    string
	Events         int
	SSEEvents      int
	Duration       time.Duration
	HotInterval    time.Duration
	RateMultiplier float64
	MaxJitter      float64
	DropAfter      time.Duration
}

type Result struct {
	Sources     int
	Pairs       int
	Period      time.Duration
	DeltasPerS  float64
	Reports     []Report
	SSEMeasured bool
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	eventsBySource, err := loadFixtureEvents(cfg.TestdataDir)
	if err != nil {
		return Result{}, err
	}
	eventIDs := syntheticEventIDs(cfg.Events)
	pairs := planReplayPairs(eventsBySource, eventIDs)
	period := time.Duration(float64(cfg.HotInterval) / cfg.RateMultiplier)
	rdb, err := redis.Open(ctx, cfg.RedisURL)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = rdb.Close() }()
	trackers, err := measure(ctx, cfg, rdb, eventIDs, &replayer{rdb: rdb, period: period, maxJitter: cfg.MaxJitter}, pairs)
	if err != nil {
		return Result{}, err
	}
	result := Result{Sources: len(eventsBySource), Pairs: len(pairs), Period: period, DeltasPerS: deltasPerSecond(pairs, period), SSEMeasured: len(trackers) > 1}
	for _, t := range trackers {
		result.Reports = append(result.Reports, t.report())
	}
	return result, nil
}

func measure(ctx context.Context, cfg Config, rdb *goredis.Client, eventIDs []string, r *replayer, pairs []replayPair) ([]*tracker, error) {
	observeCtx, stopObserving := context.WithCancel(ctx)
	defer stopObserving()
	observers, observeCtx := errgroup.WithContext(observeCtx)
	redisTracker := newTracker(PathRedis, eventIDs, cfg.DropAfter, false)
	pubsub, err := subscribeEventChannels(ctx, rdb, eventIDs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = pubsub.Close() }()
	observers.Go(func() error { return observePubSub(observeCtx, pubsub, redisTracker) })
	r.trackers = []*tracker{redisTracker}
	if cfg.APIURL != "" {
		sseTracker, err := startSSEObservers(observeCtx, observers, cfg, eventIDs[:min(cfg.SSEEvents, len(eventIDs))])
		if err != nil {
			stopObserving()
			return nil, errors.Join(err, observers.Wait())
		}
		r.trackers = append(r.trackers, sseTracker)
	}
	replayCtx, stopReplay := context.WithTimeout(observeCtx, cfg.Duration)
	defer stopReplay()
	replayErr := r.replayAll(replayCtx, pairs)
	awaitDelivery(observeCtx, r.trackers, cfg.DropAfter)
	stopObserving()
	if err := errors.Join(replayErr, observers.Wait()); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("load test interrupted: %w", ctx.Err())
	}
	return r.trackers, nil
}

func startSSEObservers(ctx context.Context, observers *errgroup.Group, cfg Config, sampled []string) (*tracker, error) {
	t := newTracker(PathSSE, sampled, cfg.DropAfter, true)
	client := &http.Client{}
	for _, eventID := range sampled {
		stream, err := openSSEStream(ctx, client, cfg.APIURL, eventID)
		if err != nil {
			return nil, err
		}
		observers.Go(func() error { return observeSSE(ctx, stream, t) })
	}
	return t, nil
}

func awaitDelivery(ctx context.Context, trackers []*tracker, dropAfter time.Duration) {
	deadline := time.Now().Add(dropAfter)
	ticker := time.NewTicker(drainPollInterval)
	defer ticker.Stop()
	for time.Now().Before(deadline) && totalOutstanding(trackers) > 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func totalOutstanding(trackers []*tracker) int {
	n := 0
	for _, t := range trackers {
		n += t.outstanding()
	}
	return n
}

func deltasPerSecond(pairs []replayPair, period time.Duration) float64 {
	markets := 0
	for _, p := range pairs {
		markets += len(p.markets)
	}
	return float64(markets) / period.Seconds()
}
