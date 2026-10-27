package engine

import (
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"

	"mybestbet/internal/signal"
	"mybestbet/pkg/canon"
)

const signalChangeTolerance = 1e-6

func (b *board) rescan(ms *marketState, cs *changeSet, now time.Time) {
	b.refreshConsensus(ms, cs, now)
	b.reusableQuotes = b.appendQuotes(b.reusableQuotes[:0], ms, now)
	quotes := b.reusableQuotes
	found := signal.FindArbs(ms.key, quotes, b.cfg.Pricing, now)
	found = append(found, signal.FindValue(ms.key, quotes, fairByOutcome(ms.consensus), b.cfg.Pricing, now)...)
	found = append(found, b.moves(ms, quotes, now)...)
	b.reconcile(ms.keyString, found, cs, now)
	if hasLineFamily(ms.key) {
		family := lineFamilyKey(ms.key)
		b.reconcile(family, b.lineGaps(family, now), cs, now)
	}
}

func (b *board) recheckOpenSignals(now time.Time) changeSet {
	for scope := range b.openByScope {
		if ms, ok := b.markets[scope]; ok {
			b.markStale(ms)
			continue
		}
		b.markStale(b.lineFamily[scope]...)
	}
	var cs changeSet
	b.rescanStale(&cs, now)
	return cs
}

func (b *board) lineGaps(family string, now time.Time) []canon.Signal {
	if !b.hasLineOnlyOver(family) {
		return nil
	}
	return signal.FindLineGaps(b.lineOffers(family, now), now)
}

func (b *board) hasLineOnlyOver(family string) bool {
	for _, ms := range b.lineFamily[family] {
		for _, sq := range ms.sources {
			for _, c := range sq.cells {
				if c.Side == canon.SideOver && c.Price.Form == canon.FormLineOnly {
					return true
				}
			}
		}
	}
	return false
}

func (b *board) appendQuotes(quotes []signal.Quote, ms *marketState, now time.Time) []signal.Quote {
	untilKickoff := b.untilKickoff(ms.key.EventID, now)
	for source, sq := range ms.sources {
		isUnreliable := b.isDegraded(source) || sq.hasExcessOverround
		feed := b.cfg.Feed(source)
		fee := b.cfg.Sources[source].FeePerShare
		maxAge := b.quoteMaxAge(source, untilKickoff)
		for _, c := range sq.cells {
			isStale := isOlderThan(c.Price, maxAge, now)
			if isStale {
				staleQuotesSkipped.WithLabelValues(source).Inc()
			}
			quotes = append(quotes, signal.Quote{
				Outcome:     signal.Outcome{Side: c.Side, Label: c.Label},
				Source:      source,
				Feed:        feed,
				Price:       c.Price,
				Rules:       sq.rules,
				DeepLink:    c.DeepLink,
				Derived:     c.Derived,
				Degraded:    isUnreliable,
				Stale:       isStale,
				FeePerShare: fee,
			})
		}
	}
	return quotes
}

func (b *board) moves(ms *marketState, quotes []signal.Quote, now time.Time) []canon.Signal {
	var found []canon.Signal
	for _, q := range signal.BestPerFeed(quotes) {
		window := ms.moveHistory[cellKey{source: q.Source, side: q.Side, label: q.Label}]
		if len(window) == 0 {
			continue
		}
		window = signal.TrimMoveWindow(window, now.Add(-b.cfg.Telegram.MoveWindow))
		if s, ok := signal.DetectMove(ms.key, q, window, b.cfg.Pricing.MinMoveHold, now); ok {
			found = append(found, s)
		}
	}
	return found
}

func (b *board) lineOffers(family string, now time.Time) []signal.LineOffer {
	var offers []signal.LineOffer
	for _, ms := range b.lineFamily[family] {
		b.reusableQuotes = b.appendQuotes(b.reusableQuotes[:0], ms, now)
		for _, q := range b.reusableQuotes {
			if q.Side != canon.SideOver {
				continue
			}
			offers = append(offers, signal.LineOffer{Market: ms.key, Over: q, OverChance: b.overChance(ms, q)})
		}
	}
	return offers
}

func (b *board) overChance(ms *marketState, q signal.Quote) float64 {
	for _, c := range ms.sources[q.Source].cells {
		if c.Side == canon.SideOver && c.Fair != nil {
			return *c.Fair
		}
	}
	return q.Price.Prob
}

func (b *board) reconcile(scope string, found []canon.Signal, cs *changeSet, now time.Time) {
	open := b.openByScope[scope]
	if open == nil {
		if len(found) == 0 {
			return
		}
		open = map[string]canon.Signal{}
		b.openByScope[scope] = open
	}
	stillOpen := make(map[string]bool, len(found))
	for _, s := range found {
		stillOpen[s.DedupeKey] = true
		previous, wasOpen := open[s.DedupeKey]
		if wasOpen && !isMaterialChange(previous, s) {
			continue
		}
		if wasOpen {
			s.ID, s.DetectedAt, s.LeagueCode = previous.ID, previous.DetectedAt, previous.LeagueCode
		} else {
			s.ID = signalID(s.DedupeKey, now)
		}
		open[s.DedupeKey] = s
		cs.signals = append(cs.signals, s)
	}
	for key, previous := range open {
		if stillOpen[key] {
			continue
		}
		closed := previous
		closed.Status = canon.SignalClosed
		closed.ClosedAt = new(now)
		delete(open, key)
		cs.signals = append(cs.signals, closed)
	}
	if len(open) == 0 {
		delete(b.openByScope, scope)
	}
}

func (b *board) adoptOpenSignal(s canon.Signal) {
	scope := signalScope(s)
	if b.openByScope[scope] == nil {
		b.openByScope[scope] = map[string]canon.Signal{}
	}
	b.openByScope[scope][s.DedupeKey] = s
}

func (b *board) closeOrphanedSignals(now time.Time) changeSet {
	var cs changeSet
	for scope := range b.openByScope {
		if _, isMarket := b.markets[scope]; isMarket {
			continue
		}
		if _, isFamily := b.lineFamily[scope]; isFamily {
			continue
		}
		b.reconcile(scope, nil, &cs, now)
	}
	return cs
}

func signalScope(s canon.Signal) string {
	if s.Type == canon.SignalLine {
		return lineFamilyKey(s.Market)
	}
	return s.Market.String()
}

func signalID(dedupeKey string, openedAt time.Time) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(dedupeKey+"@"+strconv.FormatInt(openedAt.UnixNano(), 10))).String()
}

func isMaterialChange(previous, next canon.Signal) bool {
	if math.Abs(previous.Margin-next.Margin) > signalChangeTolerance || math.Abs(previous.EV-next.EV) > signalChangeTolerance || len(previous.Legs) != len(next.Legs) {
		return true
	}
	for i := range next.Legs {
		p, n := previous.Legs[i], next.Legs[i]
		if p.Source != n.Source || p.Price.Raw != n.Price.Raw || p.Price.Prob != n.Price.Prob {
			return true
		}
	}
	return false
}
