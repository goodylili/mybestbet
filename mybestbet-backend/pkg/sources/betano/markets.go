package betano

import (
	"regexp"
	"strings"

	"mybestbet/pkg/canon"
)

type selectionSource int

const (
	fromName selectionSource = iota
	fromOverUnderName
	fromHandicapColumn
	fromScoreName
	fromLabelName
	playerYes
	scorerTable
)

type marketSpec struct {
	code       canon.MarketCode
	period     string
	selections selectionSource
}

var marketSpecs = map[string]marketSpec{
	"MRES": {code: canon.Code1X2},
	"DBLC": {code: canon.CodeDC},
	"HCTG": {code: canon.CodeOU, selections: fromOverUnderName},
	"BTSC": {code: canon.CodeBTTS},
	"DNOB": {code: canon.CodeDNB},
	"AHRF": {code: canon.CodeAH, selections: fromHandicapColumn},
	"CSFT": {code: canon.CodeCS, selections: fromScoreName},
	"HTFT": {code: canon.CodeHTFT, selections: fromLabelName},
	"H1RS": {code: canon.Code1H1X2, period: canon.PeriodFirstHalf},
	"CNOU": {code: canon.CodeCornOU, selections: fromOverUnderName},
	"PTRC": {code: canon.PropCard, selections: playerYes},
	"PSCR": {selections: scorerTable},
}

var headToHeadSpecs = map[string]marketSpec{
	"MR12": {code: canon.CodeMoneyline},
	"HCOT": {code: canon.CodeSpread, selections: fromHandicapColumn},
	"OUOT": {code: canon.CodeTotal, selections: fromOverUnderName},
	"H1ML": {code: canon.CodeMoneyline, period: canon.PeriodFirstHalf},
	"H1HC": {code: canon.CodeSpread, period: canon.PeriodFirstHalf, selections: fromHandicapColumn},
	"H1OU": {code: canon.CodeTotal, period: canon.PeriodFirstHalf, selections: fromOverUnderName},
	"S1RS": {code: canon.CodeMoneyline, period: canon.PeriodFirstSet},
	"SETH": {code: canon.CodeSetHcp, selections: fromHandicapColumn},
	"TGOU": {code: canon.CodeTotalGames, selections: fromOverUnderName},
	"M1RS": {code: canon.CodeMoneyline, period: canon.PeriodFirstMap},
	"MAPH": {code: canon.CodeMapHcp, selections: fromHandicapColumn},
	"MAPT": {code: canon.CodeTotalMaps, selections: fromOverUnderName},
}

func specsFor(sport sportSlug) map[string]marketSpec {
	if sport == sportFootball {
		return marketSpecs
	}
	return headToHeadSpecs
}

var scorerColumnCodes = map[string]canon.MarketCode{
	"first":   canon.PropFirst,
	"anytime": canon.PropAnytime,
	"last":    canon.PropLast,
}

var nameSelections = map[string]canon.Selection{
	"1":   {Side: canon.SideHome},
	"X":   {Side: canon.SideDraw},
	"2":   {Side: canon.SideAway},
	"1X":  {Label: "1X"},
	"12":  {Label: "12"},
	"X2":  {Label: "X2"},
	"Yes": {Side: canon.SideYes},
	"No":  {Side: canon.SideNo},
}

const (
	homeColumn = 0
	awayColumn = 1
)

var exactScore = regexp.MustCompile(`^(\d+)\s*[-:]\s*(\d+)$`)

func scoreLabel(name string) string {
	parts := exactScore.FindStringSubmatch(strings.TrimSpace(name))
	if parts == nil {
		return "Other"
	}
	return parts[1] + "-" + parts[2]
}

func overUnderSide(name string) (canon.Side, bool) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "over"):
		return canon.SideOver, true
	case strings.HasPrefix(lower, "under"):
		return canon.SideUnder, true
	}
	return "", false
}

func scorerColumnCode(column string) (canon.MarketCode, bool) {
	lower := strings.ToLower(column)
	for keyword, code := range scorerColumnCodes {
		if strings.Contains(lower, keyword) {
			return code, true
		}
	}
	return "", false
}
