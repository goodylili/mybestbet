package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	readBatchSize      = 2048
	prefetchedReads    = 4
	readBlock          = 2 * time.Second
	readRetryBackoff   = time.Second
	pendingCursorStart = "0"
	newMessagesCursor  = ">"
	groupStartAtTail   = "$"
	groupExistsPrefix  = "BUSYGROUP"
	deadConsumerIdle   = 10 * time.Minute
)

func ensureConsumerGroup(ctx context.Context, rdb *goredis.Client, group string) error {
	err := rdb.XGroupCreateMkStream(ctx, redis.StreamOddsDelta, group, groupStartAtTail).Err()
	if err != nil && !strings.HasPrefix(err.Error(), groupExistsPrefix) {
		return fmt.Errorf("create consumer group %s: %w", group, err)
	}
	return nil
}

func (e *Engine) dropDeadConsumers(ctx context.Context, idleFor time.Duration) error {
	consumers, err := e.rdb.XInfoConsumers(ctx, redis.StreamOddsDelta, e.group).Result()
	if err != nil {
		return fmt.Errorf("list consumers of %s: %w", e.group, err)
	}
	for _, c := range consumers {
		if c.Name == e.consumer || c.Idle < idleFor {
			continue
		}
		if err := e.rdb.XGroupDelConsumer(ctx, redis.StreamOddsDelta, e.group, c.Name).Err(); err != nil {
			return fmt.Errorf("drop dead consumer %s: %w", c.Name, err)
		}
		e.log.Info("dropped dead consumer", "consumer", c.Name, "pending", c.Pending, "idle", c.Idle)
	}
	return nil
}

func (e *Engine) consume(ctx context.Context) error {
	reads := make(chan []goredis.XMessage, prefetchedReads)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer close(reads)
		e.readDeltas(ctx, reads)
		return nil
	})
	g.Go(func() error {
		e.routeReads(ctx, reads)
		return nil
	})
	return g.Wait()
}

func (e *Engine) readDeltas(ctx context.Context, reads chan<- []goredis.XMessage) {
	cursor := pendingCursorStart
	for {
		streams, err := e.rdb.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    e.group,
			Consumer: e.consumer,
			Streams:  []string{redis.StreamOddsDelta, cursor},
			Count:    readBatchSize,
			Block:    readBlock,
		}).Result()
		if ctx.Err() != nil {
			return
		}
		if err != nil && !errors.Is(err, goredis.Nil) {
			e.log.Warn("read odds deltas", "err", err)
			if !sleepOrDone(ctx, readRetryBackoff) {
				return
			}
			continue
		}
		messages := messagesOf(streams)
		if cursor != newMessagesCursor {
			cursor = nextPendingCursor(messages)
		}
		if len(messages) == 0 {
			continue
		}
		select {
		case reads <- messages:
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) routeReads(ctx context.Context, reads <-chan []goredis.XMessage) {
	for messages := range reads {
		for _, msg := range messages {
			if err := e.route(ctx, msg); err != nil {
				return
			}
		}
	}
}

func messagesOf(streams []goredis.XStream) []goredis.XMessage {
	if len(streams) == 0 {
		return nil
	}
	return streams[0].Messages
}

func nextPendingCursor(messages []goredis.XMessage) string {
	if len(messages) == 0 {
		return newMessagesCursor
	}
	return messages[len(messages)-1].ID
}

func (e *Engine) route(ctx context.Context, msg goredis.XMessage) error {
	routed, err := routable(msg)
	if err != nil {
		e.log.Error("drop undecodable delta", "message_id", msg.ID, "err", err)
		return e.rdb.XAck(ctx, redis.StreamOddsDelta, e.group, msg.ID).Err()
	}
	target := e.shardFor(routed.eventID)
	select {
	case target.inbox <- routed.message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type routedMessage struct {
	eventID string
	message streamDelta
}

func routable(msg goredis.XMessage) (routedMessage, error) {
	if eventID, ok := msg.Values[redis.EventIDField].(string); ok && eventID != "" {
		return routedMessage{eventID: eventID, message: streamDelta{messageID: msg.ID, undecoded: &msg}}, nil
	}
	decoded, err := decodeMessage(msg)
	return routedMessage{eventID: decoded.eventID(), message: decoded}, err
}

func decodeMessage(msg goredis.XMessage) (streamDelta, error) {
	if raw, ok := msg.Values[redis.FreshnessField].(string); ok {
		f, err := decodeFreshness(msg.ID, raw)
		return streamDelta{messageID: msg.ID, freshness: &f}, err
	}
	d, err := decodeDelta(msg)
	return streamDelta{messageID: msg.ID, delta: d}, err
}

func decodeFreshness(messageID, raw string) (canon.Freshness, error) {
	var f canon.Freshness
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return canon.Freshness{}, fmt.Errorf("decode freshness %s: %w", messageID, err)
	}
	if f.EventID == "" || f.Source == "" {
		return canon.Freshness{}, fmt.Errorf("freshness %s has no event id or source", messageID)
	}
	return f, nil
}

func decodeDelta(msg goredis.XMessage) (canon.Delta, error) {
	raw, ok := msg.Values[redis.StreamField].(string)
	if !ok {
		return canon.Delta{}, fmt.Errorf("message %s has no %q field", msg.ID, redis.StreamField)
	}
	var d canon.Delta
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return canon.Delta{}, fmt.Errorf("decode delta %s: %w", msg.ID, err)
	}
	if d.Market.Key.EventID == "" {
		return canon.Delta{}, fmt.Errorf("delta %s has no event id", msg.ID)
	}
	return d, nil
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
