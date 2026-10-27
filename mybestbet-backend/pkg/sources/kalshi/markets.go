package kalshi

import (
	"strings"

	"mybestbet/pkg/canon"
)

type marketShape int

const (
	shapeOutcomeGroup marketShape = iota + 1
	shapeWinnerGroup
	shapeOverUnder
	shapeYesNo
	shapeHandicap
	shapePlayerOverUnder
	shapePlayerFirstThreshold
	shapeTeamOverUnder
)

type seriesSpec struct {
	suffix      string
	eventSuffix string
	code        canon.MarketCode
	period      string
	shape       marketShape
}

type family struct {
	sport           canon.Sport
	gameSuffix      string
	awayListedFirst bool
	series          []seriesSpec
}

const (
	gameSeriesSuffix  = "GAME"
	matchSeriesSuffix = "MATCH"
	firstMapOrSet     = "-1"
)

var soccerFamily = family{
	sport:      canon.SportFootball,
	gameSuffix: gameSeriesSuffix,
	series: []seriesSpec{
		{gameSeriesSuffix, "", canon.Code1X2, canon.PeriodFullTime, shapeOutcomeGroup},
		{"1H", "", canon.Code1H1X2, canon.PeriodFirstHalf, shapeOutcomeGroup},
		{"SCORE", "", canon.CodeCS, canon.PeriodFullTime, shapeOutcomeGroup},
		{"TOTAL", "", canon.CodeOU, canon.PeriodFullTime, shapeOverUnder},
		{"CORNERS", "", canon.CodeCornOU, canon.PeriodFullTime, shapeOverUnder},
		{"BTTS", "", canon.CodeBTTS, canon.PeriodFullTime, shapeYesNo},
		{"SPREAD", "", canon.CodeAH, canon.PeriodFullTime, shapeHandicap},
		{"ADVANCE", "", canon.CodeQualify, canon.PeriodFullTime, shapeWinnerGroup},
		{"TEAMTOTAL", "", canon.TeamGoals, canon.PeriodFullTime, shapeTeamOverUnder},
	},
}

var pointsCoreSeries = []seriesSpec{
	{gameSeriesSuffix, "", canon.CodeMoneyline, canon.PeriodFullTime, shapeWinnerGroup},
	{"SPREAD", "", canon.CodeSpread, canon.PeriodFullTime, shapeHandicap},
	{"TOTAL", "", canon.CodeTotal, canon.PeriodFullTime, shapeOverUnder},
}

type seriesPeriod struct {
	seriesPrefix string
	period       string
}

var periodLineSeries = periodLines([]seriesPeriod{
	{"1H", canon.PeriodFirstHalf},
	{"2H", canon.PeriodSecondHalf},
	{"1Q", canon.PeriodQuarter1},
	{"2Q", canon.PeriodQuarter2},
	{"3Q", canon.PeriodQuarter3},
	{"4Q", canon.PeriodQuarter4},
})

var basketballCoreFamily = family{
	sport:           canon.SportBasketball,
	gameSuffix:      gameSeriesSuffix,
	awayListedFirst: true,
	series:          pointsCoreSeries,
}

var nbaFamily = family{
	sport:           canon.SportBasketball,
	gameSuffix:      gameSeriesSuffix,
	awayListedFirst: true,
	series: concat(pointsCoreSeries, periodLineSeries, []seriesSpec{
		{"PTS", "", canon.PropPoints, canon.PeriodFullTime, shapePlayerOverUnder},
		{"REB", "", canon.PropRebounds, canon.PeriodFullTime, shapePlayerOverUnder},
		{"AST", "", canon.PropBkAssists, canon.PeriodFullTime, shapePlayerOverUnder},
		{"3PT", "", canon.PropThrees, canon.PeriodFullTime, shapePlayerOverUnder},
		{"PRA", "", canon.PropPtsRebAst, canon.PeriodFullTime, shapePlayerOverUnder},
	}),
}

