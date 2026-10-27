package engine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

func drawOnly(cells []canon.BoardCell, source string) []canon.BoardCell {
	return slices.DeleteFunc(slices.Clone(cells), func(c canon.BoardCell) bool {
		return c.Source != source || c.Side != canon.SideDraw || c.MarketKey != matchResult.String()
	})
}

func TestMergeBoardWritesKeepsEventOrderAndFinalCellState(t *testing.T) {
	t.Parallel()
	b := newTestBoard()
	withDraw := b.apply(matchResultDelta("bet9ja", 2.3, 3.4, 3.2, testNow), testNow)
	withoutDraw := b.apply(decimalDelta("bet9ja", matchResult, testNow, sel(canon.SideHome, "", 2.3, testNow), sel(canon.SideAway, "", 3.2, testNow)), testNow)
	drawBack := b.apply(matchResultDelta("bet9ja", 2.3, 3.5, 3.2, testNow), testNow)
	otherKey := canon.MarketKey{EventID: "ev2", Code: canon.Code1X2}
	otherEvent := b.apply(decimalDelta("bet9ja", otherKey, testNow, sel(canon.SideHome, "", 1.9, testNow), sel(canon.SideAway, "", 2.0, testNow)), testNow)
	drawKey := cellKey{source: "bet9ja", side: canon.SideDraw}

	tests := []struct {
		name          string
		changes       []changeSet
		drawPublished []float64
		isDrawRemoved bool
	}{
		{name: "removal after add deletes the cell", changes: []changeSet{withDraw, withoutDraw}, drawPublished: []float64{3.4, 0}, isDrawRemoved: true},
		{name: "re-add after removal keeps the newest cell", changes: []changeSet{withDraw, withoutDraw, drawBack}, drawPublished: []float64{3.4, 0, 3.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writes := mergeBoardWrites(append(slices.Clone(tt.changes), otherEvent))
			require.Len(t, writes, 2)
			ev := writes[matchResult.EventID]
			var published []float64
			for _, c := range drawOnly(ev.published, "bet9ja") {
				published = append(published, c.Price.Raw)
			}
			assert.Equal(t, tt.drawPublished, published)
			latest := ev.latestByMarket[matchResult.String()][drawKey]
			assert.Equal(t, tt.isDrawRemoved, latest.isRemoved)
			assert.InDelta(t, tt.drawPublished[len(tt.drawPublished)-1], latest.cell.Price.Raw, 1e-9)
			assert.NotContains(t, writes[matchResult.EventID].latestByMarket, otherKey.String())
		})
	}
}

func TestEventPlacementBalancesEventsAcrossShards(t *testing.T) {
	t.Parallel()
	const shards, events = 16, 40
	p := newEventPlacement(shards)
	for i := range events {
		first := p.shardOf(fmt.Sprintf("event-%d", i))
		assert.Equal(t, first, p.shardOf(fmt.Sprintf("event-%d", i)))
	}
	assert.Equal(t, (events+shards-1)/shards, slices.Max(p.eventsOnShard))
}

func TestShardAcksOnlyAfterBoardWriteSucceeds(t *testing.T) {
	_, rdb := integrationDeps(t)
	ctx := context.Background()
	group := "engine-ack-test-" + uuid.NewString()
	require.NoError(t, ensureConsumerGroup(ctx, rdb, group))
	key := canon.MarketKey{EventID: uuid.NewString(), Code: canon.Code1X2}
	t.Cleanup(func() {
		_ = rdb.XGroupDestroy(ctx, redis.StreamOddsDelta, group).Err()
		for _, code := range []canon.MarketCode{canon.Code1X2, canon.CodeDC, canon.CodeDNB} {
			_ = rdb.Del(ctx, redis.BoardKey(canon.MarketKey{EventID: key.EventID, Code: code}.String())).Err()
		}
		_ = rdb.Del(ctx, redis.EventMarketsKey(key.EventID)).Err()
	})
	d := matchResultDelta("itest-ack", 2.1, 3.3, 3.6, time.Now())
	d.Market.Key = key
	publishDelta(t, rdb, d)
	_, err := rdb.XReadGroup(ctx, &goredis.XReadGroupArgs{Group: group, Consumer: group, Streams: []string{redis.StreamOddsDelta, ">"}, Count: 10_000}).Result()
	require.NoError(t, err)
	pending, err := rdb.XPendingExt(ctx, &goredis.XPendingExtArgs{Stream: redis.StreamOddsDelta, Group: group, Start: "-", End: "+", Count: 10_000}).Result()
	require.NoError(t, err)
	messageID := ""
	for _, p := range pending {
		messageID = p.ID
	}
	require.NotEmpty(t, messageID)

	s := &shard{board: newTestBoard(), rdb: rdb, group: group, log: slog.New(slog.DiscardHandler)}
	changes, messageIDs := s.applyBatch([]streamDelta{{messageID: messageID, delta: d}})
	batch := committedBatch{changes: changes, messageIDs: messageIDs}
	isPending := func() bool {
		n, err := rdb.XPendingExt(ctx, &goredis.XPendingExtArgs{Stream: redis.StreamOddsDelta, Group: group, Start: messageID, End: messageID, Count: 1}).Result()
		require.NoError(t, err)
		return len(n) == 1
	}

	require.NoError(t, rdb.Set(ctx, redis.BoardKey(key.String()), "not a hash", 0).Err())
	require.Error(t, s.writeAndAck(ctx, batch))
	assert.True(t, isPending())

	require.NoError(t, rdb.Del(ctx, redis.BoardKey(key.String())).Err())
	require.NoError(t, s.writeAndAck(ctx, batch))
	assert.False(t, isPending())
	home, err := rdb.HGet(ctx, redis.BoardKey(key.String()), redis.BoardField("itest-ack", string(canon.SideHome), "")).Result()
	require.NoError(t, err)
	assert.Contains(t, home, `"raw":2.1`)
}

