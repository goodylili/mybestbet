package engine

import (
	"context"
	"log/slog"
	"math"
	"sync/atomic"
	"time"
)

const (
	kickoffRefreshEvery = time.Minute
	unknownKickoff      = time.Duration(math.MaxInt64)
)

type eventKickoffs struct {
	store   *store
	byEvent atomic.Pointer[map[string]time.Time]
	log     *slog.Logger
}

func newEventKickoffs(store *store, log *slog.Logger) *eventKickoffs {
	k := &eventKickoffs{store: store, log: log}
	k.byEvent.Store(&map[string]time.Time{})
	return k
}

func (k *eventKickoffs) untilKickoff(eventID string, now time.Time) time.Duration {
	kickoff, ok := (*k.byEvent.Load())[eventID]
	if !ok {
		return unknownKickoff
	}
	return kickoff.Sub(now)
}

func unknownKickoffDistance(string, time.Time) time.Duration { return unknownKickoff }

func (k *eventKickoffs) refresh(ctx context.Context, now time.Time) error {
	kickoffs, err := k.store.loadKickoffs(ctx, now.Add(-restoreKickoffLookback))
	if err != nil {
		return err
	}
	k.byEvent.Store(&kickoffs)
	return nil
}

func (k *eventKickoffs) refreshUntilDone(ctx context.Context) error {
	ticker := time.NewTicker(kickoffRefreshEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := k.refresh(ctx, time.Now()); err != nil && ctx.Err() == nil {
				k.log.Warn("refresh event kickoffs", "err", err)
			}
		}
	}
}
