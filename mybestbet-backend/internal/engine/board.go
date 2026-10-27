package engine

import (
	"slices"
	"time"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/pricing"
	"mybestbet/internal/signal"
	"mybestbet/pkg/canon"
)

const (
	consensusSource   = "consensus"
	maxMovePoints     = 512
	boardRefreshAfter = 10 * time.Minute
)

type sourceQuotes struct {
	cells              []canon.BoardCell
	sourceMarketKey    string
	rules              canon.SettleRule
	isComputed         bool
	hasExcessOverround bool
	boardWrittenAt     time.Time
}

type marketState struct {
	key         canon.MarketKey
	keyString   string
	sources     map[string]*sourceQuotes
	consensus   map[signal.Outcome]canon.BoardCell
	moveHistory map[cellKey][]signal.MovePoint
	ladderRungs map[string][]canon.MarketKey
	isStale     bool
}

type board struct {
	cfg          config.Config
	isDegraded   func(source string) bool
	untilKickoff func(eventID string, now time.Time) time.Duration
	markets      map[string]*marketState
	byEvent      map[string][]*marketState
	lineFamily   map[string][]*marketState
	openByScope  map[string]map[string]canon.Signal

	reusableQuotes []signal.Quote
	stale          []*marketState
}

type changeSet struct {
	eventID string
	cells   []canon.BoardCell
	removed []canon.BoardCell
	rows    []priceRow
	signals []canon.Signal
}

func newBoard(cfg config.Config, isDegraded func(string) bool) *board {
	return &board{
		cfg:          cfg,
		isDegraded:   isDegraded,
		untilKickoff: unknownKickoffDistance,
		markets:      map[string]*marketState{},
		byEvent:      map[string][]*marketState{},
		lineFamily:   map[string][]*marketState{},
		openByScope:  map[string]map[string]canon.Signal{},
	}
}

type cellKey struct {
	source string
	side   canon.Side
	label  string
}

func cellKeyOf(c canon.BoardCell) cellKey {
	return cellKey{source: c.Source, side: c.Side, label: c.Label}
}

func (k cellKey) boardField() string { return redis.BoardField(k.source, string(k.side), k.label) }

func lineFamilyKey(k canon.MarketKey) string {
	k.Line = nil
	return k.String()
}

func hasLineFamily(k canon.MarketKey) bool {
	return k.Line != nil && (k.Code.IsPlayerProp() || k.Code.IsTeamStat())
}

func (b *board) market(key canon.MarketKey) *marketState {
	keyString := key.String()
	if ms, ok := b.markets[keyString]; ok {
		return ms
	}
	ms := &marketState{
		key:         key,
		keyString:   keyString,
		sources:     map[string]*sourceQuotes{},
		consensus:   map[signal.Outcome]canon.BoardCell{},
		moveHistory: map[cellKey][]signal.MovePoint{},
		ladderRungs: map[string][]canon.MarketKey{},
	}
	b.markets[keyString] = ms
	b.byEvent[key.EventID] = append(b.byEvent[key.EventID], ms)
	if hasLineFamily(key) {
		family := lineFamilyKey(key)
		b.lineFamily[family] = append(b.lineFamily[family], ms)
	}
	return ms
}

func (b *board) apply(d canon.Delta, now time.Time) changeSet {
	cs := b.stage(d, now)
	b.rescanStale(&cs, now)
	return cs
}

func (b *board) stage(d canon.Delta, now time.Time) changeSet {
	cs := changeSet{eventID: d.Market.Key.EventID}
	b.markStale(b.ingest(d, &cs, now)...)
	return cs
}

func (b *board) markStale(markets ...*marketState) {
	for _, ms := range markets {
		if !ms.isStale {
			ms.isStale = true
			b.stale = append(b.stale, ms)
		}
	}
}

func (b *board) rescanStale(cs *changeSet, now time.Time) {
	for _, ms := range b.stale {
		ms.isStale = false
		b.rescan(ms, cs, now)
	}
	clear(b.stale)
	b.stale = b.stale[:0]
}

func (b *board) confirmUnchanged(f canon.Freshness, now time.Time) changeSet {
	cs := b.stageFreshness(f)
	b.rescanStale(&cs, now)
	return cs
}

func (b *board) stageFreshness(f canon.Freshness) changeSet {
	cs := changeSet{eventID: f.EventID}
	confirmed := make(map[string]bool, len(f.SourceKeys))
	for _, key := range f.SourceKeys {
		confirmed[key] = true
	}
	for _, ms := range b.byEvent[f.EventID] {
		sq := ms.sources[f.Source]
		if sq == nil || sq.isComputed || sq.sourceMarketKey == "" {
			continue
		}
		switch {
		case confirmed[sq.sourceMarketKey]:
			b.confirmQuotes(ms, sq, f.ObservedAt)
		case f.Complete:
			b.markStale(b.suspendWithdrawn(ms, f.Source, &cs)...)
		}
	}
	return cs
}

