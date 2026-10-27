package polymarket

import (
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

type pricedMarket struct {
	raw    canon.RawMarket
	tokens []string
}

func (pm *pricedMarket) applyBooks(books map[string]*ladder, observedAt time.Time) {
	for i, token := range pm.tokens {
		pm.raw.Selections[i].Price = priceFromLadder(books[token], observedAt)
	}
}

func priceFromLadder(l *ladder, observedAt time.Time) canon.Price {
	if l == nil {
		return missingBookPrice(observedAt)
	}
	return l.touchPrice(observedAt)
}

type matchTeams struct {
	home gammaTeam
	away gammaTeam
}

func teamsOf(events []gammaEvent) (matchTeams, bool) {
	for _, ev := range events {
		var t matchTeams
		for _, team := range ev.Teams {
			switch team.Ordering {
			case homeSuffix:
				t.home = team
			case awaySuffix:
				t.away = team
			}
		}
		if t.home.Name != "" && t.away.Name != "" {
			return t, true
		}
	}
	return matchTeams{}, false
}

func (t matchTeams) sideFor(suffix string) (canon.Side, bool) {
	switch suffix {
	case drawSuffix:
		return canon.SideDraw, true
	case homeSuffix, t.home.Abbreviation:
		return canon.SideHome, true
	case awaySuffix, t.away.Abbreviation:
		return canon.SideAway, true
	}
	return "", false
}

func (t matchTeams) sideOfOutcome(outcome string) (canon.Side, bool) {
	switch outcome {
	case "":
		return "", false
	case t.home.Name, t.home.Alias:
		return canon.SideHome, true
	case t.away.Name, t.away.Alias:
		return canon.SideAway, true
	}
	return "", false
}

type matchBuilder struct {
	matchID string
	teams   matchTeams
	table   marketTable
	groups  map[string]int
	markets []pricedMarket
	drop    func(sourceMarketID string)
}

func newMatchBuilder(matchID string, teams matchTeams, table marketTable, drop func(string)) *matchBuilder {
	return &matchBuilder{matchID: matchID, teams: teams, table: table, groups: map[string]int{}, drop: drop}
}

func (b *matchBuilder) addEvent(ev gammaEvent) {
	for _, m := range ev.Markets {
		if !m.isTradable() {
			continue
		}
		spec, known := b.table.specFor(m)
		if !known || !b.addMarket(ev, m, spec) {
			b.drop(m.Slug)
		}
	}
}

func (b *matchBuilder) addMarket(ev gammaEvent, m gammaMarket, spec marketSpec) bool {
	switch spec.shape {
	case shapeOutcomeGroup:
		return b.addGroupSelection(ev, m, spec)
	case shapeOverUnder:
		return m.Line != nil && b.addTwoWay(m, spec, canon.Line(*m.Line), nil, canon.SideOver, canon.SideUnder)
	case shapeYesNo:
		return b.addTwoWay(m, spec, nil, nil, canon.SideYes, canon.SideNo)
	case shapeHandicap:
		return m.Line != nil && b.addTeamTwoWay(m, spec, *m.Line)
	case shapeTeamTwoWay:
		return m.Line == nil && b.addTeamTwoWay(m, spec, 0)
	case shapePlayerOverUnder:
		player, ok := playerOf(m)
		return ok && m.Line != nil && b.addTwoWay(m, spec, canon.Line(*m.Line), player, canon.SideOver, canon.SideUnder)
	case shapePlayerYesNo:
		player, ok := playerOf(m)
		isYesNoLine := m.Line == nil || *m.Line == yesNoPlayerLine
		return ok && isYesNoLine && b.addTwoWay(m, spec, nil, player, canon.SideYes, canon.SideNo)
	case shapeTeamOverUnder:
		return m.Line != nil && b.addTeamOverUnder(m, spec)
	}
	return false
}

func (b *matchBuilder) addTeamOverUnder(m gammaMarket, spec marketSpec) bool {
	suffix, ok := teamTotalSuffix(m.Slug)
	if !ok {
		return false
	}
	team := b.teams.home.Name
	if suffix == awaySuffix {
		team = b.teams.away.Name
	}
	raw := b.rawMarket(spec, m.Slug, canon.Line(*m.Line), nil)
	raw.Team = team
	raw.Selections = []canon.Selection{{Side: canon.SideOver}, {Side: canon.SideUnder}}
	b.markets = append(b.markets, pricedMarket{raw: raw, tokens: []string(m.ClobTokenIDs)})
	return true
}

func (b *matchBuilder) addGroupSelection(ev gammaEvent, m gammaMarket, spec marketSpec) bool {
	suffix := strings.TrimPrefix(m.Slug, ev.Slug+"-")
	var sel canon.Selection
	if spec.code == canon.CodeCS {
		label, ok := exactScoreLabel(suffix)
		if !ok {
			return false
		}
		sel.Label = label
	} else {
		side, ok := b.teams.sideFor(suffix)
		if !ok {
			return false
		}
		sel.Side = side
	}
	idx, exists := b.groups[ev.Slug]
	if !exists {
		idx = len(b.markets)
		b.groups[ev.Slug] = idx
		b.markets = append(b.markets, pricedMarket{raw: b.rawMarket(spec, ev.Slug, nil, nil)})
	}
	pm := &b.markets[idx]
	pm.raw.Selections = append(pm.raw.Selections, sel)
	pm.tokens = append(pm.tokens, m.ClobTokenIDs[firstOutcome])
	return true
}

func (b *matchBuilder) addTeamTwoWay(m gammaMarket, spec marketSpec, firstOutcomeLine float64) bool {
	if len(m.Outcomes) != tokensPerBinaryMarket {
		return false
	}
	first, ok := b.teams.sideOfOutcome(m.Outcomes[firstOutcome])
	if !ok {
		return false
	}
	homeLine := firstOutcomeLine
	firstSide, secondSide := canon.SideHome, canon.SideAway
	if first == canon.SideAway {
		homeLine = -firstOutcomeLine
		firstSide, secondSide = canon.SideAway, canon.SideHome
	}
	var line *float64
	if spec.shape == shapeHandicap {
		line = canon.Line(homeLine)
	}
	return b.addTwoWay(m, spec, line, nil, firstSide, secondSide)
}

func (b *matchBuilder) addTwoWay(m gammaMarket, spec marketSpec, line *float64, player *canon.SourcePlayer, firstSide, secondSide canon.Side) bool {
	raw := b.rawMarket(spec, m.Slug, line, player)
	raw.Selections = []canon.Selection{{Side: firstSide}, {Side: secondSide}}
	b.markets = append(b.markets, pricedMarket{raw: raw, tokens: []string(m.ClobTokenIDs)})
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
