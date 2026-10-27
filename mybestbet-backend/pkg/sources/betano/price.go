package betano

import (
	"strconv"
	"time"

	"mybestbet/pkg/canon"
)

func (a *Adapter) rawMarkets(eventID string, sport sportSlug, markets []market, fetchedAt time.Time) []canon.RawMarket {
	specs := specsFor(sport)
	var out []canon.RawMarket
	for _, m := range markets {
		spec, ok := specs[m.Type]
		if !ok {
			a.deps.DropUnknownMarket(Name, string(sport)+":"+m.Type)
			continue
		}
		switch spec.selections {
		case scorerTable:
			out = append(out, scorerMarkets(eventID, m, fetchedAt)...)
		case playerYes:
			for _, s := range m.Selections {
				if pm, ok := playerMarket(eventID, m.Type, spec.code, s.Name, s, fetchedAt); ok {
					out = append(out, pm)
				}
			}
		default:
			out = append(out, lineMarkets(eventID, m, spec, fetchedAt)...)
		}
	}
	return out
}

func lineMarkets(eventID string, m market, spec marketSpec, fetchedAt time.Time) []canon.RawMarket {
	var out []canon.RawMarket
	positionByLine := map[float64]int{}
	for _, s := range m.Selections {
		sel, line, ok := matchSelection(spec, s)
		if !ok || s.Price <= 1 {
			continue
		}
		sel.Price = canon.NewPrice(s.Price, canon.FormDecimal, fetchedAt)
		key := 0.0
		if line != nil {
			key = *line
		}
		pos, seen := positionByLine[key]
		if !seen {
			pos = len(out)
			positionByLine[key] = pos
			out = append(out, newMarket(eventID, m.Type, spec.code, spec.period, line))
		}
		out[pos].Selections = append(out[pos].Selections, sel)
	}
	return out
}

func matchSelection(spec marketSpec, s selection) (canon.Selection, *float64, bool) {
	switch spec.selections {
	case fromOverUnderName:
		side, ok := overUnderSide(s.Name)
		return canon.Selection{Side: side}, canon.Line(s.Handicap), ok
	case fromHandicapColumn:
		return asianHandicapSelection(s)
	case fromScoreName:
		return canon.Selection{Label: scoreLabel(s.Name)}, nil, true
	case fromLabelName:
		return canon.Selection{Label: s.Name}, nil, true
	default:
		sel, ok := nameSelections[s.Name]
		return sel, nil, ok
	}
}

func asianHandicapSelection(s selection) (canon.Selection, *float64, bool) {
	switch s.ColumnIndex {
	case homeColumn:
		return canon.Selection{Side: canon.SideHome}, canon.Line(s.Handicap), true
	case awayColumn:
		return canon.Selection{Side: canon.SideAway}, canon.Line(-s.Handicap), true
	}
	return canon.Selection{}, nil, false
}

func newMarket(eventID, marketType string, code canon.MarketCode, period string, line *float64) canon.RawMarket {
	m := canon.RawMarket{
		EventSourceID:  eventID,
		Code:           code,
		Line:           line,
		Period:         period,
		Rules:          code.DefaultRule(),
		SourceMarketID: marketType,
	}
	if line != nil {
		m.SourceMarketID += "@" + strconv.FormatFloat(*line, 'f', -1, 64)
	}
	return m
}

func scorerMarkets(eventID string, m market, fetchedAt time.Time) []canon.RawMarket {
	if m.TableLayout == nil {
		return nil
	}
	var out []canon.RawMarket
	for _, row := range m.TableLayout.Rows {
		for i, group := range row.GroupSelections {
			if i >= len(m.TableLayout.Columns) || len(group.Selections) == 0 {
				continue
			}
			code, ok := scorerColumnCode(m.TableLayout.Columns[i].Name)
			if !ok {
				continue
			}
			if pm, ok := playerMarket(eventID, m.Type, code, row.Title, group.Selections[0], fetchedAt); ok {
				out = append(out, pm)
			}
		}
	}
	return out
}

func playerMarket(eventID, marketType string, code canon.MarketCode, player string, s selection, fetchedAt time.Time) (canon.RawMarket, bool) {
	if s.Price <= 1 || player == "" {
		return canon.RawMarket{}, false
	}
	pm := newMarket(eventID, marketType, code, canon.PeriodFullTime, nil)
	pm.SourceMarketID += ":" + string(code) + "#" + player
	pm.Player = &canon.SourcePlayer{SourceID: player, Name: player}
	pm.Selections = []canon.Selection{{Side: canon.SideYes, Price: canon.NewPrice(s.Price, canon.FormDecimal, fetchedAt)}}
	return pm, true
}