func (b *board) confirmQuotes(ms *marketState, sq *sourceQuotes, observedAt time.Time) {
	for i := range sq.cells {
		if observedAt.After(sq.cells[i].Price.ObservedAt) {
			sq.cells[i].Price.ObservedAt = observedAt
		}
	}
	b.markStale(ms)
}

func (b *board) suspendWithdrawn(ms *marketState, source string, cs *changeSet) []*marketState {
	sq := ms.sources[source]
	if !slices.ContainsFunc(sq.cells, func(c canon.BoardCell) bool { return !c.Price.Suspended }) {
		return nil
	}
	suspended := make([]canon.BoardCell, len(sq.cells))
	for i, c := range sq.cells {
		c.Price.Suspended = true
		suspended[i] = c
		row := newPriceRow(ms, c, sq)
		row.isRawChanged = true
		cs.rows = append(cs.rows, row)
	}
	b.forgetMoves(ms, sq)
	ms.sources[source] = &sourceQuotes{cells: suspended, sourceMarketKey: sq.sourceMarketKey, rules: sq.rules}
	cs.cells = append(cs.cells, suspended...)
	withdrawnCells.WithLabelValues(source).Add(float64(len(suspended)))
	return append([]*marketState{ms}, b.propagateComputed(ms, source, cs)...)
}

func (b *board) restore(d canon.Delta, now time.Time) {
	var discarded changeSet
	for _, ms := range b.ingest(d, &discarded, now) {
		if sq := ms.sources[d.Source]; sq != nil {
			b.forgetMoves(ms, sq)
		}
	}
}

func (b *board) rescanEvent(eventID string, now time.Time) changeSet {
	cs := changeSet{eventID: eventID}
	for _, ms := range b.byEvent[eventID] {
		for _, sq := range ms.sources {
			cs.cells = append(cs.cells, sq.cells...)
		}
		b.rescan(ms, &cs, now)
	}
	return cs
}

func (b *board) ingest(d canon.Delta, cs *changeSet, now time.Time) []*marketState {
	ms := b.market(d.Market.Key)
	previous := ms.sources[d.Source]
	next := b.newSourceQuotes(ms.key.Code, b.cellsFromDelta(ms, d), d.Market.Rules, false)
	next.sourceMarketKey = d.Market.SourceKey
	priceBaseline := previous
	if isDifferentSourceMarket(previous, next) {
		b.forgetMoves(ms, previous)
		priceBaseline = nil
	}
	for _, c := range next.cells {
		row := newPriceRow(ms, c, next)
		row.isRawChanged = isRawChanged(priceBaseline, c)
		if row.isRawChanged {
			b.recordMove(ms, c, now)
		}
		cs.rows = append(cs.rows, row)
	}
	gone := missingCells(previous, next.cells)
	for _, c := range gone {
		delete(ms.moveHistory, cellKeyOf(c))
	}
	cs.removed = append(cs.removed, gone...)
	ms.sources[d.Source] = next
	publishChangedCells(previous, next, cs)
	return append([]*marketState{ms}, b.propagateComputed(ms, d.Source, cs)...)
}

func publishChangedCells(previous, next *sourceQuotes, cs *changeSet) {
	observedAt := newestObservedAt(next.cells)
	if previous != nil && !hasBoardChange(previous.cells, next.cells) && observedAt.Sub(previous.boardWrittenAt) < boardRefreshAfter {
		next.boardWrittenAt = previous.boardWrittenAt
		return
	}
	next.boardWrittenAt = observedAt
	cs.cells = append(cs.cells, next.cells...)
}

func newestObservedAt(cells []canon.BoardCell) time.Time {
	var newest time.Time
	for _, c := range cells {
		if c.Price.ObservedAt.After(newest) {
			newest = c.Price.ObservedAt
		}
	}
	return newest
}

func hasBoardChange(previous, next []canon.BoardCell) bool {
	if len(previous) != len(next) {
		return true
	}
	for i, c := range next {
		j := i
		if cellKeyOf(previous[j]) != cellKeyOf(c) {
			j = slices.IndexFunc(previous, func(p canon.BoardCell) bool { return cellKeyOf(p) == cellKeyOf(c) })
		}
		if j < 0 || !isSameOnBoard(previous[j], c) {
			return true
		}
	}
	return false
}

