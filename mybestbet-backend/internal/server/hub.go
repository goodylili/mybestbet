package server

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	json "github.com/goccy/go-json"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	subscriberBuffer  = 256
	pubsubChannelSize = 10_000
)

type mailbox[T any] struct {
	items      chan T
	lagging    chan struct{}
	markLagged sync.Once
}

func (m *mailbox[T]) open(capacity int) {
	m.items = make(chan T, capacity)
	m.lagging = make(chan struct{})
}

func (m *mailbox[T]) offer(item T) {
	select {
	case m.items <- item:
	default:
		m.markLagged.Do(func() { close(m.lagging) })
	}
}

type subscriber struct {
	mailbox[canon.BoardUpdate]
	eventID string
}

type streamHub struct {
	rdb      *goredis.Client
	log      *slog.Logger
	ready    chan struct{}
	stopped  chan struct{}
	changes  *boardChanges
	signaled chan struct{}
	board    *boardCache

	mu               sync.Mutex
	subscribers      map[*subscriber]struct{}
	boardSubscribers map[*boardSubscriber]struct{}
}

func newStreamHub(rdb *goredis.Client, log *slog.Logger) *streamHub {
	return &streamHub{
		rdb:              rdb,
		log:              log,
		ready:            make(chan struct{}),
		stopped:          make(chan struct{}),
		changes:          newBoardChanges(),
		signaled:         make(chan struct{}, 1),
		board:            newBoardCache(),
		subscribers:      map[*subscriber]struct{}{},
		boardSubscribers: map[*boardSubscriber]struct{}{},
	}
}

func (h *streamHub) run(ctx context.Context) error {
	defer close(h.stopped)
	pubsub := h.rdb.Subscribe(ctx, redis.ChannelSignals, redis.ChannelBoard)
	defer func() { _ = pubsub.Close() }()
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("subscribe signals: %w", err)
	}
	if err := pubsub.PSubscribe(ctx, redis.EventChannelGlob); err != nil {
		return fmt.Errorf("subscribe event channels: %w", err)
	}
	h.board.setLive(true)
	defer h.board.setLive(false)
	close(h.ready)
	messages := pubsub.Channel(goredis.WithChannelSize(pubsubChannelSize))
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			h.dispatch(msg)
		}
	}
}

func (h *streamHub) dispatch(msg *goredis.Message) {
	switch {
	case msg.Channel == redis.ChannelBoard:
		h.recordPublishedBest(msg)
	case msg.Pattern != "":
		h.dispatchEventUpdate(msg)
	default:
		h.dispatchUpdate(msg)
	}
}

func (h *streamHub) dispatchEventUpdate(msg *goredis.Message) {
	eventID := strings.TrimPrefix(msg.Channel, redis.EventChannel(""))
	var update canon.BoardUpdate
	if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
		h.log.Warn("skip undecodable event message", "channel", msg.Channel, "err", err)
		return
	}
	update.EventID = cmp.Or(update.EventID, eventID)
	marketKeys := make([]string, len(update.Cells))
	for i, cell := range update.Cells {
		marketKeys[i] = cell.MarketKey
	}
	h.board.invalidate(eventID, marketKeys)
	if touchesMainMarket(msg.Payload) {
		h.changes.markMoved(eventID)
	}
	h.offer(update, false)
}

func (h *streamHub) recordPublishedBest(msg *goredis.Message) {
	var update EventBest
	if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil || update.EventID == "" {
		h.log.Warn("skip undecodable board message", "channel", msg.Channel, "err", err)
		return
	}
	h.changes.merge(update)
}

func (h *streamHub) dispatchUpdate(msg *goredis.Message) {
	update, err := decodeUpdate(msg)
	if err != nil {
		h.log.Warn("skip undecodable stream message", "channel", msg.Channel, "err", err)
		return
	}
	isSignal := msg.Channel == redis.ChannelSignals
	if isSignal {
		wake(h.signaled)
	}
	h.offer(update, isSignal)
}

func (h *streamHub) offer(update canon.BoardUpdate, isSignal bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subscribers {
		if wantsUpdate(sub.eventID, update, isSignal) {
			sub.offer(update)
		}
	}
}

func wantsUpdate(subscribedEventID string, update canon.BoardUpdate, isSignal bool) bool {
	if isSignal {
		return subscribedEventID == "" || subscribedEventID == update.EventID
	}
	return subscribedEventID != "" && subscribedEventID == update.EventID
}

func decodeUpdate(msg *goredis.Message) (canon.BoardUpdate, error) {
	if msg.Channel != redis.ChannelSignals {
		var update canon.BoardUpdate
		err := json.Unmarshal([]byte(msg.Payload), &update)
		return update, err
	}
	var signal canon.Signal
	if err := json.Unmarshal([]byte(msg.Payload), &signal); err != nil {
		return canon.BoardUpdate{}, err
	}
	return canon.BoardUpdate{EventID: signal.EventID, Signals: []canon.Signal{signal}, SentAt: signal.DetectedAt}, nil
}

func (h *streamHub) awaitReady(ctx context.Context) error {
	select {
	case <-h.ready:
		return nil
	case <-h.stopped:
		return fmt.Errorf("stream hub stopped")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *streamHub) subscribe(ctx context.Context, eventID string) (*subscriber, error) {
	if err := h.awaitReady(ctx); err != nil {
		return nil, err
	}
	sub := &subscriber{eventID: eventID}
	sub.open(subscriberBuffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[sub] = struct{}{}
	return sub, nil
}

func (h *streamHub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subscribers, sub)
}
