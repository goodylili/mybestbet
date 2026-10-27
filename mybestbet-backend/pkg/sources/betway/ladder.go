package betway

import (
	"math"
	"slices"

	"mybestbet/pkg/canon"
)

type ladderKey struct {
	code   canon.MarketCode
	period string
	team   string
}

type rung struct {
	index           int
	line            float64
	rising, falling float64
	main            bool
}

func (r rung) isBelow(outer rung) bool {
	return outer.rising > r.rising && outer.falling < r.falling
}

var ladderSides = map[canon.MarketShape]struct{ rising, falling canon.Side }{
	canon.ShapeOverUnder: {rising: canon.SideOver, falling: canon.SideUnder},
	canon.ShapeHandicap:  {rising: canon.SideAway, falling: canon.SideHome},
}

func withoutIncoherentRungs(markets []canon.RawMarket, mainLines map[string]bool) []canon.RawMarket {
	ladders := map[ladderKey][]rung{}
	for i, m := range markets {
		if r, ok := ladderRung(i, m); ok {
			r.main = mainLines[m.SourceMarketID]
			key := ladderKey{code: m.Code, period: m.Period, team: m.Team}
			ladders[key] = append(ladders[key], r)
		}
	}
	dropped := map[int]bool{}
	for _, rungs := range ladders {
		for _, r := range incoherentRungs(rungs) {
			dropped[r.index] = true
		}
	}
	if len(dropped) == 0 {
		return markets
	}
	kept := markets[:0]
	for i, m := range markets {
		if !dropped[i] {
			kept = append(kept, m)
		}
	}
	return kept
}

func ladderRung(index int, m canon.RawMarket) (rung, bool) {
	spec, ok := canon.SpecOf(m.Code)
	if !ok || m.Player != nil || m.Line == nil {
		return rung{}, false
	}
	sides, ok := ladderSides[spec.Shape]
	if !ok {
		return rung{}, false
	}
	r := rung{index: index, line: *m.Line}
	for _, s := range m.Selections {
		switch s.Side {
		case sides.rising:
			r.rising = s.Price.Raw
		case sides.falling:
			r.falling = s.Price.Raw
		default:
		}
	}
	return r, r.rising > 1 && r.falling > 1
}

func incoherentRungs(rungs []rung) []rung {
	slices.SortFunc(rungs, func(a, b rung) int {
		switch {
		case a.line < b.line:
			return -1
		case a.line > b.line:
			return 1
		}
		return 0
	})
	low, high := clampedEnds(rungs)
	if low > high {
		return rungs
	}
	core := rungs[low : high+1]
	anchor := anchorOf(core)
	dropped := append(slices.Clone(rungs[:low]), rungs[high+1:]...)
	kept := core[anchor]
	for i := anchor - 1; i >= 0; i-- {
		if core[i].isBelow(kept) {
			kept = core[i]
			continue
		}
		dropped = append(dropped, core[i])
	}
	kept = core[anchor]
	for i := anchor + 1; i < len(core); i++ {
		if kept.isBelow(core[i]) {
			kept = core[i]
			continue
		}
		dropped = append(dropped, core[i])
	}
	return dropped
}

func anchorOf(core []rung) int {
	if i := slices.IndexFunc(core, func(r rung) bool { return r.main }); i >= 0 {
		return i
	}
	anchor := 0
	for i, r := range core {
		if math.Abs(r.rising-r.falling) < math.Abs(core[anchor].rising-core[anchor].falling) {
			anchor = i
		}
	}
	return anchor
}

func clampedEnds(rungs []rung) (low, high int) {
	low, high = 0, len(rungs)-1
	for i := high; i > 0; i-- {
		if rungs[i].falling == rungs[0].falling {
			low = i + 1
			break
		}
	}
	for i := 0; i < len(rungs)-1; i++ {
		if rungs[i].rising == rungs[len(rungs)-1].rising {
			high = i - 1
			break
		}
	}
	return low, high
}
