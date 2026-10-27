package bayse

import (
	"strconv"
	"strings"

	"mybestbet/pkg/canon"
)

const openStatus = "open"

type pricedMarket struct {
	raw      canon.RawMarket
	outcomes []string
}

type matchCodes struct {
	home string
	away string
}

func codesOfGameSlug(slug string) (matchCodes, bool) {
	g := gameSlugCodes.FindStringSubmatch(slug)
	if g == nil {
		return matchCodes{}, false
	}
	return matchCodes{home: g[1], away: g[2]}, true
}

func (c matchCodes) sideFor(code string) (canon.Side, bool) {
	switch {
	case code == drawTitle:
		return canon.SideDraw, true
	case code != "" && strings.EqualFold(code, c.home):
		return canon.SideHome, true
	case code != "" && strings.EqualFold(code, c.away):
		return canon.SideAway, true
	}
	return "", false
}

type matchBuilder struct {
	matchID string
	codes   matchCodes
	markets []pricedMarket
	claimed map[string]bool
	drop    func(string)
}

func newMatchBuilder(matchID string, codes matchCodes, drop func(string)) *matchBuilder {
	return &matchBuilder{matchID: matchID, codes: codes, claimed: map[string]bool{}, drop: drop}
}

func (b *matchBuilder) claim(raw canon.RawMarket) bool {
	key := marketCellKey(raw)
	if b.claimed[key] {
		return false
	}
	b.claimed[key] = true
	return true
}

func marketCellKey(raw canon.RawMarket) string {
	line, player := "", ""
	if raw.Line != nil {
		line = strconv.FormatFloat(*raw.Line, 'f', -1, 64)
	}
	if raw.Player != nil {
		player = raw.Player.SourceID
	}
	return strings.Join([]string{string(raw.Code), line, player, raw.Period, raw.Team}, "|")
}

func (b *matchBuilder) addEvent(ev event) bool {
	spec, known := specFor(ev)
	if !known || ev.Status != openStatus {
		b.drop(ev.ID)
		return false
	}
	open := openMarkets(ev)
	switch spec.shape {
	case shapeOutcomeGroup:
		return b.addOutcomeGroup(ev, open, spec)
	case shapeBuckets:
		return b.addBuckets(ev, open, spec)
	case shapeOverUnder, shapeYesNo, shapeHandicap, shapePlayerOverUnder, shapePlayerAnytime, shapePlayerRating:
		added := false
		for _, m := range open {
			if b.addBinary(ev, m, spec) {
				added = true
			} else {
				b.drop(m.ID)
			}
		}
		return added
	}
	return false
}

func openMarkets(ev event) []market {
	open := make([]market, 0, len(ev.Markets))
	for _, m := range ev.Markets {
		if m.Status == openStatus {
			open = append(open, m)
		}
	}
	return open
}

func (b *matchBuilder) addOutcomeGroup(ev event, markets []market, spec marketSpec) bool {
	pm := pricedMarket{raw: b.rawMarket(spec, ev.ID, nil, nil)}
	pm.raw.Team = teamOfTeamStat(spec.code, ev.Title)
	if isMissingTeam(spec.code, pm.raw.Team) {
		b.drop(ev.ID)
		return false
	}
	for _, m := range markets {
		side, ok := b.codes.sideFor(strings.TrimSpace(m.Title))
		if !ok {
			b.drop(m.ID)
			continue
		}
		pm.raw.Selections = append(pm.raw.Selections, canon.Selection{Side: side})
		pm.outcomes = append(pm.outcomes, m.Outcome1ID)
	}
	if len(pm.outcomes) == 0 || !b.claim(pm.raw) {
		return false
	}
	b.markets = append(b.markets, pm)
	return true
}

func (b *matchBuilder) addBuckets(ev event, markets []market, spec marketSpec) bool {
	pm := pricedMarket{raw: b.rawMarket(spec, ev.ID, nil, nil)}
	pm.raw.Team = teamOfTeamStat(spec.code, ev.Title)
	if isMissingTeam(spec.code, pm.raw.Team) {
		b.drop(ev.ID)
		return false
	}
	for _, m := range markets {
		label, ok := bucketLabel(m.Title)
		if !ok {
			b.drop(ev.ID)
			return false
		}
		pm.raw.Selections = append(pm.raw.Selections, canon.Selection{Label: label})
		pm.outcomes = append(pm.outcomes, m.Outcome1ID)
	}
	if len(pm.outcomes) == 0 || !b.claim(pm.raw) {
		return false
	}
	b.markets = append(b.markets, pm)
	return true
}

func (b *matchBuilder) addBinary(ev event, m market, spec marketSpec) bool {
	switch spec.shape {
	case shapeOverUnder:
		line, ok := overLineOf(m)
		team := teamOfTeamStat(spec.code, ev.Title)
		if !ok || isMissingTeam(spec.code, team) {
			return false
		}
		raw := b.rawMarket(spec, m.ID, line, nil)
		raw.Team = team
		return b.appendSelections(raw, m, canon.SideOver, canon.SideUnder)
	case shapeYesNo:
		return b.appendTwoWay(m, spec, nil, nil, canon.SideYes, canon.SideNo)
	case shapeHandicap:
		return b.addHandicap(m, spec)
	case shapePlayerOverUnder:
		player, hasPlayer := statPlayer(ev, m)
		line, hasLine := overLineOf(m)
		return hasPlayer && hasLine && b.appendTwoWay(m, spec, line, player, canon.SideOver, canon.SideUnder)
	case shapePlayerAnytime:
		player, hasPlayer := statPlayer(ev, m)
		isAnytimeLine := m.PropLine == nil || *m.PropLine == anytimeLine
		return hasPlayer && isAnytimeLine && b.appendTwoWay(m, spec, nil, player, canon.SideYes, canon.SideNo)
	case shapePlayerRating:
		player, threshold, ok := ratingPlayer(ev)
		return ok && b.appendTwoWay(m, spec, threshold, player, canon.SideYes, canon.SideNo)
	case shapeOutcomeGroup, shapeBuckets:
		return false
	}
	return false
}

func (b *matchBuilder) addHandicap(m market, spec marketSpec) bool {
	wm, ok := winningMarginOf(m.Rules)
	if !ok || m.PropLine == nil || *m.PropLine != wm.margin {
		return false
	}
	if wm.side == canon.SideHome {
		return b.appendTwoWay(m, spec, canon.Line(-wm.margin), nil, canon.SideHome, canon.SideAway)
	}
	return b.appendTwoWay(m, spec, canon.Line(wm.margin), nil, canon.SideAway, canon.SideHome)
}

func (b *matchBuilder) appendTwoWay(m market, spec marketSpec, line *float64, player *canon.SourcePlayer, firstSide, secondSide canon.Side) bool {
	return b.appendSelections(b.rawMarket(spec, m.ID, line, player), m, firstSide, secondSide)
}

func (b *matchBuilder) appendSelections(raw canon.RawMarket, m market, firstSide, secondSide canon.Side) bool {
	if !b.claim(raw) {
		return false
	}
	raw.Selections = []canon.Selection{{Side: firstSide}, {Side: secondSide}}
	b.markets = append(b.markets, pricedMarket{raw: raw, outcomes: []string{m.Outcome1ID, m.Outcome2ID}})
	return true
}

func (b *matchBuilder) rawMarket(spec marketSpec, sourceMarketID string, line *float64, player *canon.SourcePlayer) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  b.matchID,
		Code:           spec.code,
		Line:           line,
		Player:         player,
		Period:         canon.PeriodFullTime,
		Rules:          spec.rules,
		SourceMarketID: sourceMarketID,
	}
}
