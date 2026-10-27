package resolve

import (
	"context"
	"fmt"
	"slices"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	oddsDeltaStreamMaxLen = 500_000
	publishChunk          = 500
)

func publishDeltas(ctx context.Context, rdb *goredis.Client, deltas []canon.Delta, confirmed []canon.Freshness) error {
	messages := make([]streamMessage, 0, len(deltas)+len(confirmed))
	for _, d := range deltas {
		messages = append(messages, streamMessage{field: redis.StreamField, eventID: d.Market.Key.EventID, body: d})
	}
	for _, f := range confirmed {
		messages = append(messages, streamMessage{field: redis.FreshnessField, eventID: f.EventID, body: f})
	}
	for chunk := range slices.Chunk(messages, publishChunk) {
		if err := publishChunkOf(ctx, rdb, chunk); err != nil {
			return fmt.Errorf("xadd %d deltas and %d freshness marks: %w", len(deltas), len(confirmed), err)
		}
	}
	return nil
}

type streamMessage struct {
	field   string
	eventID string
	body    any
}

func publishChunkOf(ctx context.Context, rdb *goredis.Client, chunk []streamMessage) error {
	pipe := rdb.Pipeline()
	for _, m := range chunk {
		if err := addToOddsStream(ctx, pipe, m.field, m.eventID, m.body); err != nil {
			return fmt.Errorf("encode %s message: %w", m.field, err)
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

func addToOddsStream(ctx context.Context, pipe goredis.Pipeliner, field, eventID string, message any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	pipe.XAdd(ctx, &goredis.XAddArgs{
		Stream: redis.StreamOddsDelta,
		MaxLen: oddsDeltaStreamMaxLen,
		Approx: true,
		Values: []any{field, body, redis.EventIDField, eventID},
	})
	return nil
}

func publishAliasUpdate(ctx context.Context, rdb *goredis.Client, u canon.AliasUpdate) error {
	body, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("encode alias update: %w", err)
	}
	if err := rdb.Publish(ctx, redis.ChannelAliasUpdate, body).Err(); err != nil {
		return fmt.Errorf("publish alias update: %w", err)
	}
	return nil
}
