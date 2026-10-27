package bet365

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

const (
	kickoffLayout = "20060102150405"
	evensDecimal  = 2.0
)

type teams struct {
	home string
	away string
}

const awayAtHomeSeparator = " @ "

func teamsFromFixtureName(name string) (teams, bool) {
	if away, home, ok := strings.Cut(name, awayAtHomeSeparator); ok {
		return teams{home: strings.TrimSpace(home), away: strings.TrimSpace(away)}, true
	}
	for _, sep := range []string{" v ", " vs "} {
		if home, away, ok := strings.Cut(name, sep); ok {
			return teams{home: strings.TrimSpace(home), away: strings.TrimSpace(away)}, true
		}
	}
	return teams{}, false
}

func parseKickoff(raw string) time.Time {
	t, err := time.Parse(kickoffLayout, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func decimalOdds(raw string) (float64, bool) {
	if strings.EqualFold(raw, "EVS") || strings.EqualFold(raw, "evens") {
		return evensDecimal, true
	}
	if num, den, isFractional := strings.Cut(raw, "/"); isFractional {
		n, errN := strconv.ParseFloat(num, 64)
		d, errD := strconv.ParseFloat(den, 64)
		if errN != nil || errD != nil || d <= 0 || n <= 0 {
			return 0, false
		}
		return roundOdds(1 + n/d), true
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 1 {
		return 0, false
	}
	return v, true
}

func roundOdds(v float64) float64 { return math.Round(v*1000) / 1000 }

func priceOf(p participant, observedAt time.Time) (canon.Price, bool) {
	odds, ok := decimalOdds(p.odds)
	if !ok {
		return canon.Price{}, false
	}
	price := canon.NewPrice(odds, canon.FormDecimal, observedAt)
	price.Suspended = p.isSuspended
	return price, true
}

func parseLine(raw string) (float64, bool) {
	var sum float64
	parts := strings.Split(strings.TrimSpace(raw), ",")
	for _, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return 0, false
		}
		sum += v
	}
	return sum / float64(len(parts)), true
}

func homeHandicap(raw string, side canon.Side) (float64, bool) {
	handicap, ok := parseLine(raw)
	if side == canon.SideAway {
		handicap = -handicap
	}
	return handicap, ok
}

var totalSidesByPrefix = map[string]canon.Side{"o": canon.SideOver, "u": canon.SideUnder}

func parseTotalHandicap(raw string) (canon.Side, float64, bool) {
	prefix, value, ok := strings.Cut(strings.TrimSpace(raw), " ")
	if !ok {
		return "", 0, false
	}
	side, isSide := totalSidesByPrefix[strings.ToLower(prefix)]
	line, hasLine := parseLine(value)
	return side, line, isSide && hasLine
}

func outcomeSymbol(name string, t teams) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return "", false
	}
	switch normalized {
	case strings.ToLower(t.home), "1":
		return "1", true
	case "draw", "x":
		return "X", true
	case strings.ToLower(t.away), "2":
		return "2", true
	}
	return "", false
}

var sidesBySymbol = map[string]canon.Side{"1": canon.SideHome, "X": canon.SideDraw, "2": canon.SideAway}

func outcomeSide(name string, t teams) (canon.Side, bool) {
	symbol, ok := outcomeSymbol(name, t)
	return sidesBySymbol[symbol], ok
}

func doubleChanceLabel(name string, t teams) (string, bool) {
	first, second, ok := strings.Cut(name, " or ")
	if !ok {
		return "", false
	}
	a, okA := outcomeSymbol(first, t)
	b, okB := outcomeSymbol(second, t)
	if !okA || !okB || a == b {
		return "", false
	}
	pair := []string{a, b}
	slices.SortFunc(pair, func(x, y string) int { return symbolOrder(x) - symbolOrder(y) })
	return pair[0] + pair[1], true
}

func symbolOrder(symbol string) int { return strings.Index("1X2", symbol) }

func halfTimeFullTimeLabel(name string, t teams) (string, bool) {
	half, full, ok := strings.Cut(name, " - ")
	if !ok {
		return "", false
	}
	h, okH := outcomeSymbol(half, t)
	f, okF := outcomeSymbol(full, t)
	if !okH || !okF {
		return "", false
	}
	return h + "/" + f, true
}

func yesNoSide(name string) (canon.Side, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "yes":
		return canon.SideYes, true
	case "no":
		return canon.SideNo, true
	}
	return "", false
}

func totalsSide(columnName string) (canon.Side, bool) {
	switch strings.ToLower(strings.TrimSpace(columnName)) {
	case "over":
		return canon.SideOver, true
	case "under":
		return canon.SideUnder, true
	}
	return "", false
}

func correctScoreLabel(name string, isAwayColumn bool) (string, bool) {
	if strings.Contains(strings.ToLower(name), "other") {
		return "Other", true
	}
	winner, loser, ok := strings.Cut(strings.TrimSpace(name), "-")
	if !ok {
		return "", false
	}
	if _, err := strconv.Atoi(winner); err != nil {
		return "", false
	}
	if _, err := strconv.Atoi(loser); err != nil {
		return "", false
	}
	if isAwayColumn {
		return loser + "-" + winner, true
	}
	return winner + "-" + loser, true
}

func thresholdLine(columnName string) (float64, bool) {
	count, ok := strings.CutSuffix(strings.TrimSpace(columnName), "+")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(count)
	if err != nil || n < 1 {
		return 0, false
	}
	return float64(n) - 0.5, true
}

func playerSourceID(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), "-")
}