var gridironFamily = family{
	sport:           canon.SportAmericanFootball,
	gameSuffix:      gameSeriesSuffix,
	awayListedFirst: true,
	series: concat(pointsCoreSeries, periodLineSeries, []seriesSpec{
		{"PASSYDS", "", canon.PropPassYards, canon.PeriodFullTime, shapePlayerOverUnder},
		{"RSHYDS", "", canon.PropRushYards, canon.PeriodFullTime, shapePlayerOverUnder},
		{"RECYDS", "", canon.PropRecYards, canon.PeriodFullTime, shapePlayerOverUnder},
		{"REC", "", canon.PropReceptions, canon.PeriodFullTime, shapePlayerOverUnder},
		{"PASSTDS", "", canon.PropPassTDs, canon.PeriodFullTime, shapePlayerOverUnder},
		{"TD", "", canon.PropAnytimeTD, canon.PeriodFullTime, shapePlayerFirstThreshold},
	}),
}

var tennisFamily = family{
	sport:      canon.SportTennis,
	gameSuffix: matchSeriesSuffix,
	series: []seriesSpec{
		{matchSeriesSuffix, "", canon.CodeMoneyline, canon.PeriodFullTime, shapeWinnerGroup},
		{"SETWINNER", firstMapOrSet, canon.CodeMoneyline, canon.PeriodFirstSet, shapeWinnerGroup},
		{"GTOTAL", "", canon.CodeTotalGames, canon.PeriodFullTime, shapeOverUnder},
	},
}

var esportsFamily = family{
	sport:      canon.SportEsports,
	gameSuffix: gameSeriesSuffix,
	series: []seriesSpec{
		{gameSeriesSuffix, "", canon.CodeMoneyline, canon.PeriodFullTime, shapeWinnerGroup},
		{"MAP", firstMapOrSet, canon.CodeMoneyline, canon.PeriodFirstMap, shapeWinnerGroup},
		{"SPREAD", "", canon.CodeMapHcp, canon.PeriodFullTime, shapeHandicap},
		{"TOTALMAPS", "", canon.CodeTotalMaps, canon.PeriodFullTime, shapeOverUnder},
	},
}

func periodLines(periods []seriesPeriod) []seriesSpec {
	specs := make([]seriesSpec, 0, 2*len(periods))
	for _, p := range periods {
		specs = append(specs,
			seriesSpec{p.seriesPrefix + "SPREAD", "", canon.CodeSpread, p.period, shapeHandicap},
			seriesSpec{p.seriesPrefix + "TOTAL", "", canon.CodeTotal, p.period, shapeOverUnder},
		)
	}
	return specs
}

func concat(groups ...[]seriesSpec) []seriesSpec {
	var all []seriesSpec
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}

const (
	tieSuffix             = "TIE"
	activeStatus          = "active"
	strikeGreater         = "greater"
	strikeGreaterOrEqual  = "greater_or_equal"
	wholeToHalfLineOffset = 0.5
	firstThresholdLine    = 0.5
	playerNameSeparator   = ":"
)

func overLine(m market) (*float64, bool) {
	if m.FloorStrike == nil {
		return nil, false
	}
	switch m.StrikeType {
	case strikeGreater:
		return canon.Line(*m.FloorStrike), true
	case strikeGreaterOrEqual:
		return canon.Line(*m.FloorStrike - wholeToHalfLineOffset), true
	}
	return nil, false
}

func scoreLabel(m market) (string, bool) {
	s := m.CustomStrike
	if s.HomeScore == "" || s.AwayScore == "" {
		return "", false
	}
	return s.HomeScore + "-" + s.AwayScore, true
}

func marketSuffix(ev event, m market) string {
	return strings.TrimPrefix(m.Ticker, ev.EventTicker+"-")
}

func playerName(m market) string {
	name, _, _ := strings.Cut(m.YesSubTitle, playerNameSeparator)
	return strings.TrimSpace(name)
}
