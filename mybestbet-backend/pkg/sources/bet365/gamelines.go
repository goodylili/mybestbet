package bet365

import (
	"strings"

	"mybestbet/pkg/canon"
)

func (c groupConverter) gameLines() []canon.RawMarket {
	rowLabels := lineLabelColumn(c.group.columns)
	var markets []canon.RawMarket
	for _, col := range c.group.columns {
		lineColumn, ok := gameLineColumns[strings.ToLower(strings.TrimSpace(col.name))]
		if !ok {
			continue
		}
		converter := c.forCode(lineColumn.code)
		if lineColumn.isLine {
			markets = append(markets, converter.gameLineLadder(col, rowLabels)...)
			continue
		}
		markets = append(markets, converter.gameLineMoneyline(col, rowLabels)...)
	}
	return markets
}

func (c groupConverter) gameLineMoneyline(col *column, rowLabels []participant) []canon.RawMarket {
	m := c.newMarket(c.marketID(), nil, nil, c.spec.code)
	for i, p := range col.participants {
		side, isTeamRow := c.teamRowSide(labelAt(rowLabels, i))
		if price, priced := priceOf(p, c.observedAt); isTeamRow && priced {
			m.Selections = append(m.Selections, canon.Selection{Side: side, Price: price})
		}
	}
	return nonEmpty(m)
}

func (c groupConverter) gameLineLadder(col *column, rowLabels []participant) []canon.RawMarket {
	lines := lineMarkets{}
	for i, p := range col.participants {
		rowSide, isTeamRow := c.teamRowSide(labelAt(rowLabels, i))
		price, priced := priceOf(p, c.observedAt)
		if !isTeamRow || !priced {
			continue
		}
		if side, line, hasLine := c.gameLineSelection(p, rowSide); hasLine {
			lines.add(c, line, nil, canon.Selection{Side: side, Price: price})
		}
	}
	return lines.markets
}

func (c groupConverter) gameLineSelection(p participant, rowSide canon.Side) (canon.Side, float64, bool) {
	if c.spec.code == canon.CodeTotal {
		return parseTotalHandicap(firstNonEmpty(p.handicap, p.name))
	}
	line, ok := homeHandicap(p.handicap, rowSide)
	return rowSide, line, ok
}

func (c groupConverter) teamRowSide(label string) (canon.Side, bool) {
	side, ok := outcomeSide(label, c.teams)
	return side, ok && side != canon.SideDraw
}

func (c groupConverter) playerOverUnder() []canon.RawMarket {
	playerNames := lineLabelColumn(c.group.columns)
	lines := lineMarkets{}
	for _, col := range c.group.columns {
		side, isSideColumn := totalsSide(col.name)
		if !isSideColumn {
			continue
		}
		for i, p := range col.participants {
			name := firstNonEmpty(labelAt(playerNames, i), p.name)
			line, hasLine := parseLine(p.handicap)
			price, priced := priceOf(p, c.observedAt)
			if name == "" || !hasLine || !priced {
				continue
			}
			player := &canon.SourcePlayer{SourceID: playerSourceID(name), Name: strings.TrimSpace(name)}
			lines.add(c, line, player, canon.Selection{Side: side, Price: price})
		}
	}
	return lines.markets
}
