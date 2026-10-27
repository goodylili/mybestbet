package predictfun

import (
	"strconv"
	"strings"

	"mybestbet/pkg/canon"
)

type leg struct {
	marketID int64
	outcome  outcomeSide
}

type pricedMarket struct {
	raw  canon.RawMarket
	legs []leg
}

type matchTeams struct {
	home         team
	away         team
	homeSlugCode string
	awaySlugCode string
}

func teamsOf(c category, sport canon.Sport) (matchTeams, bool) {
	listedInTitleOrder := c.VariantDetails.Sports.Teams
	listed := listedInTitleOrder
	if len(listed) == 0 {
		listed = c.Teams
	}
	if teams, flagged := teamsByHomeFlag(listed); flagged {
		return teams.withSlugCodes(c.Slug, listedInTitleOrder), true
	}
	if listsFirstNamedAsHome(sport) && len(listed) == binaryOutcomes {
		return matchTeams{home: listed[0], away: listed[1]}.withSlugCodes(c.Slug, listedInTitleOrder), true
	}
	return matchTeams{}, false
}

func (t matchTeams) withSlugCodes(slug string, listedInTitleOrder []team) matchTeams {
	parts := strings.Split(slug, "-")
	if len(parts) < minMatchSlugParts || len(listedInTitleOrder) != binaryOutcomes {
		return t
	}
	for i, tm := range listedInTitleOrder {
		switch tm {
		case t.home:
			t.homeSlugCode = parts[firstSlugTeamPart+i]
		case t.away:
			t.awaySlugCode = parts[firstSlugTeamPart+i]
		}
	}
	return t
}

func teamsByHomeFlag(listed []team) (matchTeams, bool) {
	var t matchTeams
	for _, tm := range listed {
		if tm.IsHome {
			t.home = tm
		} else {
			t.away = tm
		}
	}
	return t, t.home.Name != "" && t.away.Name != ""
}

func listsFirstNamedAsHome(sport canon.Sport) bool {
	return sport == canon.SportTennis || sport == canon.SportEsports
}

func (t matchTeams) sideOf(name string) (canon.Side, bool) {
	switch {
	case t.home.isNamed(name):
		return canon.SideHome, true
	case t.away.isNamed(name):
		return canon.SideAway, true
	case name != "" && strings.EqualFold(name, t.homeSlugCode):
		return canon.SideHome, true
	case name != "" && strings.EqualFold(name, t.awaySlugCode):
		return canon.SideAway, true
	}
	return "", false
}

func (t matchTeams) sideOfOutcome(o outcome) (canon.Side, bool) {
	if side, ok := t.sideOf(o.Name); ok {
		return side, true
	}
	if o.Team != nil {
		return t.sideOf(o.Team.Abbreviation)
	}
	return "", false
}

func (tm team) isNamed(name string) bool {
	return name != "" && (strings.EqualFold(name, tm.Abbreviation) || strings.EqualFold(name, tm.Name))
}

type matchBuilder struct {
	matchID string
	specs   map[string]marketSpec
	teams   matchTeams
	groups  map[string]int
	markets []pricedMarket
	quotes  map[int64]quote
	drop    func(string)
}

func newMatchBuilder(matchID string, sport canon.Sport, teams matchTeams, drop func(string)) *matchBuilder {
	return &matchBuilder{matchID: matchID, specs: marketSpecsBySport[sport], teams: teams, groups: map[string]int{}, quotes: map[int64]quote{}, drop: drop}
}

func (b *matchBuilder) addCategory(c category) {
	for _, m := range c.Markets {
		if m.TradingStatus != openTradingStatus {
			continue
		}
		spec, known := b.specs[m.MarketType]
		if !known || !b.addMarket(c, m, spec) {
			b.drop(strconv.FormatInt(m.ID, 10))
			continue
		}
		if q, ok := inlineQuoteOf(m); ok {
			b.quotes[m.ID] = q
		}
	}
}

func (b *matchBuilder) addMarket(c category, m market, spec marketSpec) bool {
	line, hasLine := lineOf(m, spec.shape)
	if lineRequiredByShapes[spec.shape] && !hasLine {
		return false
	}
	switch spec.shape {
	case shapeOutcomeGroup:
		return b.addGroupSelection(c, m, spec)
	case shapeOverUnder:
		return b.addOverUnder(m, spec, line, nil)
	case shapeYesNo:
		return b.addBinary(m, spec, nil, nil, canon.SideYes, canon.SideNo)
	case shapeHandicap:
		return b.addTeamBinary(m, spec, &line)
	case shapeTeamBinary:
		return b.addTeamBinary(m, spec, nil)
	case shapeFirstMapWinner:
		return gameNumberOf(m) == firstMap && b.addTeamBinary(m, spec, nil)
	case shapePlayerOverUnder:
		player, ok := playerOf(m)
		return ok && b.addOverUnder(m, spec, line, player)
	case shapePlayerYesNo:
		player, ok := playerOf(m)
		isYesNoLine := !hasLine || line == yesNoPlayerLine
		return ok && isYesNoLine && b.addBinary(m, spec, nil, player, canon.SideYes, canon.SideNo)
	case shapeTeamOverUnder:
		return b.addTeamOverUnder(m, spec, line)
	}
	return false
}

