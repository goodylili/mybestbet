package loadtest

import (
	"context"
	"fmt"
	"time"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const boardUpdateBuffer = 1 << 16

func subscribeEventChannels(ctx context.Context, rdb *goredis.Client, eventIDs []string) (*goredis.PubSub, error) {
	channels := make([]string, len(eventIDs))
	for i, id := range eventIDs {
		channels[i] = redis.EventChannel(id)
	}
	pubsub := rdb.Subscribe(ctx, channels...)
	for range channels {
		if _, err := pubsub.Receive(ctx); err != nil {
			_ = pubsub.Close()
			return nil, fmt.Errorf("subscribe event channels: %w", err)
		}
	}
	return pubsub, nil
}

func observePubSub(ctx context.Context, pubsub *goredis.PubSub, t *tracker) error {
	messages := pubsub.Channel(goredis.WithChannelSize(boardUpdateBuffer))
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			receivedAt := time.Now()
			var update canon.BoardUpdate
			if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
				return fmt.Errorf("decode board update on %s: %w", msg.Channel, err)
			}
			t.observeUpdate(update, receivedAt)
		}
	}
}
