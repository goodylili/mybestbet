package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	maxDeltasPerBatch      = 512
	pendingCommits         = 2
	openSignalRecheckEvery = 15 * time.Second
)

type committedBatch struct {
	changes    []changeSet
	messageIDs []string
}

type streamDelta struct {
	messageID string
	delta     canon.Delta
	freshness *canon.Freshness
	undecoded *goredis.XMessage
}

func (m streamDelta) eventID() string {
	if m.freshness != nil {
		return m.freshness.EventID
	}
	return m.delta.Market.Key.EventID
}

type shard struct {
	board   *board
	inbox   chan streamDelta
	rdb     *goredis.Client
	group   string
	prices  *priceFlusher
	signals *signalPublisher
	log     *slog.Logger
}

func (s *shard) run(ctx context.Context) error {
	committed := make(chan committedBatch, pendingCommits)
	var writer errgroup.Group
	writer.Go(func() error {
		s.writeCommitted(ctx, committed)
		return nil
	})
	s.applyUntilDone(ctx, committed)
	close(committed)
	return writer.Wait()
}

func (s *shard) applyUntilDone(ctx context.Context, committed chan<- committedBatch) {
	if !s.commit(ctx, committed, s.restoredBoard(), nil) {
		return
	}
	batch := make([]streamDelta, 0, maxDeltasPerBatch)
	recheck := time.NewTicker(openSignalRecheckEvery)
	defer recheck.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-recheck.C:
			if !s.commit(ctx, committed, []changeSet{s.board.recheckOpenSignals(time.Now())}, nil) {
				return
			}
		case msg := <-s.inbox:
			batch = s.drainInbox(append(batch[:0], msg))
			changes, messageIDs := s.applyBatch(batch)
			if !s.commit(ctx, committed, changes, messageIDs) {
				return
			}
		}
	}
}

func (s *shard) writeCommitted(ctx context.Context, committed <-chan committedBatch) {
	for c := range committed {
		if err := s.writeAndAck(ctx, c); err != nil && ctx.Err() == nil {
			s.log.Error("write delta batch", "deltas", len(c.messageIDs), "err", err)
		}
	}
}

func (s *shard) drainInbox(batch []streamDelta) []streamDelta {
	for len(batch) < cap(batch) {
		select {
		case msg := <-s.inbox:
			batch = append(batch, msg)
		default:
			return batch
		}
	}
	return batch
}

func (s *shard) restoredBoard() []changeSet {
	now := time.Now()
	changes := make([]changeSet, 0, len(s.board.byEvent)+1)
	for eventID := range s.board.byEvent {
		changes = append(changes, s.board.rescanEvent(eventID, now))
	}
	return append(changes, s.board.closeOrphanedSignals(now))
}

func (s *shard) applyBatch(batch []streamDelta) ([]changeSet, []string) {
	changes := make([]changeSet, 0, len(batch)+1)
	messageIDs := make([]string, 0, len(batch))
	started := time.Now()
	for _, msg := range batch {
		changes = append(changes, s.stageDelta(msg, started))
		messageIDs = append(messageIDs, msg.messageID)
	}
	var rescanned changeSet
	s.board.rescanStale(&rescanned, started)
	observability.PipelineLatency.WithLabelValues("engine_apply").Observe(time.Since(started).Seconds())
	return append(changes, rescanned), messageIDs
}

func (s *shard) stageDelta(msg streamDelta, now time.Time) changeSet {
	if msg.undecoded != nil {
		decoded, err := decodeMessage(*msg.undecoded)
		if err != nil {
			s.log.Error("drop undecodable delta", "message_id", msg.messageID, "err", err)
			return changeSet{}
		}
		msg = decoded
	}
	if msg.freshness != nil {
		return s.board.stageFreshness(*msg.freshness)
	}
	return s.board.stage(msg.delta, now)
}

func (s *shard) commit(ctx context.Context, committed chan<- committedBatch, changes []changeSet, messageIDs []string) bool {
	s.handOff(changes)
	select {
	case committed <- committedBatch{changes: changes, messageIDs: messageIDs}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *shard) writeAndAck(ctx context.Context, c committedBatch) error {
	started := time.Now()
	if err := s.writeBoard(ctx, mergeBoardWrites(c.changes), started); err != nil {
		return err
	}
	if err := s.ack(ctx, c.messageIDs); err != nil {
		return err
	}
	observability.PipelineLatency.WithLabelValues("engine_redis").Observe(time.Since(started).Seconds())
	return nil
}

func (s *shard) writeBoard(ctx context.Context, writes boardWrites, now time.Time) error {
	if len(writes) == 0 {
		return nil
	}
	pipe := s.rdb.Pipeline()
	if err := writes.queue(ctx, pipe, now); err != nil {
		return err
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("write board for %d events: %w", len(writes), err)
	}
	return nil
}

func (s *shard) ack(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := s.rdb.XAck(ctx, redis.StreamOddsDelta, s.group, messageIDs...).Err(); err != nil {
		return fmt.Errorf("ack %d deltas: %w", len(messageIDs), err)
	}
	return nil
}

func (s *shard) handOff(changes []changeSet) {
	rowChunks := make([][]priceRow, 0, len(changes))
	var signals []canon.Signal
	for _, cs := range changes {
		rowChunks = append(rowChunks, cs.rows)
		signals = append(signals, cs.signals...)
	}
	s.prices.enqueue(rowChunks...)
	s.signals.enqueue(signals)
}
