package bayse

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	clobEngine               = "CLOB"
	messageTypePriceUpdate   = "price_update"
	messageTypeOrderbookTick = "orderbook_update"
)

type selectionRef struct {
	market    int
	selection int
}

type board struct {
	fetchedAt     time.Time
	markets       []pricedMarket
	eventIDs      []string
	clobMarketIDs []string
	shares        map[string]float64
	untraded      map[string]bool
	books         map[string]quote
	isCLOB        map[string]bool
	labels        map[string]map[string]string
	refs          map[string][]selectionRef
	errs          []error
}

func newBoard(fetchedAt time.Time) *board {
	return &board{
		fetchedAt: fetchedAt,
		shares:    map[string]float64{},
		untraded:  map[string]bool{},
		books:     map[string]quote{},
		isCLOB:    map[string]bool{},
		labels:    map[string]map[string]string{},
		refs:      map[string][]selectionRef{},
	}
}

func (b *board) addEvent(ev event, markets []pricedMarket) {
	b.eventIDs = append(b.eventIDs, ev.ID)
	for _, m := range ev.Markets {
		b.shares[m.Outcome1ID] = m.Outcome1Price
		b.shares[m.Outcome2ID] = m.Outcome2Price
		if m.TotalOrders == 0 {
			b.untraded[m.Outcome1ID], b.untraded[m.Outcome2ID] = true, true
		}
		b.labels[m.ID] = map[string]string{
			strings.ToUpper(m.Outcome1Label): m.Outcome1ID,
			strings.ToUpper(m.Outcome2Label): m.Outcome2ID,
		}
		if ev.Engine == clobEngine {
			b.isCLOB[m.Outcome1ID], b.isCLOB[m.Outcome2ID] = true, true
			b.clobMarketIDs = append(b.clobMarketIDs, m.ID)
		}
	}
	link := marketURL(ev.Slug)
	for _, pm := range markets {
		for si := range pm.raw.Selections {
			pm.raw.Selections[si].DeepLink = link
		}
		mi := len(b.markets)
		b.markets = append(b.markets, pm)
		for si, outcomeID := range pm.outcomes {
			b.refs[outcomeID] = append(b.refs[outcomeID], selectionRef{market: mi, selection: si})
		}
	}
}

func (b *board) clobOutcomeIDs() []string {
	var ids []string
	for outcomeID := range b.refs {
		if b.isCLOB[outcomeID] {
			ids = append(ids, outcomeID)
		}
	}
	slices.Sort(ids)
	return ids
}

func (b *board) priceOf(outcomeID string, observedAt time.Time) canon.Price {
	if b.isCLOB[outcomeID] {
		return b.books[outcomeID].price(observedAt)
	}
	return ammPrice(b.shares[outcomeID], b.untraded[outcomeID], observedAt)
}

func (b *board) reprice(outcomeIDs []string, observedAt time.Time) []canon.RawMarket {
	var touched []int
	for _, outcomeID := range outcomeIDs {
		for _, ref := range b.refs[outcomeID] {
			b.markets[ref.market].raw.Selections[ref.selection].Price = b.priceOf(outcomeID, observedAt)
			touched = append(touched, ref.market)
		}
	}
	slices.Sort(touched)
	touched = slices.Compact(touched)
	out := make([]canon.RawMarket, 0, len(touched))
	for _, idx := range touched {
		out = append(out, b.markets[idx].raw.Clone())
	}
	return out
}

func (b *board) repriceAll(observedAt time.Time) {
	ids := make([]string, 0, len(b.refs))
	for outcomeID := range b.refs {
		ids = append(ids, outcomeID)
	}
	b.reprice(ids, observedAt)
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
	var touched []string
	for line := range bytes.SplitSeq(frame, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		ids, err := b.applyMessage(line)
		if err != nil {
			return nil, err
		}
		touched = append(touched, ids...)
	}
	return b.reprice(touched, observedAt), nil
}

func (b *board) applyMessage(line []byte) ([]string, error) {
	var msg wsMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("decode message: %w", err)
	}
	switch msg.Type {
	case messageTypePriceUpdate:
		var update priceUpdate
		if err := json.Unmarshal(msg.Data, &update); err != nil {
			return nil, fmt.Errorf("decode price update: %w", err)
		}
		return b.applyPriceUpdate(update), nil
	case messageTypeOrderbookTick:
		var update orderbookUpdate
		if err := json.Unmarshal(msg.Data, &update); err != nil {
			return nil, fmt.Errorf("decode orderbook update: %w", err)
		}
		outcomeID := update.Orderbook.OutcomeID
		if _, tracked := b.refs[outcomeID]; !tracked || !b.isCLOB[outcomeID] {
			return nil, nil
		}
		b.books[outcomeID] = quoteOf(update.Orderbook)
		return []string{outcomeID}, nil
	}
	return nil, nil
}

func (b *board) applyPriceUpdate(update priceUpdate) []string {
	var touched []string
	for _, m := range update.Markets {
		for label, share := range m.Prices {
			outcomeID := b.labels[m.ID][strings.ToUpper(label)]
			if _, tracked := b.refs[outcomeID]; !tracked || b.isCLOB[outcomeID] {
				continue
			}
			b.shares[outcomeID] = share
			delete(b.untraded, outcomeID)
			touched = append(touched, outcomeID)
		}
	}
	return touched
}
