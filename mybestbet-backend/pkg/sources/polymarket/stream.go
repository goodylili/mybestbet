package polymarket

import (
	"context"
	"fmt"
	"time"

	"github.com/coder/websocket"
	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	defaultWSURL  = "wss://ws-subscriptions-clob.polymarket.com/ws/market"
	marketChannel = "market"
	pingFrame     = "PING"
	pingInterval  = 10 * time.Second
)

var _ sources.Streamer = (*Adapter)(nil)

func (a *Adapter) Stream(ctx context.Context, eventIDs []string, out chan<- canon.Snapshot) error {
	return a.deps.RunStream(ctx, Name, out, func(ctx context.Context) (sources.StreamSession, error) {
		b, err := a.loadBoard(ctx, eventIDs)
		if err != nil {
			return sources.StreamSession{}, err
		}
		return sources.StreamSession{
			Resync:    b.snapshot(),
			URL:       a.deps.ExtraOr("wsUrl", defaultWSURL),
			Subscribe: func(ctx context.Context, conn *websocket.Conn) error { return subscribe(ctx, conn, b.tokenIDs()) },
			Apply: func(_ context.Context, _ *websocket.Conn, frame []byte, now time.Time) ([]canon.RawMarket, error) {
				return b.apply(frame, now)
			},
			KeepAlive: keepAlive,
		}, nil
	})
}

func subscribe(ctx context.Context, conn *websocket.Conn, tokens []string) error {
	msg, err := json.Marshal(wsSubscription{AssetIDs: tokens, Type: marketChannel})
	if err != nil {
		return fmt.Errorf("encode subscription: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
		return fmt.Errorf("subscribe market channel: %w", err)
	}
	return nil
}

func keepAlive(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.Write(ctx, websocket.MessageText, []byte(pingFrame)); err != nil {
				return
			}
		}
	}
}
