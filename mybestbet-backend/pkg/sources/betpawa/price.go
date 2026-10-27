package betpawa

import (
	"strconv"
	"time"

	"mybestbet/pkg/canon"
)

func (a *Adapter) eventMarkets(ev event, fetchedAt time.Time) []canon.RawMarket {
	var markets []canon.RawMarket
	for _, m := range ev.Markets {
		spec, ok := marketSpecs[marketKey{category: ev.Category.ID, marketTypeID: m.MarketType.ID}]
		if !ok {
			a.deps.DropUnknownMarket(Name, string(ev.Category.ID)+":"+m.MarketType.ID)
			continue
		}
		switch spec.selections {
		case playerYes, playerAtLeastNumber:
			markets = append(markets, playerMarkets(ev, m, spec, fetchedAt)...)
		default:
			markets = append(markets, lineMarkets(ev, m, spec, fetchedAt)...)
		}
	}
	return markets
}

func lineMarkets(ev event, m market, spec marketSpec, fetchedAt time.Time) []canon.RawMarket {
	var markets []canon.RawMarket
	positionByLine := map[float64]int{}
	for _, r := range m.Row {
		line, ok := rowLine(spec, r.Specifier)
		if !ok {
			continue
		}
		for _, p := range r.Prices {
			sel, ok := matchSelection(spec, p, fetchedAt)
			if !ok {
				continue
			}
			pos, seen := positionByLine[line]
			if !seen {
				pos = len(markets)
				positionByLine[line] = pos
				lm := newMarket(ev.ID, m.MarketType.ID, spec, lineIfQuoted(spec, line))
				lm.Team = participantName(ev.Participants, spec.team)
				markets = append(markets, lm)
			}
			markets[pos].Selections = append(markets[pos].Selections, sel)
		}
	}
	return markets
}

func rowLine(spec marketSpec, s specifier) (float64, bool) {
	var raw string
	switch spec.line {
	case lineFromTotal:
		raw = s.Total
	case lineFromHandicap:
		raw = s.Hcp
	default:
		return 0, true
	}
	line, err := strconv.ParseFloat(raw, 64)
	return line, err == nil
}

func matchSelection(spec marketSpec, p price, fetchedAt time.Time) (canon.Selection, bool) {
	if p.Odds <= 1 {
		return canon.Selection{}, false
	}
	var sel canon.Selection
	if spec.selections == fromPriceName {
		sel.Label = p.Name
	} else {
		known, ok := typeIDSelections[p.TypeID]
		if !ok {
			return canon.Selection{}, false
		}
		sel = known
	}
	sel.Price = decimalPrice(p, fetchedAt)
	return sel, true
}

func decimalPrice(p price, fetchedAt time.Time) canon.Price {
	pr := canon.NewPrice(p.Odds, canon.FormDecimal, fetchedAt)
	pr.Suspended = p.Suspended
	return pr
}

func lineIfQuoted(spec marketSpec, line float64) *float64 {
	if spec.line == noLine {
		return nil
	}
	return canon.Line(line)
}

func newMarket(eventID, marketTypeID string, spec marketSpec, line *float64) canon.RawMarket {
	m := canon.RawMarket{
		EventSourceID:  eventID,
		Code:           spec.code,
		Period:         spec.period,
		Rules:          spec.code.DefaultRule(),
		SourceMarketID: marketTypeID,
	}
	if line != nil {
		m.Line = line
		m.SourceMarketID += "@" + strconv.FormatFloat(*line, 'f', -1, 64)
	}
	return m
}

func playerMarkets(ev event, m market, spec marketSpec, fetchedAt time.Time) []canon.RawMarket {
	var markets []canon.RawMarket
	for _, r := range m.Row {
		if spec.goalNumber != "" && r.Specifier.Number != spec.goalNumber {
			continue
		}
		side, line, ok := playerOutcome(spec, r.Specifier)
		if !ok {
			continue
		}
		for _, p := range r.Prices {
			if p.Odds <= 1 || noScorerTypeIDs[p.TypeID] {
				continue
			}
			pm := newMarket(ev.ID, m.MarketType.ID, spec, line)
			pm.SourceMarketID += "#" + p.TypeID
			pm.Player = &canon.SourcePlayer{SourceID: p.TypeID, Name: p.Name, Team: participantName(ev.Participants, p.Team)}
			pm.Selections = []canon.Selection{{Side: side, Price: decimalPrice(p, fetchedAt)}}
			markets = append(markets, pm)
		}
	}
	return markets
}

func playerOutcome(spec marketSpec, s specifier) (canon.Side, *float64, bool) {
	if spec.selections != playerAtLeastNumber {
		return canon.SideYes, nil, true
	}
	atLeast, err := strconv.Atoi(s.Number)
	if err != nil {
		return "", nil, false
	}
	return canon.SideOver, canon.Line(float64(atLeast) - 0.5), true
}
