package ingest

import (
	"errors"
	"sync"
	"time"

	"mybestbet/pkg/sources"
)

const (
	degradedAfterFailures = 3
	backoffBase           = 5 * time.Second
	backoffCap            = 5 * time.Minute
	maxLastErrorLength    = 300
)

type health struct {
	mu                  sync.Mutex
	lastSuccess         time.Time
	lastError           string
	consecutiveFailures int
	requests            int64
	errors              int64
	lastFetch           time.Duration
	newestPrice         time.Time
	pausedUntil         time.Time
}

func (h *health) recordSuccess(now time.Time, took time.Duration, newestPrice time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests++
	h.lastSuccess = now
	h.lastFetch = took
	h.consecutiveFailures = 0
	h.pausedUntil = time.Time{}
	h.notePriceLocked(newestPrice)
}

func (h *health) recordFailure(now time.Time, took time.Duration, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests++
	h.errors++
	h.lastFetch = took
	h.consecutiveFailures++
	h.lastError = truncate(err.Error(), maxLastErrorLength)
	if isBackoffError(err) {
		h.pausedUntil = now.Add(backoffAfter(h.consecutiveFailures))
	}
}

func (h *health) notePrice(observedAt time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notePriceLocked(observedAt)
}

func (h *health) notePriceLocked(observedAt time.Time) {
	if observedAt.After(h.newestPrice) {
		h.newestPrice = observedAt
	}
}

func (h *health) isPaused(now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return now.Before(h.pausedUntil)
}

func (h *health) isDegraded() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.consecutiveFailures >= degradedAfterFailures
}

func (h *health) redisFields(now time.Time) map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	fields := map[string]any{
		"lastSuccess":         "",
		"lastError":           h.lastError,
		"consecutiveFailures": h.consecutiveFailures,
		"degraded":            boolFlag(h.consecutiveFailures >= degradedAfterFailures),
		"requests":            h.requests,
		"errors":              h.errors,
		"lastFetchMs":         h.lastFetch.Milliseconds(),
		"freshnessMs":         0,
	}
	if !h.lastSuccess.IsZero() {
		fields["lastSuccess"] = h.lastSuccess.Format(time.RFC3339Nano)
	}
	if !h.newestPrice.IsZero() {
		fields["freshnessMs"] = now.Sub(h.newestPrice).Milliseconds()
	}
	return fields
}

func isBackoffError(err error) bool {
	return errors.Is(err, sources.ErrThrottled) || errors.Is(err, sources.ErrForbidden)
}

func backoffAfter(consecutiveFailures int) time.Duration {
	if consecutiveFailures < 1 {
		return 0
	}
	delay := backoffBase
	for range consecutiveFailures - 1 {
		delay *= 2
		if delay >= backoffCap {
			return backoffCap
		}
	}
	return delay
}

func boolFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}
