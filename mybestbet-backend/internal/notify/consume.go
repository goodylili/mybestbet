package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	ConsumerGroup      = "notifier"
	readBatch          = 50
	readBlock          = 5 * time.Second
	pendingFromStart   = "0"
	onlyNewForConsumer = ">"
)

func (n *Notifier) consumeSignals(ctx context.Context) error {
	if err := n.ensureConsumerGroup(ctx); err != nil {
		return err
	}
	if err := n.drainStream(ctx, pendingFromStart); err != nil {
		return err
	}
	for ctx.Err() == nil {
		if err := n.drainStream(ctx, onlyNewForConsumer); err != nil {
			n.log.WarnContext(ctx, "read signals failed", "err", err)
			sleepContext(ctx, pollRetryDelay)
		}
	}
	return nil
}

func (n *Notifier) ensureConsumerGroup(ctx context.Context) error {
	err := n.rdb.XGroupCreateMkStream(ctx, redis.StreamSignals, ConsumerGroup, "$").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

func (n *Notifier) drainStream(ctx context.Context, startID string) error {
	for {
		streams, err := n.rdb.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: n.consumer,
			Streams:  []string{redis.StreamSignals, startID},
			Count:    readBatch,
			Block:    readBlock,
		}).Result()
		if errors.Is(err, goredis.Nil) || ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read signals: %w", err)
		}
		delivered := 0
		for _, stream := range streams {
			delivered += len(stream.Messages)
			for _, msg := range stream.Messages {
				n.alertSignalMessage(ctx, msg)
				if err := n.rdb.XAck(ctx, redis.StreamSignals, ConsumerGroup, msg.ID).Err(); err != nil {
					return fmt.Errorf("ack signal: %w", err)
				}
			}
		}
		if startID == onlyNewForConsumer || delivered == 0 {
			return nil
		}
	}
}

func (n *Notifier) alertSignalMessage(ctx context.Context, msg goredis.XMessage) {
	encoded, ok := msg.Values[redis.StreamField].(string)
	if !ok {
		n.log.WarnContext(ctx, "signal message without payload", "message_id", msg.ID)
		return
	}
	var signal canon.Signal
	if err := json.Unmarshal([]byte(encoded), &signal); err != nil {
		n.log.WarnContext(ctx, "undecodable signal", "message_id", msg.ID, "err", err)
		return
	}
	if err := n.alertSubscribers(ctx, signal); err != nil {
		n.log.ErrorContext(ctx, "alert signal", "event_id", signal.EventID, "signal_id", signal.ID, "err", err)
	}
}

func (n *Notifier) alertSubscribers(ctx context.Context, signal canon.Signal) error {
	if signal.Status != canon.SignalOpen {
		return nil
	}
	subs, err := n.subscriptions(ctx)
	if err != nil {
		return err
	}
	now := n.clock()
	sport := sportOf(signal, n.leagueSports)
	var message string
	for _, sub := range subs {
		if !sub.wants(signal, sport, now) {
			continue
		}
		if message == "" {
			details, err := n.alertContext(ctx, signal.EventID, signal.Market.PlayerID)
			if err != nil {
				return err
			}
			message = formatAlert(signal, sport, details, n.webBaseURL)
		}
		if err := n.deliver(ctx, sub.ChatID, signal, message); err != nil {
			n.log.WarnContext(ctx, "deliver alert", "chat_id", sub.ChatID, "event_id", signal.EventID, "err", err)
		}
	}
	return nil
}

func (n *Notifier) deliver(ctx context.Context, chatID int64, signal canon.Signal, message string) error {
	outcome, claim, err := n.limiter.claim(ctx, signal.DedupeKey, chatID, signalStrength(signal))
	if err != nil || outcome != claimGranted {
		return err
	}
	if err := n.bot.sendMessage(ctx, chatID, message); err != nil {
		return errors.Join(err, n.limiter.release(context.WithoutCancel(ctx), claim))
	}
	return nil
}
