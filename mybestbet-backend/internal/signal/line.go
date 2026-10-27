package signal

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"time"

	"mybestbet/pkg/canon"
)

type LineOffer struct {
	Market     canon.MarketKey
	Over       Quote
	OverChance float64
}

const (
	evenMoneyChance  = 0.5
	lineGapTolerance = 1e-9
)

func FindLineGaps(offers []LineOffer, now time.Time) []canon.Signal {
	mainLines := bookMainLines(offers)
	if len(mainLines) == 0 {
		return nil
	}
	consensusLine := lowerMedianLine(mainLines)
	var found []canon.Signal
	for _, pick := range offers {
		if pick.Over.Price.Form != canon.FormLineOnly || pick.Market.Line == nil {
			continue
		}
		gap := *pick.Market.Line - consensusLine
		if math.Abs(gap) <= lineGapTolerance {
			continue
		}
		legs := []canon.Leg{pick.Over.leg(0)}
		for _, book := range mainLines {
			leg := book.Over.leg(0)
			leg.Label = formatLine(*book.Market.Line)
			leg.MarketKey, leg.Line = book.Market.String(), book.Market.Line
			legs = append(legs, leg)
		}
		s := newSignal(canon.SignalLine, pick.Market, legs, now, pick.Over.Source, formatLine(consensusLine))
		s.Margin = math.Abs(gap)
		found = append(found, s)
	}
	return found
}

func bookMainLines(offers []LineOffer) []LineOffer {
	closest := map[string]LineOffer{}
	for _, o := range offers {
		if o.Market.Line == nil || o.OverChance <= 0 || !o.Over.isBettable(0) {
			continue
		}
		current, seen := closest[o.Over.Source]
		if !seen || math.Abs(o.OverChance-evenMoneyChance) < math.Abs(current.OverChance-evenMoneyChance) {
			closest[o.Over.Source] = o
		}
	}
	lines := make([]LineOffer, 0, len(closest))
	for _, o := range closest {
		lines = append(lines, o)
	}
	slices.SortFunc(lines, func(a, b LineOffer) int {
		if c := cmp.Compare(*a.Market.Line, *b.Market.Line); c != 0 {
			return c
		}
		return cmp.Compare(a.Over.Source, b.Over.Source)
	})
	return lines
}

func lowerMedianLine(sortedByLine []LineOffer) float64 {
	return *sortedByLine[(len(sortedByLine)-1)/2].Market.Line
}

func formatLine(line float64) string { return strconv.FormatFloat(line, 'f', -1, 64) }
