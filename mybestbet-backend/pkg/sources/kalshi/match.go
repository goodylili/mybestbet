package kalshi

import (
	"slices"
	"strings"

	"mybestbet/pkg/canon"
)

type leg struct {
	ticker  string
	outcome outcome
}

type pricedMarket struct {
	raw  canon.RawMarket
	legs []leg
}

const noGroupYet = -1

type eventBuilder struct {
	matchID string
	lineup  lineup
	spec    seriesSpec
	group   int
	markets []pricedMarket
}

func buildEventMarkets(matchID string, ev event, spec seriesSpec, players lineup, drop func(string)) []pricedMarket {
	if spec.shape == shapeWinnerGroup && offersTie(ev) {
		drop(ev.EventTicker)
		return nil
	}
	b := &eventBuilder{matchID: matchID, lineup: players, spec: spec, group: noGroupYet}
	for _, m := range ev.Markets {
		if m.Status != activeStatus {
			continue
		}
		if !b.add(ev, m) {
			drop(m.Ticker)
		}
	}
	return b.markets
}

func offersTie(ev event) bool {
	return slices.ContainsFunc(ev.Markets, func(m market) bool { return marketSuffix(ev, m) == tieSuffix })
}

func (b *eventBuilder) add(ev event, m market) bool {
	switch b.spec.shape {
	case shapeOutcomeGroup, shapeWinnerGroup:
		return b.addGroupSelection(ev, m)
	case shapeOverUnder:
		line, ok := overLine(m)
		return ok && b.addBinary(b.rawMarket(m.Ticker, line), m, canon.SideOver, canon.SideUnder)
	case shapeYesNo:
		return b.addBinary(b.rawMarket(m.Ticker, nil), m, canon.SideYes, canon.SideNo)
	case shapeHandicap:
		return b.addHandicap(ev, m)
	case shapePlayerOverUnder:
		return b.addPlayerOverUnder(ev, m)
	case shapePlayerFirstThreshold:
		return b.addPlayerFirstThreshold(ev, m)
	case shapeTeamOverUnder:
		return b.addTeamOverUnder(ev, m)
	}
	return false
}

func (b *eventBuilder) addTeamOverUnder(ev event, m market) bool {
	line, ok := overLine(m)
	if !ok {
		return false
	}
	side, ok := b.lineup.sideOfMarginMarket(m, marketSuffix(ev, m))
	if !ok {
		return false
	}
	raw := b.rawMarket(m.Ticker, line)
	raw.Team = b.lineup.teamName(side)
	return b.addBinary(raw, m, canon.SideOver, canon.SideUnder)
}

func (b *eventBuilder) addGroupSelection(ev event, m market) bool {
	var sel canon.Selection
	if b.spec.code == canon.CodeCS {
		label, ok := scoreLabel(m)
		if !ok {
			return false
		}
		sel.Label = label
	} else {
		side, ok := b.lineup.sideOfCode(marketSuffix(ev, m))
		if !ok {
			return false
		}
		sel.Side = side
	}
	if b.group == noGroupYet {
		b.group = len(b.markets)
		b.markets = append(b.markets, pricedMarket{raw: b.rawMarket(ev.EventTicker, nil)})
	}
	pm := &b.markets[b.group]
	pm.raw.Selections = append(pm.raw.Selections, sel)
	pm.legs = append(pm.legs, leg{ticker: m.Ticker, outcome: outcomeYes})
	return true
}

func (b *eventBuilder) addHandicap(ev event, m market) bool {
	if m.FloorStrike == nil || m.StrikeType != strikeGreater {
		return false
	}
	margin := *m.FloorStrike
	side, ok := b.lineup.sideOfMarginMarket(m, marketSuffix(ev, m))
	switch {
	case !ok:
		return false
	case side == canon.SideHome:
		return b.addBinary(b.rawMarket(m.Ticker, canon.Line(-margin)), m, canon.SideHome, canon.SideAway)
	case side == canon.SideAway:
		return b.addBinary(b.rawMarket(m.Ticker, canon.Line(margin)), m, canon.SideAway, canon.SideHome)
	}
	return false
}

func (b *eventBuilder) addPlayerOverUnder(ev event, m market) bool {
	line, ok := overLine(m)
	if !ok {
		return false
	}
	raw := b.rawMarket(m.Ticker, line)
	raw.Player = b.playerOf(ev, m)
	return b.addBinary(raw, m, canon.SideOver, canon.SideUnder)
}

func (b *eventBuilder) addPlayerFirstThreshold(ev event, m market) bool {
	line, ok := overLine(m)
	if !ok || *line != firstThresholdLine {
		return false
	}
	raw := b.rawMarket(m.Ticker, nil)
	raw.Player = b.playerOf(ev, m)
	return b.addBinary(raw, m, canon.SideYes, canon.SideNo)
}

func (b *eventBuilder) playerOf(ev event, m market) *canon.SourcePlayer {
	playerCode, _, _ := strings.Cut(marketSuffix(ev, m), "-")
	id := m.CustomStrike.player()
	if id == "" {
		id = playerCode
	}
	return &canon.SourcePlayer{SourceID: id, Name: playerName(m), Team: b.teamOfPlayer(m, playerCode)}
}

func (b *eventBuilder) teamOfPlayer(m market, playerCode string) string {
	side, ok := b.lineup.sideOfTeam(m.CustomStrike.team())
	if !ok {
		side, ok = b.lineup.sideOfPlayerCode(playerCode)
	}
	if !ok {
		return ""
	}
	return b.lineup.teamName(side)
}

func (b *eventBuilder) addBinary(raw canon.RawMarket, m market, yesSide, noSide canon.Side) bool {
	raw.Selections = []canon.Selection{{Side: yesSide}, {Side: noSide}}
	b.markets = append(b.markets, pricedMarket{
		raw:  raw,
		legs: []leg{{ticker: m.Ticker, outcome: outcomeYes}, {ticker: m.Ticker, outcome: outcomeNo}},
	})
	return true
}

func (b *eventBuilder) rawMarket(sourceMarketID string, line *float64) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  b.matchID,
		Code:           b.spec.code,
		Line:           line,
		Period:         b.spec.period,
		Rules:          b.spec.code.DefaultRule(),
		SourceMarketID: sourceMarketID,
	}
}
