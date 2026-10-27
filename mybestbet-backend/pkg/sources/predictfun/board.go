package predictfun

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	messageTypePush     = "M"
	messageTypeResponse = "R"
	heartbeatTopic      = "heartbeat"
	heartbeatMethod     = "heartbeat"
	orderbookTopic      = "predictOrderbook/"
)

type selectionRef struct {
	market    int
	selection int
	outcome   outcomeSide
}

type board struct {
	fetchedAt time.Time
	markets   []pricedMarket
	quotes    map[int64]quote
	refs      map[int64][]selectionRef
	errs      []error
}

func newBoard(markets []pricedMarket, fetchedAt time.Time) *board {
	b := &board{fetchedAt: fetchedAt, markets: markets, quotes: map[int64]quote{}, refs: map[int64][]selectionRef{}}
	for mi, pm := range markets {
		for si, l := range pm.legs {
			b.refs[l.marketID] = append(b.refs[l.marketID], selectionRef{market: mi, selection: si, outcome: l.outcome})
		}
	}
	return b
}

func (b *board) marketIDs() []int64 {
	return slices.Sorted(maps.Keys(b.refs))
}

func (b *board) setQuote(marketID int64, q quote, observedAt time.Time) []int {
	b.quotes[marketID] = q
	var touched []int
	for _, ref := range b.refs[marketID] {
		b.markets[ref.market].raw.Selections[ref.selection].Price = q.price(ref.outcome, observedAt)
		touched = append(touched, ref.market)
	}
	return touched
}

func (b *board) snapshot() canon.Snapshot {
	snap := sources.NewSnapshot(Name, b.fetchedAt)
	snap.Errors = b.errs
	for _, pm := range b.markets {
		snap.Markets = append(snap.Markets, pm.raw.Clone())
	}
	return snap
}

func (b *board) apply(frame []byte, observedAt time.Time) ([]canon.RawMarket, []byte, error) {
	var msg wsMessage
	if err := json.Unmarshal(frame, &msg); err != nil {
		return nil, nil, fmt.Errorf("decode frame: %w", err)
	}
	switch {
	case msg.Type == messageTypeResponse && !msg.Success && msg.Error != nil:
		return nil, nil, fmt.Errorf("request %d rejected: %s %s", msg.RequestID, msg.Error.Code, msg.Error.Message)
	case msg.Type != messageTypePush:
		return nil, nil, nil
	case msg.Topic == heartbeatTopic:
		reply, err := json.Marshal(heartbeatReply{Method: heartbeatMethod, Data: msg.Data})
		if err != nil {
			return nil, nil, fmt.Errorf("encode heartbeat: %w", err)
		}
		return nil, reply, nil
	case strings.HasPrefix(msg.Topic, orderbookTopic):
		changed, err := b.applyOrderbook(msg, observedAt)
		return changed, nil, err
	}
	return nil, nil, nil
}

func (b *board) applyOrderbook(msg wsMessage, observedAt time.Time) ([]canon.RawMarket, error) {
	marketID, err := strconv.ParseInt(strings.TrimPrefix(msg.Topic, orderbookTopic), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("orderbook topic %q: %w", msg.Topic, err)
	}
	if _, tracked := b.refs[marketID]; !tracked {
		return nil, nil
	}
	var book orderbook
	if err := json.Unmarshal(msg.Data, &book); err != nil {
		return nil, fmt.Errorf("decode orderbook %d: %w", marketID, err)
	}
	touched := b.setQuote(marketID, quoteOf(book), observedAt)
	slices.Sort(touched)
	touched = slices.Compact(touched)
	out := make([]canon.RawMarket, 0, len(touched))
	for _, idx := range touched {
		out = append(out, b.markets[idx].raw.Clone())
	}
	return out, nil
}
