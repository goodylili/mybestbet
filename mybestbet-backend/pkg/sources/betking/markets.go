package betking

import (
	"regexp"
	"strings"

	"mybestbet/pkg/canon"
)

type selectionSource int

const (
	fromOddType selectionSource = iota
	fromScoreName
	fromPlayerName
)

type marketSpec struct {
	code         canon.MarketCode
	period       string
	hasLine      bool
	selections   selectionSource
	firstSetOnly bool
	teamOrder    int
}

type marketKey struct {
	sport      sportID
	oddsTypeID int
}

var marketSpecs = map[marketKey]marketSpec{
	{sportFootball, 110}:   {code: canon.Code1X2},
	{sportFootball, 146}:   {code: canon.CodeDC},
	{sportFootball, 160}:   {code: canon.CodeOU, hasLine: true},
	{sportFootball, 302}:   {code: canon.CodeBTTS},
	{sportFootball, 147}:   {code: canon.CodeDNB},
	{sportFootball, 305}:   {code: canon.CodeAH, hasLine: true},
	{sportFootball, 102}:   {code: canon.CodeCS, selections: fromScoreName},
	{sportFootball, 9881}:  {code: canon.CodeHTFT},
	{sportFootball, 142}:   {code: canon.Code1H1X2, period: canon.PeriodFirstHalf},
	{sportFootball, 190}:   {code: canon.CodeCornOU, hasLine: true},
	{sportFootball, 318}:   {code: canon.CodeQualify},
	{sportFootball, 161}:   {code: canon.CodeOU, period: canon.PeriodFirstHalf, hasLine: true},
	{sportFootball, 9280}:  {code: canon.CodeOU, period: canon.PeriodSecondHalf, hasLine: true},
	{sportFootball, 344}:   {code: canon.CodeAH, period: canon.PeriodFirstHalf, hasLine: true},
	{sportFootball, 9335}:  {code: canon.CodeAH, period: canon.PeriodSecondHalf, hasLine: true},
	{sportFootball, 9588}:  {code: canon.CodeDC, period: canon.PeriodFirstHalf},
	{sportFootball, 10299}: {code: canon.CodeDC, period: canon.PeriodSecondHalf},
	{sportFootball, 9887}:  {code: canon.CodeBTTS, period: canon.PeriodFirstHalf},
	{sportFootball, 9889}:  {code: canon.CodeBTTS, period: canon.PeriodSecondHalf},
	{sportFootball, 9328}:  {code: canon.CodeDNB, period: canon.PeriodFirstHalf},
	{sportFootball, 10300}: {code: canon.CodeDNB, period: canon.PeriodSecondHalf},
	{sportFootball, 10298}: {code: canon.CodeCS, period: canon.PeriodFirstHalf, selections: fromScoreName},
	{sportFootball, 1018}:  {code: canon.CodeCS, period: canon.PeriodSecondHalf, selections: fromScoreName},
	{sportFootball, 9626}:  {code: canon.PropAnytime, selections: fromPlayerName},
	{sportFootball, 9625}:  {code: canon.PropFirst, selections: fromPlayerName},
	{sportFootball, 9627}:  {code: canon.PropLast, selections: fromPlayerName},
	{sportFootball, 10283}: {code: canon.TeamGoals, hasLine: true, teamOrder: homeTeamOrder},
	{sportFootball, 10284}: {code: canon.TeamGoals, hasLine: true, teamOrder: awayTeamOrder},
	{sportFootball, 10290}: {code: canon.TeamGoals, period: canon.PeriodFirstHalf, hasLine: true, teamOrder: homeTeamOrder},
	{sportFootball, 10291}: {code: canon.TeamGoals, period: canon.PeriodFirstHalf, hasLine: true, teamOrder: awayTeamOrder},
	{sportFootball, 10302}: {code: canon.TeamGoals, period: canon.PeriodSecondHalf, hasLine: true, teamOrder: homeTeamOrder},
	{sportFootball, 10303}: {code: canon.TeamGoals, period: canon.PeriodSecondHalf, hasLine: true, teamOrder: awayTeamOrder},

	{sportBasketball, 9300}:  {code: canon.CodeMoneyline},
	{sportBasketball, 9322}:  {code: canon.CodeSpread, hasLine: true},
	{sportBasketball, 9302}:  {code: canon.CodeTotal, hasLine: true},
	{sportBasketball, 344}:   {code: canon.CodeSpread, period: canon.PeriodFirstHalf, hasLine: true},
	{sportBasketball, 161}:   {code: canon.CodeTotal, period: canon.PeriodFirstHalf, hasLine: true},
	{sportBasketball, 9280}:  {code: canon.CodeTotal, period: canon.PeriodSecondHalf, hasLine: true},
	{sportBasketball, 10414}: {code: canon.CodeSpread, period: canon.PeriodQuarter1, hasLine: true},
	{sportBasketball, 10406}: {code: canon.CodeTotal, period: canon.PeriodQuarter1, hasLine: true},

	{sportAmericanFootball, 9300}: {code: canon.CodeMoneyline},
	{sportAmericanFootball, 9322}: {code: canon.CodeSpread, hasLine: true},
	{sportAmericanFootball, 9302}: {code: canon.CodeTotal, hasLine: true},

	{sportTennis, 9388}: {code: canon.CodeMoneyline},
	{sportTennis, 9389}: {code: canon.CodeMoneyline, period: canon.PeriodFirstSet, firstSetOnly: true},
	{sportTennis, 285}:  {code: canon.CodeTotalGames, hasLine: true},
}

var oddTypeSelections = map[int]canon.Selection{
	4:    {Side: canon.SideHome},
	2:    {Side: canon.SideDraw},
	5:    {Side: canon.SideAway},
	9:    {Label: "1X"},
	10:   {Label: "12"},
	11:   {Label: "X2"},
	12:   {Side: canon.SideOver},
	13:   {Side: canon.SideUnder},
	74:   {Side: canon.SideYes},
	76:   {Side: canon.SideNo},
	1714: {Side: canon.SideHome},
	1715: {Side: canon.SideAway},
	418:  {Label: "1/1"},
	420:  {Label: "1/X"},
	422:  {Label: "1/2"},
	424:  {Label: "X/1"},
	426:  {Label: "X/X"},
	428:  {Label: "X/2"},
	430:  {Label: "2/1"},
	432:  {Label: "2/X"},
	434:  {Label: "2/2"},
}

var basketballPlayerStat = regexp.MustCompile(`^\[(.+?)\] (.+) Total (Points|Rebounds|Assists) \(Incl\. Overtime\)$`)

var playerStatCodes = map[string]canon.MarketCode{
	"Points":   canon.PropPoints,
	"Rebounds": canon.PropRebounds,
	"Assists":  canon.PropBkAssists,
}

type playerStat struct {
	team   string
	player string
	code   canon.MarketCode
}

func parsePlayerStat(sport sportID, oddsTypeName string) (playerStat, bool) {
	if sport != sportBasketball {
		return playerStat{}, false
	}
	parts := basketballPlayerStat.FindStringSubmatch(strings.TrimSpace(oddsTypeName))
	if parts == nil {
		return playerStat{}, false
	}
	return playerStat{team: parts[1], player: parts[2], code: playerStatCodes[parts[3]]}, true
}

const otherScoresName = "Others"

func scoreLabel(oddName string) string {
	if oddName == otherScoresName {
		return "Other"
	}
	return strings.ReplaceAll(oddName, ":", "-")
}
