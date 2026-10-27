package ingest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"mybestbet/pkg/canon"
)

const (
	streamReconcileInterval = 5 * time.Minute
	streamBackoffBase       = 2 * time.Second
	streamBackoffCap        = time.Minute
	streamStableAfter       = time.Minute
	streamIdleRecheck       = 30 * time.Second
	streamBufferSize        = 64
)

var (
	errStreamRestart = errors.New("stream restart requested")
	errStreamEnded   = errors.New("stream ended")
)

func (r *runner) signalStreamRestart() {
	if r.streamer == nil {
		return
	}
	select {
	case r.streamRestart <- struct{}{}:
	default:
	}
}

func (r *runner) holdStream(ctx context.Context) error {
	failures := 0
	for ctx.Err() == nil {
		ids := r.schedule.ids()
		if len(ids) == 0 {
			r.waitForRestart(ctx, streamIdleRecheck)
			continue
		}
		started := time.Now()
		err := r.streamOnce(ctx, ids)
		r.isStreamLive.Store(false)
		r.schedule.pollAllNow()
		switch {
		case ctx.Err() != nil:
			return nil
		case isPollOnly(err):
			r.log.Info("source has no stream, polling only", "source", r.name, "reason", err)
			return nil
		case errors.Is(err, errStreamRestart):
			failures = 0
			continue
		case time.Since(started) > streamStableAfter:
			failures = 0
		}
		failures++
		r.log.Warn("stream dropped, polling hot tier until reconnect", "source", r.name, "failures", failures, "error", err)
		r.waitForRestart(ctx, streamBackoff(failures))
	}
	return nil
}

type pollOnlySource interface {
	PollsInstead() bool
}

func isPollOnly(err error) bool {
	var pollOnly pollOnlySource
	return errors.As(err, &pollOnly) && pollOnly.PollsInstead()
}

func (r *runner) waitForRestart(ctx context.Context, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-r.streamRestart:
	case <-timer.C:
	}
}

func streamBackoff(failures int) time.Duration {
	delay := streamBackoffBase
	for range failures - 1 {
		delay *= 2
		if delay >= streamBackoffCap {
			return streamBackoffCap
		}
	}
	return delay
}

func (r *runner) streamOnce(ctx context.Context, eventIDs []string) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	snapshots := make(chan canon.Snapshot, streamBufferSize)
	streamDone := make(chan error, 1)
	go func() { streamDone <- r.streamer.Stream(streamCtx, eventIDs, snapshots) }()
	r.resyncFromREST(ctx, eventIDs)
	r.isStreamLive.Store(true)
	for {
		select {
		case <-ctx.Done():
			return stopStream(cancel, streamDone, snapshots, ctx.Err())
		case <-r.streamRestart:
			return stopStream(cancel, streamDone, snapshots, errStreamRestart)
		case err := <-streamDone:
			if err == nil {
				return errStreamEnded
			}
			return fmt.Errorf("%w: %w", errStreamEnded, err)
		case snap := <-snapshots:
			r.health.notePrice(newestObservation(snap))
			r.ingest(ctx, snap, false)
		}
	}
}

func stopStream(cancel context.CancelFunc, streamDone <-chan error, snapshots <-chan canon.Snapshot, reason error) error {
	cancel()
	for {
		select {
		case err := <-streamDone:
			if err == nil || errors.Is(err, context.Canceled) {
				return reason
			}
			return errors.Join(reason, err)
		case <-snapshots:
		}
	}
}

func (r *runner) resyncFromREST(ctx context.Context, eventIDs []string) {
	timeout := fetchTimeoutFor(r.tiers[hotTier].Interval)
	for chunk := range slices.Chunk(eventIDs, resyncChunk) {
		if ctx.Err() != nil {
			return
		}
		r.fetchPrices(ctx, timeout, chunk)
	}
}
