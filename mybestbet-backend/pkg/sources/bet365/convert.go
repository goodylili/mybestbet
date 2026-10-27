package bet365

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

type groupConverter struct {
	fixtureID  string
	teams      teams
	group      *marketGroup
	spec       marketSpec
	observedAt time.Time
	idPrefix   string
}

func (c groupConverter) markets() []canon.RawMarket {
	switch c.spec.shape {
	case shapeOutcomes:
		return c.singleMarket(func(p participant) (canon.Side, string, bool) {
			side, ok := outcomeSide(p.name, c.teams)
			return side, "", ok
		})
	case shapeTwoWay:
		return c.twoWay()
	case shapeDoubleChance:
		return c.singleMarket(func(p participant) (canon.Side, string, bool) {
			label, ok := doubleChanceLabel(p.name, c.teams)
			return "", label, ok
		})
	case shapeYesNo:
		return c.singleMarket(func(p participant) (canon.Side, string, bool) {
			side, ok := yesNoSide(p.name)
			return side, "", ok
		})
	case shapeHalfTimeFullTime:
		return c.singleMarket(func(p participant) (canon.Side, string, bool) {
			label, ok := halfTimeFullTimeLabel(p.name, c.teams)
			return "", label, ok
		})
	case shapeCorrectScore:
		return c.correctScore()
	case shapeTotals:
		return c.totals()
	case shapeAsianHandicap:
		return c.asianHandicap()
	case shapeScorers:
		return c.scorers()
	case shapePlayerThresholds:
		return c.playerThresholds()
	case shapePlayerYes:
		return c.playerYes()
	case shapePlayerOverUnder:
		return c.playerOverUnder()
	case shapeGameLines:
		return c.gameLines()
	}
	return nil
}

func (c groupConverter) newMarket(sourceMarketID string, line *float64, player *canon.SourcePlayer, code canon.MarketCode) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  c.fixtureID,
		Code:           code,
		Line:           line,
		Player:         player,
		Period:         c.spec.period,
		Rules:          code.DefaultRule(),
		SourceMarketID: sourceMarketID,
	}
}

func (c groupConverter) marketID() string {
	if c.idPrefix != "" {
		return c.idPrefix
	}
	return c.group.id
}

func (c groupConverter) forCode(code canon.MarketCode) groupConverter {
	c.spec.code = code
	c.idPrefix = c.group.id + ":" + string(code)
	return c
}

func (c groupConverter) singleMarket(classify func(participant) (canon.Side, string, bool)) []canon.RawMarket {
	m := c.newMarket(c.marketID(), nil, nil, c.spec.code)
	for _, col := range c.group.columns {
		for _, p := range col.participants {
			side, label, ok := classify(p)
			price, priced := priceOf(p, c.observedAt)
			if ok && priced {
				m.Selections = append(m.Selections, canon.Selection{Side: side, Label: label, Price: price})
			}
		}
	}
	return nonEmpty(m)
}

func (c groupConverter) twoWay() []canon.RawMarket {
	m := c.newMarket(c.marketID(), nil, nil, c.spec.code)
	for _, col := range c.group.columns {
		for _, p := range col.participants {
			side, ok := outcomeSide(p.name, c.teams)
			if ok && side == canon.SideDraw {
				return nil
			}
			if price, priced := priceOf(p, c.observedAt); ok && priced {
				m.Selections = append(m.Selections, canon.Selection{Side: side, Price: price})
			}
		}
	}
	return nonEmpty(m)
}

func (c groupConverter) correctScore() []canon.RawMarket {
	m := c.newMarket(c.marketID(), nil, nil, c.spec.code)
	for i, col := range c.group.columns {
		side, named := outcomeSide(col.name, c.teams)
		isAwayColumn := side == canon.SideAway || (!named && i == 2)
		for _, p := range col.participants {
			label, ok := correctScoreLabel(p.name, isAwayColumn)
			price, priced := priceOf(p, c.observedAt)
			if ok && priced {
				m.Selections = append(m.Selections, canon.Selection{Label: label, Price: price})
			}
		}
	}
	return nonEmpty(m)
}

