package loadtest

import (
	"context"
	"errors"
	"fmt"
	"time"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const oddsDeltaStreamMaxLen = 200_000

type replayer struct {
	rdb       *goredis.Client
	trackers  []*tracker
	period    time.Duration
	maxJitter float64
}

func (r *replayer) replayAll(ctx context.Context, pairs []replayPair) error {
	group, groupCtx := errgroup.WithContext(ctx)
	for i, pair := range pairs {
		phase := r.period * time.Duration(i) / time.Duration(len(pairs))
		group.Go(func() error { return r.replayEvery(groupCtx, pair, phase) })
	}
	return group.Wait()
}

func (r *replayer) replayEvery(ctx context.Context, pair replayPair, phase time.Duration) error {
	if !sleepOrDone(ctx, phase) {
		return nil
	}
	ticker := time.NewTicker(r.period)
	defer ticker.Stop()
	for {
		if err := r.sendPair(ctx, pair); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *replayer) sendPair(ctx context.Context, pair replayPair) error {
	observedAt := time.Now().UTC()
	pipe := r.rdb.Pipeline()
	for _, m := range pair.markets {
		market := repriced(m, observedAt, r.maxJitter)
		body, err := json.Marshal(canon.Delta{Source: pair.source, Market: market, FetchedAt: observedAt, ResolvedAt: observedAt})
		if err != nil {
			return fmt.Errorf("encode delta: %w", err)
		}
		pipe.XAdd(ctx, &goredis.XAddArgs{Stream: redis.StreamOddsDelta, MaxLen: oddsDeltaStreamMaxLen, Approx: true, Values: []any{redis.StreamField, body, redis.EventIDField, market.Key.EventID}})
	}
	r.expectPair(pair, observedAt, time.Now())
	if _, err := pipe.Exec(ctx); err != nil {
		r.retractPair(pair, observedAt)
		return fmt.Errorf("xadd %d deltas for %s: %w", len(pair.markets), pair.source, err)
	}
	return nil
}

func (r *replayer) retractPair(pair replayPair, observedAt time.Time) {
	for _, m := range pair.markets {
		stream := cellStream{marketKey: m.Key.String(), source: pair.source}
		for _, t := range r.trackers {
			t.retract(pair.eventID, stream, observedAt)
		}
	}
}

func (r *replayer) expectPair(pair replayPair, observedAt, sentAt time.Time) {
	for _, m := range pair.markets {
		stream := cellStream{marketKey: m.Key.String(), source: pair.source}
		for _, t := range r.trackers {
			t.expect(pair.eventID, stream, observedAt, sentAt)
		}
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled)
}
