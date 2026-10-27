package engine

import (
	"time"

	"mybestbet/internal/pricing"
	"mybestbet/pkg/canon"
)

func (b *board) propagateComputed(ms *marketState, source string, cs *changeSet) []*marketState {
	switch {
	case ms.key.Code == canon.Code1X2 && ms.key.Period == canon.PeriodFullTime:
		return b.deriveFromMatchResult(ms, source, cs)
	case ms.key.Code.IsTeamStat() && ms.key.Line == nil:
		return b.ladderFromBuckets(ms, source, cs)
	}
	return nil
}

func (b *board) deriveFromMatchResult(ms *marketState, source string, cs *changeSet) []*marketState {
	doubleChance := b.market(canon.MarketKey{EventID: ms.key.EventID, Code: canon.CodeDC, PlayerID: ms.key.PlayerID})
	drawNoBet := b.market(canon.MarketKey{EventID: ms.key.EventID, Code: canon.CodeDNB, PlayerID: ms.key.PlayerID})
	sq := ms.sources[source]
	dcCells, dnbCells := derivedCells(doubleChance.keyString, drawNoBet.keyString, source, sq)
	var touched []*marketState
	if b.replaceComputed(doubleChance, source, dcCells, sq.rules, cs) {
		touched = append(touched, doubleChance)
	}
	if b.replaceComputed(drawNoBet, source, dnbCells, sq.rules, cs) {
		touched = append(touched, drawNoBet)
	}
	return touched
}

type threeWayCells struct {
	home, draw, away canon.BoardCell
}

func matchResultCells(sq *sourceQuotes) (threeWayCells, bool) {
	var tw threeWayCells
	found := 0
	for _, c := range sq.cells {
		if c.Price.Suspended || c.Price.Prob <= 0 {
			continue
		}
		switch c.Side {
		case canon.SideHome:
			tw.home = c
			found++
		case canon.SideDraw:
			tw.draw = c
			found++
		case canon.SideAway:
			tw.away = c
			found++
		case canon.SideOver, canon.SideUnder, canon.SideYes, canon.SideNo:
		}
	}
	return tw, found == 3
}

func (tw threeWayCells) implied() pricing.ThreeWay {
	return pricing.ThreeWay{Home: tw.home.Price.Prob, Draw: tw.draw.Price.Prob, Away: tw.away.Price.Prob}
}

func (tw threeWayCells) fair() (pricing.ThreeWay, bool) {
	if tw.home.Fair == nil || tw.draw.Fair == nil || tw.away.Fair == nil {
		return pricing.ThreeWay{}, false
	}
	return pricing.ThreeWay{Home: *tw.home.Fair, Draw: *tw.draw.Fair, Away: *tw.away.Fair}, true
}

func (tw threeWayCells) newestObservation() time.Time {
	newest := tw.home.Price.ObservedAt
	for _, t := range []time.Time{tw.draw.Price.ObservedAt, tw.away.Price.ObservedAt} {
		if t.After(newest) {
			newest = t
		}
	}
	return newest
}

