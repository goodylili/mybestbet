package predictfun

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	defaultWSURL    = "wss://ws.predict.fun/ws"
	subscribeMethod = "subscribe"
)

type pollOnlyError string

func (e pollOnlyError) Error() string { return string(e) }

func (pollOnlyError) PollsInstead() bool { return true }

var ErrStreamNeedsAPIKey error = pollOnlyError("predictfun: websocket needs an api key, poll rest instead")

var _ sources.Streamer = (*Adapter)(nil)

func (a *Adapter) Stream(ctx context.Context, eventIDs []string, out chan<- canon.Snapshot) error {
	if a.deps.Config.APIKey == "" {
		return ErrStreamNeedsAPIKey
	}
	headers := http.Header{}
	headers.Set(apiKeyHeader, a.deps.Config.APIKey)
	return a.deps.RunStream(ctx, Name, out, func(ctx context.Context) (sources.StreamSession, error) {
		b, err := a.loadBoard(ctx, eventIDs)
		if err != nil {
			return sources.StreamSession{}, err
		}
		return sources.StreamSession{
			Resync: b.snapshot(),
			URL:    a.deps.ExtraOr("wsUrl", defaultWSURL),
			Header: headers,
			Subscribe: func(ctx context.Context, conn *websocket.Conn) error {
				return subscribeOrderbooks(ctx, conn, b.marketIDs())
			},
			Apply: answeringHeartbeats(b),
		}, nil
	})
}

func answeringHeartbeats(b *board) sources.FrameApplier {
	return func(ctx context.Context, conn *websocket.Conn, frame []byte, now time.Time) ([]canon.RawMarket, error) {
		changed, reply, err := b.apply(frame, now)
		if err != nil {
			return nil, err
		}
		if reply != nil {
			if err := conn.Write(ctx, websocket.MessageText, reply); err != nil {
				return nil, fmt.Errorf("answer heartbeat: %w", err)
			}
		}
		return changed, nil
	}
}

func subscribeOrderbooks(ctx context.Context, conn *websocket.Conn, marketIDs []int64) error {
	for i, id := range marketIDs {
		msg, err := json.Marshal(subscribeRequest{Method: subscribeMethod, RequestID: i + 1, Params: []string{orderbookTopic + strconv.FormatInt(id, 10)}})
		if err != nil {
			return fmt.Errorf("encode subscription: %w", err)
		}
		if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
			return fmt.Errorf("subscribe orderbook %d: %w", id, err)
		}
	}
	return nil
}
