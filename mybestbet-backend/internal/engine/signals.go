package engine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	signalStreamMaxLen = 100_000
	signalWriteTimeout = 5 * time.Second
	signalFlushEvery   = 100 * time.Millisecond
)

type signalInstance struct {
	dedupeKey string
	id        string
}

type signalPublisher struct {
	store *store
	rdb   *goredis.Client
	log   *slog.Logger

	mu      sync.Mutex
	pending map[signalInstance]canon.Signal
}

func newSignalPublisher(store *store, rdb *goredis.Client, log *slog.Logger) *signalPublisher {
	return &signalPublisher{store: store, rdb: rdb, log: log, pending: map[signalInstance]canon.Signal{}}
}

func (p *signalPublisher) enqueue(signals []canon.Signal) {
	if len(signals) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, sig := range signals {
		p.pending[signalInstance{dedupeKey: sig.DedupeKey, id: sig.ID}] = sig
	}
}

func (p *signalPublisher) takePending() map[signalInstance]canon.Signal {
	p.mu.Lock()
	defer p.mu.Unlock()
	taken := p.pending
	p.pending = make(map[signalInstance]canon.Signal, len(taken))
	return taken
}

func (p *signalPublisher) requeueUnlessSuperseded(failed map[signalInstance]canon.Signal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for instance, sig := range failed {
		if _, superseded := p.pending[instance]; !superseded {
			p.pending[instance] = sig
		}
	}
}

func (p *signalPublisher) run(ctx context.Context) error {
	ticker := time.NewTicker(signalFlushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			p.publishOnShutdown()
			return nil
		case <-ticker.C:
			p.publishPending(ctx)
		}
	}
}

func (p *signalPublisher) publishOnShutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
	defer cancel()
	p.publishPending(ctx)
}

func (p *signalPublisher) publishPending(ctx context.Context) {
	pending := p.takePending()
	if len(pending) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, signalWriteTimeout)
	defer cancel()
	stored, err := p.store.persistSignals(ctx, chronologicalRounds(pending))
	if err != nil {
		p.log.Error("persist signals", "signals", len(pending), "err", err)
		p.requeueUnlessSuperseded(pending)
		return
	}
	if err := p.emit(ctx, stored); err != nil {
		p.log.Error("emit signals", "signals", len(stored), "err", err)
	}
}

func chronologicalRounds(pending map[signalInstance]canon.Signal) [][]canon.Signal {
	byDedupeKey := map[string][]canon.Signal{}
	for _, sig := range pending {
		byDedupeKey[sig.DedupeKey] = append(byDedupeKey[sig.DedupeKey], sig)
	}
	var rounds [][]canon.Signal
	for _, instances := range byDedupeKey {
		slices.SortFunc(instances, compareLifetimeEnd)
		for i, sig := range instances {
			if i == len(rounds) {
				rounds = append(rounds, nil)
			}
			rounds[i] = append(rounds[i], sig)
		}
	}
	return rounds
}

func compareLifetimeEnd(a, b canon.Signal) int {
	switch {
	case a.ClosedAt == nil && b.ClosedAt == nil:
		return 0
	case a.ClosedAt == nil:
		return 1
	case b.ClosedAt == nil:
		return -1
	default:
		return a.ClosedAt.Compare(*b.ClosedAt)
	}
}

func (p *signalPublisher) emit(ctx context.Context, stored []canon.Signal) error {
	if len(stored) == 0 {
		return nil
	}
	pipe := p.rdb.Pipeline()
	byEvent := map[string][]canon.Signal{}
	for _, sig := range stored {
		encoded, err := json.Marshal(sig)
		if err != nil {
			return fmt.Errorf("encode signal %s: %w", sig.DedupeKey, err)
		}
		pipe.XAdd(ctx, &goredis.XAddArgs{Stream: redis.StreamSignals, MaxLen: signalStreamMaxLen, Approx: true, Values: []any{redis.StreamField, encoded}})
		pipe.Publish(ctx, redis.ChannelSignals, encoded)
		byEvent[sig.EventID] = append(byEvent[sig.EventID], sig)
	}
	now := time.Now()
	for eventID, signals := range byEvent {
		update, err := json.Marshal(canon.BoardUpdate{EventID: eventID, Signals: signals, SentAt: now})
		if err != nil {
			return fmt.Errorf("encode signal board update %s: %w", eventID, err)
		}
		pipe.Publish(ctx, redis.EventChannel(eventID), update)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("emit signals: %w", err)
	}
	return nil
}
