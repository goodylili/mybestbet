package server

import (
	"context"
	"sync"
	"time"
)

const (
	snapshotTimeout = 30 * time.Second
	minRefreshGap   = 500 * time.Millisecond
)

type snapshot[T any] struct {
	mu     sync.RWMutex
	value  T
	loaded bool
}

func (s *snapshot[T]) get() (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value, s.loaded
}

func (s *snapshot[T]) set(value T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value, s.loaded = value, true
}

func wake(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *Server) keepRefreshed(ctx context.Context, name string, interval time.Duration, wakeup <-chan struct{}, refresh func(context.Context) error) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	gap := time.NewTimer(0)
	defer gap.Stop()
	for {
		refreshCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
		if err := refresh(refreshCtx); err != nil && ctx.Err() == nil {
			s.log.WarnContext(ctx, "refresh snapshot", "snapshot", name, "err", err)
		}
		cancel()
		gap.Reset(minRefreshGap)
		select {
		case <-ctx.Done():
			return nil
		case <-gap.C:
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-wakeup:
		}
	}
}
