package server

import (
	"cmp"
	"slices"

	"mybestbet/pkg/canon"
)

const consensusSource = "consensus"

type marketGroup struct {
	key       canon.MarketKey
	keyString string
	cells     []canon.BoardCell
}

var sideOrder = map[canon.Side]int{
	canon.SideHome: 1, canon.SideDraw: 2, canon.SideAway: 3,
	canon.SideOver: 4, canon.SideUnder: 5, canon.SideYes: 6, canon.SideNo: 7,
}

var labelOrder = map[string]int{"1X": 1, "12": 2, "X2": 3, "Other": 99}

func groupByMarket(cells []canon.BoardCell) []marketGroup {
	byKey := map[string]*marketGroup{}
	for _, cell := range cells {
		group, seen := byKey[cell.MarketKey]
		if !seen {
			key, err := canon.ParseMarketKey(cell.MarketKey)
			if err != nil {
				continue
			}
			group = &marketGroup{key: key, keyString: cell.MarketKey}
			byKey[cell.MarketKey] = group
		}
		group.cells = append(group.cells, cell)
	}
	groups := make([]marketGroup, 0, len(byKey))
	for _, group := range byKey {
		groups = append(groups, *group)
	}
	slices.SortFunc(groups, compareMarketGroups)
	return groups
}

func compareMarketGroups(a, b marketGroup) int {
	return cmp.Or(
		cmp.Compare(a.key.Code, b.key.Code),
		cmp.Compare(a.key.PlayerID, b.key.PlayerID),
		cmp.Compare(a.key.TeamID, b.key.TeamID),
		cmp.Compare(a.key.Period, b.key.Period),
		cmp.Compare(lineOrZero(a.key.Line), lineOrZero(b.key.Line)),
	)
}

func lineOrZero(line *float64) float64 {
	if line == nil {
		return 0
	}
	return *line
}

func selectionRows(cells []canon.BoardCell) []SelectionRow {
	bySelection := map[string]*SelectionRow{}
	competing := map[string][]canon.BoardCell{}
	for _, cell := range cells {
		id := string(cell.Side) + "|" + cell.Label
		row, seen := bySelection[id]
		if !seen {
			row = &SelectionRow{Side: string(cell.Side), Label: cell.Label, Cells: map[string]canon.BoardCell{}}
			bySelection[id] = row
		}
		if cell.Source == consensusSource {
			row.Consensus = &cell
			continue
		}
		row.Cells[cell.Source] = cell
		competing[id] = append(competing[id], cell)
	}
	rows := make([]SelectionRow, 0, len(bySelection))
	for id, row := range bySelection {
		if best, ok := bestCell(competing[id]); ok {
			row.BestSource = best.Source
		}
		rows = append(rows, *row)
	}
	slices.SortFunc(rows, compareSelections)
	return rows
}

func compareSelections(a, b SelectionRow) int {
	return cmp.Or(
		cmp.Compare(sideOrder[canon.Side(a.Side)], sideOrder[canon.Side(b.Side)]),
		cmp.Compare(labelOrder[a.Label], labelOrder[b.Label]),
		cmp.Compare(a.Label, b.Label),
	)
}

func isBettable(cell canon.BoardCell) bool {
	return cell.Source != consensusSource && !cell.Derived && !cell.Price.Suspended && cell.Price.Prob > 0
}

func bestCell(cells []canon.BoardCell) (canon.BoardCell, bool) {
	var best canon.BoardCell
	found := false
	for _, cell := range cells {
		if !isBettable(cell) {
			continue
		}
		if !found || cell.Price.Prob < best.Price.Prob {
			best, found = cell, true
		}
	}
	return best, found
}

func sourceColumns(cells []canon.BoardCell) []string {
	sources := []string{}
	for _, cell := range cells {
		if cell.Source != consensusSource && !slices.Contains(sources, cell.Source) {
			sources = append(sources, cell.Source)
		}
	}
	slices.Sort(sources)
	return sources
}