func (b *matchBuilder) addTeamOverUnder(m market, spec marketSpec, line float64) bool {
	name, ok := teamTotalName(m.Title)
	if !ok {
		return false
	}
	side, ok := b.teams.sideOf(name)
	if !ok || !b.addOverUnder(m, spec, line, nil) {
		return false
	}
	b.markets[len(b.markets)-1].raw.Team = b.teams.nameOf(side)
	return true
}

func (t matchTeams) nameOf(side canon.Side) string {
	if side == canon.SideAway {
		return t.away.Name
	}
	return t.home.Name
}

func (b *matchBuilder) addOverUnder(m market, spec marketSpec, line float64, player *canon.SourcePlayer) bool {
	if listsUnderFirst(m) {
		return b.addBinary(m, spec, canon.Line(line), player, canon.SideUnder, canon.SideOver)
	}
	return b.addBinary(m, spec, canon.Line(line), player, canon.SideOver, canon.SideUnder)
}

func (b *matchBuilder) addGroupSelection(c category, m market, spec marketSpec) bool {
	sel, ok := b.groupSelection(m, spec)
	if !ok {
		return false
	}
	groupKey := c.Slug + "|" + string(spec.code)
	idx, exists := b.groups[groupKey]
	if !exists {
		idx = len(b.markets)
		b.groups[groupKey] = idx
		b.markets = append(b.markets, pricedMarket{raw: b.rawMarket(spec, c.Slug, nil, nil)})
	}
	pm := &b.markets[idx]
	pm.raw.Selections = append(pm.raw.Selections, sel)
	pm.legs = append(pm.legs, leg{marketID: m.ID, outcome: outcomeYes})
	return true
}

func (b *matchBuilder) groupSelection(m market, spec marketSpec) (canon.Selection, bool) {
	if spec.code == canon.CodeCS {
		label, ok := exactScoreLabel(m.Title)
		return canon.Selection{Label: label}, ok
	}
	if t := m.VariantDetails.Sports.Team; t != nil {
		side, ok := b.teams.sideOf(t.Abbreviation)
		return canon.Selection{Side: side}, ok
	}
	return canon.Selection{Side: canon.SideDraw}, m.Title == drawTitle
}

func (b *matchBuilder) addTeamBinary(m market, spec marketSpec, firstOutcomeLine *float64) bool {
	if len(m.Outcomes) != binaryOutcomes {
		return false
	}
	side, ok := b.teams.sideOfOutcome(m.Outcomes[firstOutcome])
	switch {
	case ok && side == canon.SideHome:
		return b.addBinary(m, spec, firstOutcomeLine, nil, canon.SideHome, canon.SideAway)
	case ok && side == canon.SideAway:
		return b.addBinary(m, spec, homeHandicapFromAway(firstOutcomeLine), nil, canon.SideAway, canon.SideHome)
	}
	return false
}

func homeHandicapFromAway(awayLine *float64) *float64 {
	if awayLine == nil {
		return nil
	}
	return canon.Line(-*awayLine)
}

func (b *matchBuilder) addBinary(m market, spec marketSpec, line *float64, player *canon.SourcePlayer, yesSide, noSide canon.Side) bool {
	raw := b.rawMarket(spec, strconv.FormatInt(m.ID, 10), line, player)
	raw.Selections = []canon.Selection{{Side: yesSide}, {Side: noSide}}
	b.markets = append(b.markets, pricedMarket{
		raw:  raw,
		legs: []leg{{marketID: m.ID, outcome: outcomeYes}, {marketID: m.ID, outcome: outcomeNo}},
	})
	return true
}

func (b *matchBuilder) rawMarket(spec marketSpec, sourceMarketID string, line *float64, player *canon.SourcePlayer) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  b.matchID,
		Code:           spec.code,
		Line:           line,
		Player:         player,
		Period:         spec.period,
		Rules:          spec.code.DefaultRule(),
		SourceMarketID: sourceMarketID,
	}
}
