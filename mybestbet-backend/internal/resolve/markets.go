package resolve

import (
	"strings"

	"mybestbet/pkg/canon"
)

var flippedDoubleChanceLabels = map[string]string{"1X": "X2", "X2": "1X", "12": "12"}

var homeAwayDigitSwap = strings.NewReplacer("1", "2", "2", "1")

func canonicalMarket(m canon.RawMarket, link EventLink, playerID, teamID string) canon.Market {
	key := canon.MarketKey{EventID: link.EventID, Code: m.Code, Line: m.Line, PlayerID: playerID, Period: m.Period, TeamID: teamID}
	selections := m.Selections
	if link.Flipped {
		key.Line = flippedLine(m.Code, m.Line)
		selections = flippedSelections(m.Code, m.Selections)
	}
	return canon.Market{Key: key, SourceKey: m.SourceMarketID, Selections: selections, Rules: m.Rules}
}

func flippedLine(code canon.MarketCode, line *float64) *float64 {
	spec, known := canon.SpecOf(code)
	if !known || spec.Shape != canon.ShapeHandicap || line == nil || *line == 0 {
		return line
	}
	return canon.Line(-*line)
}

func flippedSelections(code canon.MarketCode, selections []canon.Selection) []canon.Selection {
	flipped := make([]canon.Selection, len(selections))
	for i, s := range selections {
		s.Side = s.Side.Flipped()
		s.Label = flippedLabel(code, s.Label)
		flipped[i] = s
	}
	return flipped
}

func flippedLabel(code canon.MarketCode, label string) string {
	switch code {
	case canon.CodeDC:
		if swapped, ok := flippedDoubleChanceLabels[label]; ok {
			return swapped
		}
	case canon.CodeCS:
		if home, away, ok := strings.Cut(label, "-"); ok {
			return away + "-" + home
		}
	case canon.CodeHTFT:
		return homeAwayDigitSwap.Replace(label)
	default:
	}
	return label
}
