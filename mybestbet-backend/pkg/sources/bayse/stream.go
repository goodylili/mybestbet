package bayse

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
	defaultWSURL           = "wss://socket.bayse.markets/ws/v1/markets"
	subscribeType          = "subscribe"
	pricesChannel          = "prices"
	orderbookChannel       = "orderbook"
	marketsPerSubscription = 10
	subscribeSpacing       = 120 * time.Millisecond
)

var _ sources.Streamer = (*Adapter)(nil)

func (a *Adapter) Stream(ctx context.Context, eventIDs []string, out chan<- canon.Snapshot) error {
	return a.deps.RunStream(ctx, Name, out, func(ctx context.Context) (sources.StreamSession, error) {
		b, err := a.loadBoard(ctx, eventIDs)
		if err != nil {
			return sources.StreamSession{}, err
		}
		return sources.StreamSession{
			Resync: b.snapshot(),
			URL:    a.deps.ExtraOr("wsUrl", defaultWSURL),
			Subscribe: func(ctx context.Context, conn *websocket.Conn) error {
				return subscribeAll(ctx, conn, subscriptionsFor(b))
			},
			Apply: func(_ context.Context, _ *websocket.Conn, frame []byte, now time.Time) ([]canon.RawMarket, error) {
				return b.apply(frame, now)
			},
		}, nil
	})
}

func subscriptionsFor(b *board) []wsSubscribe {
	subs := make([]wsSubscribe, 0, len(b.eventIDs))
	for _, id := range b.eventIDs {
		subs = append(subs, wsSubscribe{Type: subscribeType, Channel: pricesChannel, EventID: id})
	}
	for start := 0; start < len(b.clobMarketIDs); start += marketsPerSubscription {
		end := min(start+marketsPerSubscription, len(b.clobMarketIDs))
		subs = append(subs, wsSubscribe{Type: subscribeType, Channel: orderbookChannel, MarketIDs: b.clobMarketIDs[start:end]})
	}
	return subs
}

func subscribeAll(ctx context.Context, conn *websocket.Conn, subs []wsSubscribe) error {
	pace := time.NewTicker(subscribeSpacing)
	defer pace.Stop()
	for i, sub := range subs {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pace.C:
			}
		}
		msg, err := json.Marshal(sub)
		if err != nil {
			return fmt.Errorf("encode subscription: %w", err)
		}
		if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
			return fmt.Errorf("subscribe %s: %w", sub.Channel, err)
		}
	}
	return nil
}
