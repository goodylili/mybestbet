package engine

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/pkg/canon"
)

const (
	DefaultGroup             = "engine"
	defaultShards            = 16
	defaultFlushEvery        = 500 * time.Millisecond
	defaultFlushRows         = 5000
	shardInboxSize           = 1024
	restoreKickoffLookback   = 6 * time.Hour
	partitionMaintenanceTick = time.Hour
	kickoffSweepTick         = time.Minute
	deadConsumerSweepTick    = 5 * time.Minute
)

type Engine struct {
	rdb       *goredis.Client
	store     *store
	health    *sourceHealth
	kickoffs  *eventKickoffs
	shards    []*shard
	placement eventPlacement
	prices    *priceFlusher
	signals   *signalPublisher
	group     string
	consumer  string
	log       *slog.Logger
}

func New(ctx context.Context, deps Deps) (*Engine, error) {
	e := &Engine{
		rdb:      deps.Redis,
		store:    &store{pool: deps.Pool},
		group:    orDefault(deps.Group, DefaultGroup),
		consumer: orDefault(deps.Consumer, DefaultGroup),
		log:      deps.Log,
	}
	priceSpool, err := openSpool(deps.SpoolDir)
	if err != nil {
		return nil, err
	}
	e.prices = newPriceFlusher(e.store, priceSpool, positiveOr(deps.Config.FlushEvery, defaultFlushEvery), positiveOr(deps.Config.FlushRows, defaultFlushRows), deps.Log)
	e.signals = newSignalPublisher(e.store, deps.Redis, deps.Log)
	e.health = newSourceHealth(deps.Redis, slices.Sorted(maps.Keys(deps.Config.Sources)), deps.Log)
	if err := ensureConsumerGroup(ctx, deps.Redis, e.group); err != nil {
		return nil, err
	}
	if err := e.dropDeadConsumers(ctx, deadConsumerIdle); err != nil {
		e.log.Warn("drop dead consumers", "err", err)
	}
	if err := e.store.ensureHistoryPartitions(ctx, time.Now()); err != nil {
		e.log.Warn("ensure history partitions", "err", err)
	}
	if err := e.health.refresh(ctx); err != nil {
		e.log.Warn("initial source health", "err", err)
	}
	e.kickoffs = newEventKickoffs(e.store, deps.Log)
	if err := e.kickoffs.refresh(ctx, time.Now()); err != nil {
		e.log.Warn("initial event kickoffs", "err", err)
	}
	e.shards = make([]*shard, positiveOr(deps.Config.EngineShards, defaultShards))
	e.placement = newEventPlacement(len(e.shards))
	for i := range e.shards {
		b := newBoard(deps.Config, e.health.isDegraded)
		b.untilKickoff = e.kickoffs.untilKickoff
		e.shards[i] = &shard{
			board:   b,
			inbox:   make(chan streamDelta, shardInboxSize),
			rdb:     deps.Redis,
			group:   e.group,
			prices:  e.prices,
			signals: e.signals,
			log:     deps.Log,
		}
	}
	if err := e.restore(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) shardFor(eventID string) *shard {
	return e.shards[e.placement.shardOf(eventID)]
}

func (e *Engine) restore(ctx context.Context) error {
	open, err := e.store.loadOpenSignals(ctx)
	if err != nil {
		return fmt.Errorf("restore engine: %w", err)
	}
	for _, sig := range open {
		e.shardFor(sig.EventID).board.adoptOpenSignal(sig)
	}
	now := time.Now()
	err = e.store.loadCurrentPrices(ctx, now.Add(-restoreKickoffLookback), func(d canon.Delta) error {
		e.shardFor(d.Market.Key.EventID).board.restore(d, now)
		return nil
	})
	if err != nil {
		return fmt.Errorf("restore engine: %w", err)
	}
	return nil
}

func (e *Engine) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, s := range e.shards {
		g.Go(func() error { return s.run(ctx) })
	}
	g.Go(func() error { return e.prices.run(ctx) })
	g.Go(func() error { return e.signals.run(ctx) })
	g.Go(func() error { return e.health.refreshUntilDone(ctx) })
	g.Go(func() error { return e.kickoffs.refreshUntilDone(ctx) })
	g.Go(func() error { return e.maintainPartitions(ctx) })
	g.Go(func() error { return e.closeSignalsAtKickoff(ctx) })
	g.Go(func() error { return e.dropDeadConsumersPeriodically(ctx) })
	g.Go(func() error { return e.consume(ctx) })
	return g.Wait()
}

func (e *Engine) maintainPartitions(ctx context.Context) error {
	ticker := time.NewTicker(partitionMaintenanceTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := e.store.ensureHistoryPartitions(ctx, time.Now()); err != nil {
				e.log.Warn("ensure history partitions", "err", err)
			}
		}
	}
}

func (e *Engine) dropDeadConsumersPeriodically(ctx context.Context) error {
	ticker := time.NewTicker(deadConsumerSweepTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := e.dropDeadConsumers(ctx, deadConsumerIdle); err != nil && ctx.Err() == nil {
				e.log.Warn("drop dead consumers", "err", err)
			}
		}
	}
}

func (e *Engine) closeSignalsAtKickoff(ctx context.Context) error {
	ticker := time.NewTicker(kickoffSweepTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			e.closeStartedEventSignals(ctx, time.Now())
		}
	}
}

func (e *Engine) closeStartedEventSignals(ctx context.Context, now time.Time) {
	started, err := e.store.loadOpenSignalsOfStartedEvents(ctx, now)
	if err != nil {
		e.log.Warn("load signals of started events", "err", err)
		return
	}
	for i := range started {
		started[i].Status, started[i].ClosedAt = canon.SignalClosed, new(now)
	}
	e.signals.enqueue(started)
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func positiveOr[T int | time.Duration](v, fallback T) T {
	if v <= 0 {
		return fallback
	}
	return v
}