func derivedCells(dcKey, dnbKey, source string, sq *sourceQuotes) (doubleChance, drawNoBet []canon.BoardCell) {
	tw, ok := matchResultCells(sq)
	if !ok {
		return nil, nil
	}
	implied, ok := pricing.DeriveFromThreeWay(tw.implied())
	if !ok {
		return nil, nil
	}
	var fair pricing.DerivedThreeWay
	fairThreeWay, hasFair := tw.fair()
	if hasFair {
		fair, hasFair = pricing.DeriveFromThreeWay(fairThreeWay)
	}
	form, at := tw.home.Price.Form, tw.newestObservation()
	build := func(key string, side canon.Side, label string, impliedProb, fairProb float64) (canon.BoardCell, bool) {
		if impliedProb >= 1 {
			return canon.BoardCell{}, false
		}
		c := canon.BoardCell{MarketKey: key, Source: source, Side: side, Label: label, Price: pricing.PriceFromProb(impliedProb, form, at), Derived: true}
		if hasFair {
			c.Fair = new(fairProb)
		}
		return c, true
	}
	for _, spec := range []struct {
		label         string
		implied, fair float64
	}{
		{pricing.LabelHomeOrDraw, implied.HomeOrDraw, fair.HomeOrDraw},
		{pricing.LabelHomeOrAway, implied.HomeOrAway, fair.HomeOrAway},
		{pricing.LabelDrawOrAway, implied.DrawOrAway, fair.DrawOrAway},
	} {
		if c, ok := build(dcKey, "", spec.label, spec.implied, spec.fair); ok {
			doubleChance = append(doubleChance, c)
		}
	}
	for _, spec := range []struct {
		side          canon.Side
		implied, fair float64
	}{
		{canon.SideHome, implied.DrawNoBetHome, fair.DrawNoBetHome},
		{canon.SideAway, implied.DrawNoBetAway, fair.DrawNoBetAway},
	} {
		if c, ok := build(dnbKey, spec.side, "", spec.implied, spec.fair); ok {
			drawNoBet = append(drawNoBet, c)
		}
	}
	return doubleChance, drawNoBet
}

func (b *board) ladderFromBuckets(ms *marketState, source string, cs *changeSet) []*marketState {
	sq := ms.sources[source]
	rungs, form, at := bucketLadder(sq)
	previous := ms.ladderRungs[source]
	current := make([]canon.MarketKey, 0, len(rungs))
	var touched []*marketState
	for _, rung := range rungs {
		key := ms.key
		key.Line = canon.Line(rung.Line)
		target := b.market(key)
		over := ladderCell(target.keyString, source, canon.SideOver, rung.Over, form, at)
		under := ladderCell(target.keyString, source, canon.SideUnder, rung.Under, form, at)
		if b.replaceComputed(target, source, []canon.BoardCell{over, under}, sq.rules, cs) {
			touched = append(touched, target)
		}
		current = append(current, key)
	}
	for _, stale := range staleRungs(previous, current) {
		target := b.market(stale)
		if b.replaceComputed(target, source, nil, sq.rules, cs) {
			touched = append(touched, target)
		}
	}
	ms.ladderRungs[source] = current
	return touched
}

func bucketLadder(sq *sourceQuotes) ([]pricing.LadderRung, canon.PriceForm, time.Time) {
	quotes := make([]pricing.BucketQuote, 0, len(sq.cells))
	var newest time.Time
	for _, c := range sq.cells {
		bucket, ok := pricing.ParseBucket(c.Label)
		if !ok || c.Fair == nil || c.Price.Suspended {
			return nil, "", time.Time{}
		}
		quotes = append(quotes, pricing.BucketQuote{Bucket: bucket, Implied: c.Price.Prob, Fair: *c.Fair, Liquidity: c.Price.Liquidity})
		if c.Price.ObservedAt.After(newest) {
			newest = c.Price.ObservedAt
		}
	}
	rungs, ok := pricing.Ladder(quotes)
	if !ok {
		return nil, "", time.Time{}
	}
	return rungs, sq.cells[0].Price.Form, newest
}

func ladderCell(key, source string, side canon.Side, s pricing.LadderSide, form canon.PriceForm, at time.Time) canon.BoardCell {
	price := pricing.PriceFromProb(s.Implied, form, at)
	price.Liquidity = s.Liquidity
	return canon.BoardCell{MarketKey: key, Source: source, Side: side, Price: price, Fair: new(s.Fair)}
}

func staleRungs(previous, current []canon.MarketKey) []canon.MarketKey {
	kept := make(map[string]bool, len(current))
	for _, k := range current {
		kept[k.String()] = true
	}
	var stale []canon.MarketKey
	for _, k := range previous {
		if !kept[k.String()] {
			stale = append(stale, k)
		}
	}
	return stale
}