func marketBoard(group marketGroup) MarketBoard {
	return MarketBoard{Key: group.key, KeyString: group.keyString, Selections: selectionRows(group.cells)}
}

var periodOrder = map[string]int{
	canon.PeriodFullTime: 1, canon.PeriodFirstHalf: 2, canon.PeriodSecondHalf: 3,
	canon.PeriodQuarter1: 4, canon.PeriodQuarter2: 5, canon.PeriodQuarter3: 6, canon.PeriodQuarter4: 7,
	canon.PeriodFirstSet: 8, canon.PeriodFirstMap: 9, canon.PeriodMaps1To2: 10,
}

func comparePeriods(a, b MarketPeriod) int {
	return cmp.Or(cmp.Compare(rankOrLast(periodOrder, a.Period), rankOrLast(periodOrder, b.Period)), cmp.Compare(a.Period, b.Period))
}

func rankOrLast(ranks map[string]int, value string) int {
	if rank, ok := ranks[value]; ok {
		return rank
	}
	return len(ranks) + 1
}

func eventDetail(event EventInfo, cells []canon.BoardCell) EventDetail {
	boardsByCode := map[canon.MarketCode]map[string][]MarketBoard{}
	var matchCells []canon.BoardCell
	for _, group := range groupByMarket(cells) {
		if group.key.PlayerID != "" {
			continue
		}
		if _, known := canon.SpecOf(group.key.Code); !known {
			continue
		}
		byPeriod := boardsByCode[group.key.Code]
		if byPeriod == nil {
			byPeriod = map[string][]MarketBoard{}
			boardsByCode[group.key.Code] = byPeriod
		}
		byPeriod[group.key.Period] = append(byPeriod[group.key.Period], marketBoard(group))
		matchCells = append(matchCells, group.cells...)
	}
	return EventDetail{Event: event, Sources: sourceColumns(matchCells), Markets: marketTabs(boardsByCode)}
}

func marketTabs(boardsByCode map[canon.MarketCode]map[string][]MarketBoard) []MarketTab {
	supplied := make(map[canon.MarketCode]bool, len(boardsByCode))
	for code := range boardsByCode {
		supplied[code] = true
	}
	tabs := []MarketTab{}
	for _, code := range canon.InCanonicalOrder(supplied) {
		spec, _ := canon.SpecOf(code)
		tab := MarketTab{Code: code, Kind: spec.Kind, Shape: spec.Shape}
		for period, boards := range boardsByCode[code] {
			tab.Periods = append(tab.Periods, MarketPeriod{Period: period, Boards: boards})
		}
		slices.SortFunc(tab.Periods, comparePeriods)
		tabs = append(tabs, tab)
	}
	return tabs
}

func bestOutcomes(key canon.MarketKey, cells []canon.BoardCell) BestOutcomes {
	bySide := map[canon.Side][]canon.BoardCell{}
	for _, cell := range cells {
		bySide[cell.Side] = append(bySide[cell.Side], cell)
	}
	return BestOutcomes{
		Market: key.Code,
		Line:   key.Line,
		Period: key.Period,
		Home:   bestPrice(bySide[canon.SideHome]),
		Draw:   bestPrice(bySide[canon.SideDraw]),
		Away:   bestPrice(bySide[canon.SideAway]),
		Over:   bestPrice(bySide[canon.SideOver]),
		Under:  bestPrice(bySide[canon.SideUnder]),
		Yes:    bestPrice(bySide[canon.SideYes]),
		No:     bestPrice(bySide[canon.SideNo]),
	}
}

func bestPrice(cells []canon.BoardCell) *BestPrice {
	best, ok := bestCell(cells)
	if !ok {
		return nil
	}
	return &BestPrice{Source: best.Source, Price: best.Price, DeepLink: best.DeepLink}
}
