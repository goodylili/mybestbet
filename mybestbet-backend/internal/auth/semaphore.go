package auth

import (
	"context"
	"time"
)

type semaphore struct {
	slots chan struct{}
	wait  time.Duration
}

func newSemaphore(size int, wait time.Duration) *semaphore {
	return &semaphore{slots: make(chan struct{}, size), wait: wait}
}

func (s *semaphore) acquire(ctx context.Context) (func(), error) {
	timer := time.NewTimer(s.wait)
	defer timer.Stop()
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-timer.C:
		return nil, ErrBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
