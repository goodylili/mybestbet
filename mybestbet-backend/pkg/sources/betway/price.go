package betway

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

type pricedOutcome struct {
	outcome
	typeCode   string
	specifiers []string
	code       string
	price      canon.Price
}

func (a *Adapter) eventMarkets(eventID string, resp eventMarketsResponse, fetchedAt time.Time) []canon.RawMarket {
	decimalByOutcome := map[string]float64{}
	for _, p := range resp.Prices {
		decimalByOutcome[p.OutcomeID] = p.PriceDecimal
	}
	suspendedMarkets := map[string]bool{}
	mainLines := map[string]bool{}
	displayNames := map[string]string{}
	for _, m := range resp.MarketsInGroup {
		suspendedMarkets[m.MarketID] = m.IsSuspended
		displayNames[m.MarketID] = m.DisplayName
		if m.DefaultLine != "" {
			mainLines[m.DefaultLine] = true
		}
	}
	specs := specsForEvent(eventID, resp.Outcomes)
	var markets []canon.RawMarket
	positionByMarket := map[string]int{}
	reportedUnknown := map[string]bool{}
	for _, o := range resp.Outcomes {
		po := describeOutcome(eventID, o)
		spec, ok := specs[po.typeCode]
		if !ok {
			if !reportedUnknown[po.typeCode] {
				reportedUnknown[po.typeCode] = true
				a.deps.DropUnknownMarket(Name, po.typeCode)
			}
			continue
		}
		decimal := decimalByOutcome[o.OutcomeID]
		if decimal <= 1 || !hasRequiredSpecifier(spec, po.specifiers) {
			continue
		}
		if spec.periodFromQuarter {
			if spec.period, ok = quarterPeriod(po.specifiers); !ok {
				continue
			}
		}
		po.price = canon.NewPrice(decimal, canon.FormDecimal, fetchedAt)
		po.price.Suspended = !o.IsTradingActive || suspendedMarkets[o.MarketID]
		switch spec.selections {
		case playerYes, playerAtLeastCount:
			if m, ok := playerMarket(eventID, spec, po); ok {
				markets = append(markets, m)
			}
		default:
			sel, ok := matchSelection(spec, po)
			if !ok {
				continue
			}
			team, ok := teamOfMarket(spec, displayNames[o.OriginalMarketID])
			if !ok {
				continue
			}
			pos, seen := positionByMarket[o.OriginalMarketID]
			if !seen {
				pos = len(markets)
				positionByMarket[o.OriginalMarketID] = pos
				m := newMarket(eventID, spec, o.OriginalMarketID, lineIfQuoted(spec, o.Handicap))
				m.Team = team
				markets = append(markets, m)
			}
			markets[pos].Selections = append(markets[pos].Selections, sel)
		}
	}
	return withoutIncoherentRungs(markets, mainLines)
}

func specsForEvent(eventID string, outcomes []outcome) map[string]marketSpec {
	for _, o := range outcomes {
		if slices.Contains(headToHeadMarkerTypes, describeOutcome(eventID, o).typeCode) {
			return headToHeadSpecs
		}
	}
	return footballSpecs
}

func quarterPeriod(specifiers []string) (string, bool) {
	for _, s := range specifiers {
		if quarter, ok := strings.CutPrefix(s, quarterSpecifier); ok {
			period, known := quarterPeriods[quarter]
			return period, known
		}
	}
	return "", false
}

func describeOutcome(eventID string, o outcome) pricedOutcome {
	afterEvent := strings.TrimPrefix(o.OriginalMarketID, eventID)
	digits := len(afterEvent) - len(strings.TrimLeft(afterEvent, "0123456789"))
	return pricedOutcome{
		outcome:    o,
		typeCode:   afterEvent[:digits],
		specifiers: strings.Split(strings.TrimSuffix(afterEvent[digits:], "~"), "~"),
		code:       strings.TrimPrefix(o.OutcomeID, o.OriginalMarketID),
	}
}

func teamOfMarket(spec marketSpec, displayName string) (string, bool) {
	if !spec.teamTotal {
		return "", true
	}
	return teamOfTotal(displayName)
}

func hasRequiredSpecifier(spec marketSpec, specifiers []string) bool {
	return spec.specifier == "" || slices.Contains(specifiers, spec.specifier)
}

func lineIfQuoted(spec marketSpec, handicap float64) *float64 {
	if !spec.hasLine {
		return nil
	}
	return canon.Line(handicap)
}

func newMarket(eventID string, spec marketSpec, sourceMarketID string, line *float64) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  eventID,
		Code:           spec.code,
		Line:           line,
		Period:         spec.period,
		Rules:          spec.code.DefaultRule(),
		SourceMarketID: sourceMarketID,
	}
}

func matchSelection(spec marketSpec, po pricedOutcome) (canon.Selection, bool) {
	if spec.selections == fromScoreName {
		return canon.Selection{Label: scoreLabel(po.Name), Price: po.price}, true
	}
	sel, ok := outcomeSelections[po.code]
	sel.Price = po.price
	return sel, ok
}

func playerMarket(eventID string, spec marketSpec, po pricedOutcome) (canon.RawMarket, bool) {
	if po.code == noGoalOutcomeCode {
		return canon.RawMarket{}, false
	}
	side, line := canon.SideYes, (*float64)(nil)
	if spec.selections == playerAtLeastCount {
		atLeast, ok := countSpecifier(po.specifiers)
		if !ok {
			return canon.RawMarket{}, false
		}
		side, line = canon.SideOver, canon.Line(float64(atLeast)-0.5)
	}
	m := newMarket(eventID, spec, po.OriginalMarketID+"#"+po.code, line)
	name := strings.TrimSpace(po.Name)
	m.Player = &canon.SourcePlayer{SourceID: po.code, Name: name}
	m.Selections = []canon.Selection{{Side: side, Price: po.price}}
	return m, true
}

func countSpecifier(specifiers []string) (int, bool) {
	for _, s := range specifiers {
		if raw, ok := strings.CutPrefix(s, "count="); ok {
			n, err := strconv.Atoi(raw)
			return n, err == nil
		}
	}
	return 0, false
}
