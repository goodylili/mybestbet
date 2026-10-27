package kalshi

import (
	"fmt"
	"maps"
	"slices"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	messageTypeTicker = "ticker"
	messageTypeError  = "error"
)

type selectionRef struct {
	market    int
	selection int
	outcome   outcome
}

type board struct {
	fetchedAt time.Time
	markets   []pricedMarket
	quotes    map[string]quote
	refs      map[string][]selectionRef
	errs      []error
}

func newBoard(markets []pricedMarket, quotes map[string]quote, fetchedAt time.Time) *board {
	b := &board{fetchedAt: fetchedAt, markets: markets, quotes: quotes, refs: map[string][]selectionRef{}}
	for mi, pm := range markets {
		for si, l := range pm.legs {
			b.refs[l.ticker] = append(b.refs[l.ticker], selectionRef{market: mi, selection: si, outcome: l.outcome})
		}
	}
	for ticker := range b.refs {
		b.reprice(ticker, fetchedAt)
	}
	return b
}

func (b *board) tickers() []string {
	return slices.Sorted(maps.Keys(b.refs))
}

func (b *board) reprice(ticker string, observedAt time.Time) []int {
	q := b.quotes[ticker]
	var touched []int
	for _, ref := range b.refs[ticker] {
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

func (b *board) apply(frame []byte, observedAt time.Time) ([]canon.RawMarket, error) {
	var env wsEnvelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return nil, fmt.Errorf("decode frame: %w", err)
	}
	switch env.Type {
	case messageTypeTicker:
		var msg tickerMsg
		if err := json.Unmarshal(env.Msg, &msg); err != nil {
			return nil, fmt.Errorf("decode ticker: %w", err)
		}
		return b.applyTicker(msg, observedAt), nil
	case messageTypeError:
		var e wsError
		if err := json.Unmarshal(env.Msg, &e); err != nil {
			return nil, fmt.Errorf("decode stream error: %w", err)
		}
		return nil, fmt.Errorf("stream error %d: %s", e.Code, e.Message)
	}
	return nil, nil
}

func (b *board) applyTicker(msg tickerMsg, observedAt time.Time) []canon.RawMarket {
	if _, tracked := b.refs[msg.MarketTicker]; !tracked {
		return nil
	}
	b.quotes[msg.MarketTicker] = b.quotes[msg.MarketTicker].withUpdate(msg)
	touched := b.reprice(msg.MarketTicker, observedAt)
	slices.Sort(touched)
	touched = slices.Compact(touched)
	out := make([]canon.RawMarket, 0, len(touched))
	for _, idx := range touched {
		out = append(out, b.markets[idx].raw.Clone())
	}
	return out
}