func arbSignal(dedupeKey string, margin float64, detectedAt time.Time) canon.Signal {
	return canon.Signal{
		ID:         signalID(dedupeKey, detectedAt),
		DedupeKey:  dedupeKey,
		Type:       canon.SignalArb,
		Status:     canon.SignalOpen,
		EventID:    matchResult.EventID,
		Market:     matchResult,
		Margin:     margin,
		DetectedAt: detectedAt,
	}
}

func closedAt(sig canon.Signal, at time.Time) canon.Signal {
	sig.Status, sig.ClosedAt = canon.SignalClosed, &at
	return sig
}

func TestSignalPublisherCoalescesUpdatesOfOneSignal(t *testing.T) {
	t.Parallel()
	p := newSignalPublisher(nil, nil, slog.New(slog.DiscardHandler))
	opened := arbSignal("arb|a", 0.01, testNow)
	updated := opened
	updated.Margin = 0.02
	p.enqueue([]canon.Signal{opened, updated})
	p.enqueue([]canon.Signal{arbSignal("arb|b", 0.03, testNow)})

	pending := p.takePending()
	require.Len(t, pending, 2)
	assert.InDelta(t, 0.02, pending[signalInstance{dedupeKey: "arb|a", id: opened.ID}].Margin, 1e-9)
	assert.Empty(t, p.takePending())
}

func TestSignalPublisherRequeueKeepsNewerState(t *testing.T) {
	t.Parallel()
	p := newSignalPublisher(nil, nil, slog.New(slog.DiscardHandler))
	opened := arbSignal("arb|a", 0.01, testNow)
	failed := p.takePending()
	failed[signalInstance{dedupeKey: opened.DedupeKey, id: opened.ID}] = opened
	p.enqueue([]canon.Signal{closedAt(opened, testNow.Add(time.Second))})

	p.requeueUnlessSuperseded(failed)
	pending := p.takePending()
	require.Len(t, pending, 1)
	assert.Equal(t, canon.SignalClosed, pending[signalInstance{dedupeKey: opened.DedupeKey, id: opened.ID}].Status)
}

func TestChronologicalRoundsCloseBeforeReopen(t *testing.T) {
	t.Parallel()
	first := arbSignal("arb|a", 0.01, testNow)
	reopened := arbSignal("arb|a", 0.02, testNow.Add(2*time.Second))
	other := arbSignal("arb|b", 0.03, testNow)
	pending := map[signalInstance]canon.Signal{}
	for _, sig := range []canon.Signal{reopened, closedAt(first, testNow.Add(time.Second)), other} {
		pending[signalInstance{dedupeKey: sig.DedupeKey, id: sig.ID}] = sig
	}

	rounds := chronologicalRounds(pending)
	require.Len(t, rounds, 2)
	firstRound := map[string]canon.SignalStatus{}
	for _, sig := range rounds[0] {
		firstRound[sig.DedupeKey] = sig.Status
	}
	assert.Equal(t, map[string]canon.SignalStatus{"arb|a": canon.SignalClosed, "arb|b": canon.SignalOpen}, firstRound)
	require.Len(t, rounds[1], 1)
	assert.Equal(t, reopened.ID, rounds[1][0].ID)
}

func TestPersistSignalsBatchIsReplaySafe(t *testing.T) {
	pool, _ := integrationDeps(t)
	ctx := context.Background()
	eventID := createTestEvent(t, pool, time.Now().Add(time.Hour))
	st := &store{pool: pool}
	now := time.Now().UTC().Truncate(time.Millisecond)
	key := canon.MarketKey{EventID: eventID, Code: canon.Code1X2}
	instance := func(margin float64, at time.Time) canon.Signal {
		sig := arbSignal("arb|"+eventID, margin, at)
		sig.EventID, sig.Market = eventID, key
		return sig
	}
	fleeting := instance(0.01, now)
	reopened := instance(0.02, now.Add(2*time.Second))
	pending := map[signalInstance]canon.Signal{}
	for _, sig := range []canon.Signal{closedAt(fleeting, now.Add(time.Second)), reopened} {
		pending[signalInstance{dedupeKey: sig.DedupeKey, id: sig.ID}] = sig
	}

	emitted, err := st.persistSignals(ctx, chronologicalRounds(pending))
	require.NoError(t, err)
	var statuses []canon.SignalStatus
	for _, sig := range emitted {
		statuses = append(statuses, sig.Status)
	}
	assert.Equal(t, []canon.SignalStatus{canon.SignalOpen, canon.SignalClosed, canon.SignalOpen}, statuses)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1 AND status = 'closed'`, eventID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1 AND status = 'open'`, eventID))

	replayed, err := st.persistSignals(ctx, chronologicalRounds(pending))
	require.NoError(t, err)
	assert.Len(t, replayed, 1)
	assert.Equal(t, canon.SignalOpen, replayed[0].Status)
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1`, eventID))
}
