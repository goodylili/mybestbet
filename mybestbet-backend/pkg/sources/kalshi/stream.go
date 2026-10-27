package kalshi

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/coder/websocket"
	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	defaultWSURL     = "wss://api.elections.kalshi.com/trade-api/ws/v2"
	tickerChannel    = "ticker"
	subscribeCommand = "subscribe"
)

type pollOnlyError string

func (e pollOnlyError) Error() string { return string(e) }

func (pollOnlyError) PollsInstead() bool { return true }

var ErrStreamNeedsAPIKey error = pollOnlyError("kalshi: websocket needs an api key, poll rest instead")

var _ sources.Streamer = (*Adapter)(nil)

func (a *Adapter) Stream(ctx context.Context, eventIDs []string, out chan<- canon.Snapshot) error {
	if a.deps.Config.APIKey == "" {
		return ErrStreamNeedsAPIKey
	}
	wsURL := a.deps.ExtraOr("wsUrl", defaultWSURL)
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("parse websocket url: %w", err)
	}
	return a.deps.RunStream(ctx, Name, out, func(ctx context.Context) (sources.StreamSession, error) {
		b, err := a.loadBoard(ctx, eventIDs)
		if err != nil {
			return sources.StreamSession{}, err
		}
		headers, err := signedHandshakeHeaders(a.deps.Config.APIKey, a.deps.Config.APISecret, parsed.Path, a.deps.Clock())
		if err != nil {
			return sources.StreamSession{}, err
		}
		return sources.StreamSession{
			Resync:    b.snapshot(),
			URL:       wsURL,
			Header:    headers,
			Subscribe: func(ctx context.Context, conn *websocket.Conn) error { return subscribe(ctx, conn, b.tickers()) },
			Apply: func(_ context.Context, _ *websocket.Conn, frame []byte, now time.Time) ([]canon.RawMarket, error) {
				return b.apply(frame, now)
			},
		}, nil
	})
}

func subscribe(ctx context.Context, conn *websocket.Conn, tickers []string) error {
	msg, err := json.Marshal(wsCommand{ID: 1, Cmd: subscribeCommand, Params: wsParams{Channels: []string{tickerChannel}, MarketTickers: tickers}})
	if err != nil {
		return fmt.Errorf("encode subscription: %w", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, msg); err != nil {
		return fmt.Errorf("subscribe ticker channel: %w", err)
	}
	return nil
}