func (c groupConverter) totals() []canon.RawMarket {
	lineLabels := lineLabelColumn(c.group.columns)
	lines := lineMarkets{}
	for _, col := range c.group.columns {
		side, ok := totalsSide(col.name)
		if !ok {
			continue
		}
		for i, p := range col.participants {
			line, hasLine := parseLine(firstNonEmpty(p.handicap, p.name, labelAt(lineLabels, i)))
			price, priced := priceOf(p, c.observedAt)
			if hasLine && priced {
				lines.add(c, line, nil, canon.Selection{Side: side, Price: price})
			}
		}
	}
	return lines.markets
}

func (c groupConverter) asianHandicap() []canon.RawMarket {
	lines := lineMarkets{}
	for i, col := range c.group.columns {
		side, named := outcomeSide(col.name, c.teams)
		if !named {
			side = [...]canon.Side{canon.SideHome, canon.SideAway}[min(i, 1)]
		}
		if i > 1 || side == canon.SideDraw {
			continue
		}
		for _, p := range col.participants {
			homeLine, hasLine := homeHandicap(p.handicap, side)
			price, priced := priceOf(p, c.observedAt)
			if hasLine && priced {
				lines.add(c, homeLine, nil, canon.Selection{Side: side, Price: price})
			}
		}
	}
	return lines.markets
}

func (c groupConverter) scorers() []canon.RawMarket {
	var markets []canon.RawMarket
	for _, col := range c.group.columns {
		code, ok := scorerCodesByColumn[strings.ToLower(strings.TrimSpace(col.name))]
		if !ok {
			continue
		}
		for _, p := range col.participants {
			price, priced := priceOf(p, c.observedAt)
			if !priced || isNoScorer(p.name) {
				continue
			}
			player := &canon.SourcePlayer{SourceID: playerSourceID(p.name), Name: p.name}
			m := c.newMarket(fmt.Sprintf("%s:%s:%s", c.group.id, code, player.SourceID), nil, player, code)
			m.Selections = []canon.Selection{{Side: canon.SideYes, Price: price}}
			markets = append(markets, m)
		}
	}
	return markets
}

func (c groupConverter) playerThresholds() []canon.RawMarket {
	lines := lineMarkets{}
	for _, col := range c.group.columns {
		line, ok := thresholdLine(col.name)
		if !ok {
			continue
		}
		for _, p := range col.participants {
			price, priced := priceOf(p, c.observedAt)
			if priced {
				player := &canon.SourcePlayer{SourceID: playerSourceID(p.name), Name: p.name}
				lines.add(c, line, player, canon.Selection{Side: canon.SideOver, Price: price})
			}
		}
	}
	return lines.markets
}

func (c groupConverter) playerYes() []canon.RawMarket {
	var markets []canon.RawMarket
	for _, col := range c.group.columns {
		for _, p := range col.participants {
			price, priced := priceOf(p, c.observedAt)
			if !priced {
				continue
			}
			player := &canon.SourcePlayer{SourceID: playerSourceID(p.name), Name: p.name}
			m := c.newMarket(c.group.id+":"+player.SourceID, nil, player, c.spec.code)
			m.Selections = []canon.Selection{{Side: canon.SideYes, Price: price}}
			markets = append(markets, m)
		}
	}
	return markets
}

type lineMarkets struct {
	markets []canon.RawMarket
	index   map[string]int
}

func (l *lineMarkets) add(c groupConverter, line float64, player *canon.SourcePlayer, sel canon.Selection) {
	key := strconv.FormatFloat(line, 'f', -1, 64)
	if player != nil {
		key = player.SourceID + "@" + key
	}
	if l.index == nil {
		l.index = map[string]int{}
	}
	i, ok := l.index[key]
	if !ok {
		i = len(l.markets)
		l.index[key] = i
		l.markets = append(l.markets, c.newMarket(c.marketID()+"@"+key, canon.Line(line), player, c.spec.code))
	}
	l.markets[i].Selections = append(l.markets[i].Selections, sel)
}

func lineLabelColumn(columns []*column) []participant {
	for _, col := range columns {
		if _, isSide := totalsSide(col.name); isSide || len(col.participants) == 0 || col.participants[0].odds != "" {
			continue
		}
		return col.participants
	}
	return nil
}

func labelAt(labels []participant, i int) string {
	if i < len(labels) {
		return labels[i].name
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func isNoScorer(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "no goalscorer")
}

func nonEmpty(m canon.RawMarket) []canon.RawMarket {
	if len(m.Selections) == 0 {
		return nil
	}
	return []canon.RawMarket{m}
}
