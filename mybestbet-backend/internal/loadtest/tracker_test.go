package loadtest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

const (
	watchedEvent   = "11111111-1111-1111-1111-111111111111"
	unwatchedEvent = "22222222-2222-2222-2222-222222222222"
	testMarketKey  = watchedEvent + "|1X2||||"
)

var (
	sendBase      = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	testStream    = cellStream{marketKey: testMarketKey, source: "bet9ja"}
	firstStamp    = sendBase
	secondStamp   = sendBase.Add(500 * time.Millisecond)
	testDropAfter = 2 * time.Second
)

func boardUpdate(eventID string, cells ...canon.BoardCell) canon.BoardUpdate {
	return canon.BoardUpdate{EventID: eventID, Cells: cells}
}

func cellAt(source string, observedAt time.Time) canon.BoardCell {
	return canon.BoardCell{MarketKey: testMarketKey, Source: source, Side: canon.SideHome, Price: canon.Price{Raw: 2, ObservedAt: observedAt}}
}

func TestTracker_Report(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		isSupersedeDelivered bool
		updates              []canon.BoardUpdate
		receivedAfterSend    time.Duration
		expected             Report
	}{
		{
			name:              "every delta matched by its exact stamp",
			updates:           []canon.BoardUpdate{boardUpdate(watchedEvent, cellAt("bet9ja", firstStamp)), boardUpdate(watchedEvent, cellAt("bet9ja", secondStamp))},
			receivedAfterSend: 40 * time.Millisecond,
			expected:          Report{Sent: 2, Observed: 2, P50: 40 * time.Millisecond, P95: 40 * time.Millisecond, P99: 40 * time.Millisecond, Max: 40 * time.Millisecond},
		},
		{
			name:              "repeated cell of one delta counts once",
			updates:           []canon.BoardUpdate{boardUpdate(watchedEvent, cellAt("bet9ja", firstStamp), cellAt("bet9ja", firstStamp))},
			receivedAfterSend: 10 * time.Millisecond,
			expected:          Report{Sent: 2, Observed: 1, Dropped: 1, P50: 10 * time.Millisecond, P95: 10 * time.Millisecond, P99: 10 * time.Millisecond, Max: 10 * time.Millisecond},
		},
		{
			name:              "newer stamp does not settle an older delta on the redis path",
			updates:           []canon.BoardUpdate{boardUpdate(watchedEvent, cellAt("bet9ja", secondStamp))},
			receivedAfterSend: 10 * time.Millisecond,
			expected:          Report{Sent: 2, Observed: 1, Dropped: 1, P50: 10 * time.Millisecond, P95: 10 * time.Millisecond, P99: 10 * time.Millisecond, Max: 10 * time.Millisecond},
		},
		{
			name:                 "coalesced stream settles the superseded delta",
			isSupersedeDelivered: true,
			updates:              []canon.BoardUpdate{boardUpdate(watchedEvent, cellAt("bet9ja", secondStamp))},
			receivedAfterSend:    10 * time.Millisecond,
			expected:             Report{Sent: 2, Observed: 2, P50: 10 * time.Millisecond, P95: 10 * time.Millisecond, P99: 10 * time.Millisecond, Max: 10 * time.Millisecond},
		},
		{
			name:              "other source, consensus and unwatched events are ignored",
			updates:           []canon.BoardUpdate{boardUpdate(watchedEvent, cellAt("betking", firstStamp), cellAt("consensus", firstStamp)), boardUpdate(unwatchedEvent, cellAt("bet9ja", firstStamp))},
			receivedAfterSend: 10 * time.Millisecond,
			expected:          Report{Sent: 2, Dropped: 2},
		},
		{
			name:              "observation later than drop window is a late drop that still counts in percentiles",
			updates:           []canon.BoardUpdate{boardUpdate(watchedEvent, cellAt("bet9ja", firstStamp))},
			receivedAfterSend: 3 * time.Second,
			expected:          Report{Sent: 2, Late: 1, Dropped: 2, P50: 3 * time.Second, P95: 3 * time.Second, P99: 3 * time.Second, Max: 3 * time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tr := newTracker(PathRedis, []string{watchedEvent}, testDropAfter, tt.isSupersedeDelivered)
			sentAt := time.Unix(0, 0)
			tr.expect(watchedEvent, testStream, firstStamp, sentAt)
			tr.expect(watchedEvent, testStream, secondStamp, sentAt)
			tr.expect(unwatchedEvent, testStream, firstStamp, sentAt)
			for _, u := range tt.updates {
				tr.observeUpdate(u, sentAt.Add(tt.receivedAfterSend))
			}
			tt.expected.Path = PathRedis
			assert.Equal(t, tt.expected, tr.report())
		})
	}
}

func TestTracker_Outstanding(t *testing.T) {
	t.Parallel()
	tr := newTracker(PathRedis, []string{watchedEvent}, testDropAfter, false)
	tr.expect(watchedEvent, testStream, firstStamp, sendBase)
	tr.expect(watchedEvent, testStream, secondStamp, sendBase)
	assert.Equal(t, 2, tr.outstanding())
	tr.observeUpdate(boardUpdate(watchedEvent, cellAt("bet9ja", secondStamp)), sendBase)
	tr.observeUpdate(boardUpdate(watchedEvent, cellAt("bet9ja", firstStamp)), sendBase)
	assert.Equal(t, 0, tr.outstanding())
}

func TestTrackerRetractsSendsThatNeverReachedRedis(t *testing.T) {
	tr := newTracker(PathRedis, []string{"ev"}, time.Second, false)
	stream := cellStream{marketKey: "ev|1X2||||", source: "betking"}
	at := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	tr.expect("ev", stream, at, at)
	tr.expect("ev", stream, at.Add(time.Millisecond), at)
	tr.retract("ev", stream, at.Add(time.Millisecond))

	if tr.sent != 1 || tr.outstanding() != 1 {
		t.Fatalf("sent=%d outstanding=%d, want 1 and 1", tr.sent, tr.outstanding())
	}
}
