package engine

import (
	"math"
	"time"

	"mybestbet/internal/pricing"
	"mybestbet/internal/signal"
	"mybestbet/pkg/canon"
)

const fairChangeTolerance = 1e-9

func isPriced(c canon.BoardCell) bool {
	return !c.Price.Suspended && c.Price.Prob > 0 && c.Price.Form != canon.FormLineOnly
}

func pricedProbs(cells []canon.BoardCell) []float64 {
	implied := make([]float64, 0, len(cells))
	for _, c := range cells {
		if isPriced(c) {
			implied = append(implied, c.Price.Prob)
		}
	}
	return implied
}

func (b *board) applySourceFair(code canon.MarketCode, cells []canon.BoardCell) {
	priced := make([]int, 0, len(cells))
	for i := range cells {
		cells[i].Fair = nil
		if isPriced(cells[i]) {
			priced = append(priced, i)
		}
	}
	fair, ok := pricing.DevigMarket(b.cfg.Pricing, code, pricedProbs(cells))
	if !ok {
		return
	}
	for j, i := range priced {
		cells[i].Fair = new(fair[j])
	}
}

func (b *board) consensusFair(ms *marketState, now time.Time) map[signal.Outcome]canon.BoardCell {
	bySelection := map[signal.Outcome][]pricing.WeightedProb{}
	newest := map[signal.Outcome]canon.BoardCell{}
	untilKickoff := b.untilKickoff(ms.key.EventID, now)
	for source, sq := range ms.sources {
		if b.isDegraded(source) || sq.hasExcessOverround {
			continue
		}
		maxAge := b.quoteMaxAge(source, untilKickoff)
		for _, c := range sq.cells {
			if c.Fair == nil || !isPriced(c) || isOlderThan(c.Price, maxAge, now) {
				continue
			}
			o := signal.Outcome{Side: c.Side, Label: c.Label}
			bySelection[o] = append(bySelection[o], pricing.WeightedProb{Feed: b.cfg.Feed(source), Prob: *c.Fair, Weight: pricing.SourceWeight(b.cfg, source, c.Price)})
			if c.Price.ObservedAt.After(newest[o].Price.ObservedAt) {
				newest[o] = c
			}
		}
	}
	consensus := make(map[signal.Outcome]canon.BoardCell, len(bySelection))
	for o, values := range bySelection {
		fair, ok := pricing.Consensus(values, b.cfg.Pricing.MinFairSources)
		if !ok {
			continue
		}
		consensus[o] = canon.BoardCell{
			MarketKey: ms.keyString,
			Source:    consensusSource,
			Side:      o.Side,
			Label:     o.Label,
			Price:     pricing.PriceFromProb(fair, canon.FormDecimal, newest[o].Price.ObservedAt),
			Fair:      new(fair),
		}
	}
	return consensus
}

func (b *board) quoteMaxAge(source string, untilKickoff time.Duration) time.Duration {
	return b.cfg.Pricing.QuoteMaxAge.For(b.cfg.Profiles[source].Type, untilKickoff)
}

func isOlderThan(p canon.Price, maxAge time.Duration, now time.Time) bool {
	return maxAge > 0 && now.Sub(p.ObservedAt) > maxAge
}

func (b *board) refreshConsensus(ms *marketState, cs *changeSet, now time.Time) {
	next := b.consensusFair(ms, now)
	for o, c := range next {
		if previous, ok := ms.consensus[o]; !ok || math.Abs(*previous.Fair-*c.Fair) > fairChangeTolerance {
			cs.cells = append(cs.cells, c)
		}
	}
	for o, previous := range ms.consensus {
		if _, ok := next[o]; !ok {
			cs.removed = append(cs.removed, tombstone(previous))
		}
	}
	ms.consensus = next
}

func fairByOutcome(consensus map[signal.Outcome]canon.BoardCell) map[signal.Outcome]float64 {
	fair := make(map[signal.Outcome]float64, len(consensus))
	for o, c := range consensus {
		fair[o] = *c.Fair
	}
	return fair
}
