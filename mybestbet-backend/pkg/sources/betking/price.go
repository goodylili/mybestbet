package betking

import (
	"strconv"
	"time"

	"mybestbet/pkg/canon"
)

const firstSet = 1

type collectionKey struct {
	itemID       int64
	collectionID int64
}

func mergeFragments(areas []areaMatch) []matchItem {
	var merged []matchItem
	positionByItem := map[int64]int{}
	seenCollections := map[collectionKey]bool{}
	for _, area := range areas {
		for _, fragment := range area.Items {
			pos, ok := positionByItem[fragment.ItemID]
			if !ok {
				pos = len(merged)
				positionByItem[fragment.ItemID] = pos
				header := fragment
				header.OddsCollection = nil
				header.sport = area.SportID
				merged = append(merged, header)
			}
			if merged[pos].Goalscorer == nil {
				merged[pos].Goalscorer = fragment.Goalscorer
			}
			for _, collection := range fragment.OddsCollection {
				key := collectionKey{itemID: fragment.ItemID, collectionID: collection.OddCollectionID}
				if seenCollections[key] {
					continue
				}
				seenCollections[key] = true
				merged[pos].OddsCollection = append(merged[pos].OddsCollection, collection)
			}
		}
	}
	return merged
}

func (a *Adapter) itemMarkets(item matchItem, fetchedAt time.Time) []canon.RawMarket {
	eventID := strconv.FormatInt(item.ItemID, 10)
	teamByPlayer := playerTeams(item.Goalscorer)
	var markets []canon.RawMarket
	for _, collection := range item.OddsCollection {
		typeID := collection.OddsType.OddsTypeID
		if stat, ok := parsePlayerStat(item.sport, collection.OddsType.OddsTypeName); ok {
			markets = append(markets, playerStatMarkets(eventID, typeID, stat, collection.MatchOdds, fetchedAt)...)
			continue
		}
		spec, ok := marketSpecs[marketKey{sport: item.sport, oddsTypeID: typeID}]
		if !ok {
			a.deps.DropUnknownMarket(Name, strconv.Itoa(int(item.sport))+":"+strconv.Itoa(typeID))
			continue
		}
		if spec.selections == fromPlayerName {
			markets = append(markets, playerMarkets(eventID, typeID, spec, collection.MatchOdds, teamByPlayer, fetchedAt)...)
			continue
		}
		lines := lineMarkets(eventID, typeID, spec, collection.MatchOdds, fetchedAt)
		for i := range lines {
			lines[i].Team = teamOfOrder(item.Teams, spec.teamOrder)
		}
		markets = append(markets, lines...)
	}
	return markets
}

func teamOfOrder(teams []team, order int) string {
	for _, t := range teams {
		if t.ItemOrder == order {
			return t.Name
		}
	}
	return ""
}

func playerTeams(scorers *goalscorer) map[string]string {
	teams := map[string]string{}
	if scorers == nil {
		return teams
	}
	for _, t := range scorers.GoalscorerTeams {
		for _, p := range t.GoalscorerPlayers {
			teams[p.PlayerName] = t.TeamName
		}
	}
	return teams
}

func lineMarkets(eventID string, typeID int, spec marketSpec, odds []matchOdd, fetchedAt time.Time) []canon.RawMarket {
	var markets []canon.RawMarket
	positionByLine := map[float64]int{}
	for _, odd := range odds {
		sel, ok := selectionFor(spec, odd, fetchedAt)
		if !ok {
			continue
		}
		line := 0.0
		if spec.hasLine {
			line = odd.OddAttribute.SpecialValue
		}
		pos, seen := positionByLine[line]
		if !seen {
			pos = len(markets)
			positionByLine[line] = pos
			markets = append(markets, newMarket(eventID, typeID, spec, line))
		}
		markets[pos].Selections = append(markets[pos].Selections, sel)
	}
	return markets
}

func newMarket(eventID string, typeID int, spec marketSpec, line float64) canon.RawMarket {
	m := canon.RawMarket{
		EventSourceID:  eventID,
		Code:           spec.code,
		Period:         spec.period,
		Rules:          spec.code.DefaultRule(),
		SourceMarketID: strconv.Itoa(typeID),
	}
	if spec.hasLine {
		m.Line = canon.Line(line)
		m.SourceMarketID += "@" + strconv.FormatFloat(line, 'f', -1, 64)
	}
	return m
}

func playerStatMarkets(eventID string, typeID int, stat playerStat, odds []matchOdd, fetchedAt time.Time) []canon.RawMarket {
	markets := lineMarkets(eventID, typeID, marketSpec{code: stat.code, hasLine: true}, odds, fetchedAt)
	for i := range markets {
		markets[i].SourceMarketID += "#" + stat.player
		markets[i].Player = &canon.SourcePlayer{SourceID: stat.player, Name: stat.player, Team: stat.team}
	}
	return markets
}

func playerMarkets(eventID string, typeID int, spec marketSpec, odds []matchOdd, teamByPlayer map[string]string, fetchedAt time.Time) []canon.RawMarket {
	var markets []canon.RawMarket
	for _, odd := range odds {
		sel, ok := selectionFor(spec, odd, fetchedAt)
		if !ok {
			continue
		}
		name := odd.OddAttribute.OddName
		m := newMarket(eventID, typeID, spec, 0)
		m.SourceMarketID += "#" + name
		m.Player = &canon.SourcePlayer{SourceID: name, Name: name, Team: teamByPlayer[name]}
		m.Selections = []canon.Selection{sel}
		markets = append(markets, m)
	}
	return markets
}

func selectionFor(spec marketSpec, odd matchOdd, fetchedAt time.Time) (canon.Selection, bool) {
	if odd.Outcome.OddOutcome <= 1 || (spec.firstSetOnly && odd.OddAttribute.SpecialValue != firstSet) {
		return canon.Selection{}, false
	}
	var sel canon.Selection
	switch spec.selections {
	case fromScoreName:
		sel.Label = scoreLabel(odd.OddAttribute.OddName)
	case fromPlayerName:
		sel.Side = canon.SideYes
	default:
		known, ok := oddTypeSelections[odd.OddAttribute.OddTypeID]
		if !ok {
			return canon.Selection{}, false
		}
		sel = known
	}
	sel.Price = canon.NewPrice(odd.Outcome.OddOutcome, canon.FormDecimal, fetchedAt)
	sel.Price.Suspended = odd.Outcome.ScheduleStatus != 0
	return sel, true
}
