package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

const (
	StreamOddsDelta    = "odds.delta"
	StreamSignals      = "signals"
	ChannelAliasUpdate = "alias.updated"
	ChannelSignals     = "signals.live"
	ChannelBoard       = "board.live"
	EventChannelGlob   = "event:*"
	StreamField        = "d"
	FreshnessField     = "f"
	EventIDField       = "e"
	PriorityEventsKey  = "ingest:priority:events"
)

func EventChannel(eventID string) string { return "event:" + eventID }

func BoardKey(marketKey string) string { return "board:" + marketKey }

func EventMarketsKey(eventID string) string { return "board:event:" + eventID }

func SourceHealthKey(source string) string { return "source:health:" + source }

func AlertDedupeKey(signalDedupe string, chatID int64) string {
	return fmt.Sprintf("alert:%s:%d", signalDedupe, chatID)
}

func BoardField(source, side, label string) string { return source + "|" + side + "|" + label }

func Open(ctx context.Context, url string) (*goredis.Client, error) {
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := goredis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
