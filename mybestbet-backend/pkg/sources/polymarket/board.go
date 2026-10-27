package polymarket

import (
	"bytes"
	"fmt"
	"slices"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	eventTypeBook        = "book"
	eventTypePriceChange = "price_change"
)

type selectionRef struct {
	market    int
	selection int
}

type board struct {
	fetchedAt time.Time
	markets   []pricedMarket
	books     map[string]*ladder
	refs      map[string][]selectionRef
	errs      []error
}

func newBoard(markets []pricedMarket, fetchedAt time.Time) *board {
	b := &board{fetchedAt: fetchedAt, markets: markets, books: map[string]*ladder{}, refs: map[string][]selectionRef{}}
	for mi, pm := range markets {
		for si, token := range pm.tokens {
			b.refs[token] = append(b.refs[token], selectionRef{market: mi, selection: si})
		}
	}
	return b
}

func (b *board) tokenIDs() []string {
	ids := make([]string, 0, len(b.refs))
	seen := make(map[string]bool, len(b.refs))
	for _, pm := range b.markets {
		for _, token := range pm.tokens {
			if !seen[token] {
				seen[token] = true
				ids = append(ids, token)
			}
		}
	}
	return ids
}

func (b *board) repriceAll(observedAt time.Time) {
	for i := range b.markets {
		b.markets[i].applyBooks(b.books, observedAt)
	}
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
	messages, err := decodeFrame(frame)
	if err != nil {
		return nil, err
	}
	touched := map[string]bool{}
	for _, msg := range messages {
		switch msg.EventType {
		case eventTypeBook:
			if _, tracked := b.refs[msg.AssetID]; tracked {
				b.books[msg.AssetID] = newLadder(msg.Bids, msg.Asks)
				touched[msg.AssetID] = true
			}
		case eventTypePriceChange:
			for _, change := range msg.PriceChanges {
				if b.applyPriceChange(change) {
					touched[change.AssetID] = true
				}
			}
		}
	}
	return b.repriceTokens(touched, observedAt), nil
}

func (b *board) applyPriceChange(change wsPriceChange) bool {
	if _, tracked := b.refs[change.AssetID]; !tracked {
		return false
	}
	l, ok := b.books[change.AssetID]
	if !ok {
		l = newLadder(nil, nil)
		b.books[change.AssetID] = l
	}
	l.set(change.Side, float64(change.Price), float64(change.Size))
	return true
}

func (b *board) repriceTokens(tokens map[string]bool, observedAt time.Time) []canon.RawMarket {
	var changed []int
	for token := range tokens {
		for _, ref := range b.refs[token] {
			b.markets[ref.market].raw.Selections[ref.selection].Price = priceFromLadder(b.books[token], observedAt)
			if !slices.Contains(changed, ref.market) {
				changed = append(changed, ref.market)
			}
		}
	}
	slices.Sort(changed)
	out := make([]canon.RawMarket, 0, len(changed))
	for _, idx := range changed {
		out = append(out, b.markets[idx].raw.Clone())
	}
	return out
}

func decodeFrame(frame []byte) ([]wsMessage, error) {
	frame = bytes.TrimSpace(frame)
	if len(frame) == 0 {
		return nil, nil
	}
	switch frame[0] {
	case '[':
		var messages []wsMessage
		if err := json.Unmarshal(frame, &messages); err != nil {
			return nil, fmt.Errorf("decode frame batch: %w", err)
		}
		return messages, nil
	case '{':
		var msg wsMessage
		if err := json.Unmarshal(frame, &msg); err != nil {
			return nil, fmt.Errorf("decode frame: %w", err)
		}
		return []wsMessage{msg}, nil
	}
	return nil, nil
}
