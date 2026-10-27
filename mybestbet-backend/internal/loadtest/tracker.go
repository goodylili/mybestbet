package loadtest

import (
	"sync"
	"time"

	"mybestbet/pkg/canon"
)

type cellStream struct {
	marketKey string
	source    string
}

type sentDelta struct {
	stamp  int64
	sentAt time.Time
}

type tracker struct {
	path                 string
	watchedEvents        map[string]bool
	dropAfter            time.Duration
	isSupersedeDelivered bool

	mu        sync.Mutex
	pending   map[cellStream][]sentDelta
	latencies []time.Duration
	sent      int
	late      int
}

func newTracker(path string, eventIDs []string, dropAfter time.Duration, isSupersedeDelivered bool) *tracker {
	watched := make(map[string]bool, len(eventIDs))
	for _, id := range eventIDs {
		watched[id] = true
	}
	return &tracker{path: path, watchedEvents: watched, dropAfter: dropAfter, isSupersedeDelivered: isSupersedeDelivered, pending: map[cellStream][]sentDelta{}}
}

func (t *tracker) expect(eventID string, stream cellStream, observedAt, sentAt time.Time) {
	if !t.watchedEvents[eventID] {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[stream] = append(t.pending[stream], sentDelta{stamp: observedAt.UnixNano(), sentAt: sentAt})
	t.sent++
}

func (t *tracker) retract(eventID string, stream cellStream, observedAt time.Time) {
	if !t.watchedEvents[eventID] {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	stamp := observedAt.UnixNano()
	queue := t.pending[stream]
	for i, sent := range queue {
		if sent.stamp == stamp {
			t.pending[stream] = append(queue[:i], queue[i+1:]...)
			t.sent--
			return
		}
	}
}

func (t *tracker) observeUpdate(update canon.BoardUpdate, receivedAt time.Time) {
	if !t.watchedEvents[update.EventID] {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range update.Cells {
		t.observe(cellStream{marketKey: c.MarketKey, source: c.Source}, c.Price.ObservedAt.UnixNano(), receivedAt)
	}
}

func (t *tracker) observe(stream cellStream, stamp int64, receivedAt time.Time) {
	queue := t.pending[stream]
	kept := queue[:0]
	for _, sent := range queue {
		switch {
		case sent.stamp == stamp, sent.stamp < stamp && t.isSupersedeDelivered:
			t.record(receivedAt.Sub(sent.sentAt))
		default:
			kept = append(kept, sent)
		}
	}
	if len(kept) == 0 {
		delete(t.pending, stream)
		return
	}
	t.pending[stream] = kept
}

func (t *tracker) record(latency time.Duration) {
	if latency > t.dropAfter {
		t.late++
	}
	t.latencies = append(t.latencies, latency)
}

func (t *tracker) outstanding() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, queue := range t.pending {
		n += len(queue)
	}
	return n
}

func (t *tracker) report() Report {
	unseen := t.outstanding()
	t.mu.Lock()
	defer t.mu.Unlock()
	return newReport(t.path, t.sent, t.late, unseen, t.latencies)
}