func isSameOnBoard(a, b canon.BoardCell) bool {
	pa, pb := a.Price, b.Price
	return a.DeepLink == b.DeepLink && a.Derived == b.Derived && isSameOptional(a.Fair, b.Fair) &&
		pa.Raw == pb.Raw && pa.Form == pb.Form && pa.Prob == pb.Prob && pa.Suspended == pb.Suspended &&
		isSameOptional(pa.Bid, pb.Bid) && isSameOptional(pa.Ask, pb.Ask) && isSameOptional(pa.Liquidity, pb.Liquidity)
}

func isSameOptional(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (b *board) newSourceQuotes(code canon.MarketCode, cells []canon.BoardCell, rules canon.SettleRule, isComputed bool) *sourceQuotes {
	if !isComputed {
		b.applySourceFair(code, cells)
	}
	return &sourceQuotes{
		cells:              cells,
		rules:              rules,
		isComputed:         isComputed,
		hasExcessOverround: pricing.ExceedsMaxOverround(code, pricedProbs(cells), b.cfg.Pricing.MaxOverround),
	}
}

func isDifferentSourceMarket(previous, next *sourceQuotes) bool {
	return previous != nil && previous.sourceMarketKey != "" && previous.sourceMarketKey != next.sourceMarketKey
}

func (b *board) forgetMoves(ms *marketState, sq *sourceQuotes) {
	for _, c := range sq.cells {
		delete(ms.moveHistory, cellKeyOf(c))
	}
}

func (b *board) cellsFromDelta(ms *marketState, d canon.Delta) []canon.BoardCell {
	cells := make([]canon.BoardCell, 0, len(d.Market.Selections))
	seen := make(map[cellKey]bool, len(d.Market.Selections))
	for _, sel := range d.Market.Selections {
		if !pricing.FitsShape(ms.key.Code, sel.Side) {
			misshapenCellsDropped.WithLabelValues(d.Source, string(ms.key.Code)).Inc()
			continue
		}
		c := canon.BoardCell{MarketKey: ms.keyString, Source: d.Source, Side: sel.Side, Label: sel.Label, Price: detachedFromDecodeBuffer(sel.Price), DeepLink: sel.DeepLink}
		key := cellKeyOf(c)
		if seen[key] {
			duplicateCellsDropped.WithLabelValues(d.Source).Inc()
			continue
		}
		seen[key] = true
		cells = append(cells, c)
	}
	return cells
}

func detachedFromDecodeBuffer(p canon.Price) canon.Price {
	p.Bid, p.Ask, p.Liquidity = cloneFloat(p.Bid), cloneFloat(p.Ask), cloneFloat(p.Liquidity)
	return p
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	return new(*v)
}

func isRawChanged(previous *sourceQuotes, c canon.BoardCell) bool {
	if previous == nil || previous.isComputed {
		return true
	}
	for _, p := range previous.cells {
		if p.Side == c.Side && p.Label == c.Label {
			return p.Price.Raw != c.Price.Raw || p.Price.Suspended != c.Price.Suspended || p.Price.Form != c.Price.Form
		}
	}
	return true
}

func missingCells(previous *sourceQuotes, next []canon.BoardCell) []canon.BoardCell {
	if previous == nil {
		return nil
	}
	kept := make(map[cellKey]bool, len(next))
	for _, c := range next {
		kept[cellKeyOf(c)] = true
	}
	var gone []canon.BoardCell
	for _, p := range previous.cells {
		if !kept[cellKeyOf(p)] {
			gone = append(gone, tombstone(p))
		}
	}
	return gone
}

func tombstone(c canon.BoardCell) canon.BoardCell {
	return canon.BoardCell{MarketKey: c.MarketKey, Source: c.Source, Side: c.Side, Label: c.Label, Price: canon.Price{Form: c.Price.Form, Suspended: true, ObservedAt: c.Price.ObservedAt}}
}

func (b *board) recordMove(ms *marketState, c canon.BoardCell, now time.Time) {
	window := b.cfg.Telegram.MoveWindow
	if window <= 0 || c.Price.Prob <= 0 || c.Price.Suspended {
		return
	}
	key := cellKeyOf(c)
	history := signal.TrimMoveWindow(ms.moveHistory[key], now.Add(-window))
	if len(history) >= maxMovePoints {
		history = history[len(history)-maxMovePoints+1:]
	}
	ms.moveHistory[key] = append(history, signal.NewMovePoint(c.Price, now))
}

func (b *board) replaceComputed(target *marketState, source string, cells []canon.BoardCell, rules canon.SettleRule, cs *changeSet) bool {
	previous := target.sources[source]
	if previous != nil && !previous.isComputed {
		return false
	}
	cs.removed = append(cs.removed, missingCells(previous, cells)...)
	if len(cells) == 0 {
		delete(target.sources, source)
		return previous != nil
	}
	next := b.newSourceQuotes(target.key.Code, cells, rules, true)
	publishChangedCells(previous, next, cs)
	target.sources[source] = next
	return true
}
