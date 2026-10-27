package betpawa

import "mybestbet/pkg/canon"

type lineSource int

const (
	noLine lineSource = iota
	lineFromTotal
	lineFromHandicap
)

type selectionSource int

const (
	fromTypeID selectionSource = iota
	fromPriceName
	playerYes
	playerAtLeastNumber
)

type marketSpec struct {
	code       canon.MarketCode
	period     string
	line       lineSource
	selections selectionSource
	goalNumber string
	team       int
}

const firstGoal = "1"

type marketKey struct {
	category     categoryID
	marketTypeID string
}

var marketSpecs = map[marketKey]marketSpec{
	{categoryFootball, "3743"}:     {code: canon.Code1X2},
	{categoryFootball, "4693"}:     {code: canon.CodeDC},
	{categoryFootball, "5000"}:     {code: canon.CodeOU, line: lineFromTotal},
	{categoryFootball, "3795"}:     {code: canon.CodeBTTS},
	{categoryFootball, "4703"}:     {code: canon.CodeDNB},
	{categoryFootball, "3774"}:     {code: canon.CodeAH, line: lineFromHandicap},
	{categoryFootball, "28000869"}: {code: canon.CodeCS, selections: fromPriceName},
	{categoryFootball, "4706"}:     {code: canon.CodeHTFT, selections: fromPriceName},
	{categoryFootball, "3668"}:     {code: canon.Code1H1X2, period: canon.PeriodFirstHalf},
	{categoryFootball, "1096783"}:  {code: canon.CodeCornOU, line: lineFromTotal},
	{categoryFootball, "4958"}:     {code: canon.CodeOU, period: canon.PeriodFirstHalf, line: lineFromTotal},
	{categoryFootball, "4976"}:     {code: canon.CodeOU, period: canon.PeriodSecondHalf, line: lineFromTotal},
	{categoryFootball, "3747"}:     {code: canon.CodeAH, period: canon.PeriodFirstHalf, line: lineFromHandicap},
	{categoryFootball, "3756"}:     {code: canon.CodeAH, period: canon.PeriodSecondHalf, line: lineFromHandicap},
	{categoryFootball, "4673"}:     {code: canon.CodeDC, period: canon.PeriodFirstHalf},
	{categoryFootball, "4681"}:     {code: canon.CodeDC, period: canon.PeriodSecondHalf},
	{categoryFootball, "3789"}:     {code: canon.CodeBTTS, period: canon.PeriodFirstHalf},
	{categoryFootball, "3792"}:     {code: canon.CodeBTTS, period: canon.PeriodSecondHalf},
	{categoryFootball, "4697"}:     {code: canon.CodeDNB, period: canon.PeriodFirstHalf},
	{categoryFootball, "4700"}:     {code: canon.CodeDNB, period: canon.PeriodSecondHalf},
	{categoryFootball, "3819"}:     {code: canon.CodeCS, period: canon.PeriodFirstHalf, selections: fromPriceName},
	{categoryFootball, "4063"}:     {code: canon.CodeCS, period: canon.PeriodSecondHalf, selections: fromPriceName},
	{categoryFootball, "129108"}:   {code: canon.PropAnytime, selections: playerYes},
	{categoryFootball, "129110"}:   {code: canon.PropFirst, selections: playerYes, goalNumber: firstGoal},
	{categoryFootball, "129109"}:   {code: canon.PropLast, selections: playerYes},
	{categoryFootball, "28000815"}: {code: canon.PropGA, selections: playerYes},
	{categoryFootball, "28000816"}: {code: canon.PropCard, selections: playerYes},
	{categoryFootball, "28000818"}: {code: canon.PropShots, selections: playerAtLeastNumber},
	{categoryFootball, "28000819"}: {code: canon.PropSOT, selections: playerAtLeastNumber},
	{categoryFootball, "28000827"}: {code: canon.PropSaves, selections: playerAtLeastNumber},
	{categoryFootball, "5006"}:     {code: canon.TeamGoals, line: lineFromTotal, team: homePosition},
	{categoryFootball, "5003"}:     {code: canon.TeamGoals, line: lineFromTotal, team: awayPosition},
	{categoryFootball, "4964"}:     {code: canon.TeamGoals, period: canon.PeriodFirstHalf, line: lineFromTotal, team: homePosition},
	{categoryFootball, "4961"}:     {code: canon.TeamGoals, period: canon.PeriodFirstHalf, line: lineFromTotal, team: awayPosition},
	{categoryFootball, "4982"}:     {code: canon.TeamGoals, period: canon.PeriodSecondHalf, line: lineFromTotal, team: homePosition},
	{categoryFootball, "4979"}:     {code: canon.TeamGoals, period: canon.PeriodSecondHalf, line: lineFromTotal, team: awayPosition},

	{categoryBasketball, "4791"}: {code: canon.CodeMoneyline},
	{categoryBasketball, "4697"}: {code: canon.CodeMoneyline, period: canon.PeriodFirstHalf},
	{categoryBasketball, "4700"}: {code: canon.CodeMoneyline, period: canon.PeriodSecondHalf},
	{categoryBasketball, "3777"}: {code: canon.CodeSpread, line: lineFromHandicap},
	{categoryBasketball, "3747"}: {code: canon.CodeSpread, period: canon.PeriodFirstHalf, line: lineFromHandicap},
	{categoryBasketball, "3756"}: {code: canon.CodeSpread, period: canon.PeriodSecondHalf, line: lineFromHandicap},
	{categoryBasketball, "3753"}: {code: canon.CodeSpread, period: canon.PeriodQuarter1, line: lineFromHandicap},
	{categoryBasketball, "3762"}: {code: canon.CodeSpread, period: canon.PeriodQuarter2, line: lineFromHandicap},
	{categoryBasketball, "3768"}: {code: canon.CodeSpread, period: canon.PeriodQuarter3, line: lineFromHandicap},
	{categoryBasketball, "3771"}: {code: canon.CodeSpread, period: canon.PeriodQuarter4, line: lineFromHandicap},
	{categoryBasketball, "5009"}: {code: canon.CodeTotal, line: lineFromTotal},
	{categoryBasketball, "4958"}: {code: canon.CodeTotal, period: canon.PeriodFirstHalf, line: lineFromTotal},
	{categoryBasketball, "4976"}: {code: canon.CodeTotal, period: canon.PeriodSecondHalf, line: lineFromTotal},
	{categoryBasketball, "4973"}: {code: canon.CodeTotal, period: canon.PeriodQuarter1, line: lineFromTotal},
	{categoryBasketball, "4988"}: {code: canon.CodeTotal, period: canon.PeriodQuarter2, line: lineFromTotal},
	{categoryBasketball, "4994"}: {code: canon.CodeTotal, period: canon.PeriodQuarter3, line: lineFromTotal},
	{categoryBasketball, "4997"}: {code: canon.CodeTotal, period: canon.PeriodQuarter4, line: lineFromTotal},

	{categoryTennis, "2043818"}: {code: canon.CodeMoneyline},
	{categoryTennis, "4758"}:    {code: canon.CodeMoneyline, period: canon.PeriodFirstSet},
	{categoryTennis, "4895"}:    {code: canon.CodeTotalGames, line: lineFromTotal},
	{categoryTennis, "4880"}:    {code: canon.CodeTotalGames, period: canon.PeriodFirstSet, line: lineFromTotal},
}

var typeIDSelections = map[string]canon.Selection{
	"3744":    {Side: canon.SideHome},
	"3745":    {Side: canon.SideDraw},
	"3746":    {Side: canon.SideAway},
	"4694":    {Label: "1X"},
	"4695":    {Label: "X2"},
	"4696":    {Label: "12"},
	"5001":    {Side: canon.SideOver},
	"5002":    {Side: canon.SideUnder},
	"3796":    {Side: canon.SideYes},
	"3797":    {Side: canon.SideNo},
	"4704":    {Side: canon.SideHome},
	"4705":    {Side: canon.SideAway},
	"3775":    {Side: canon.SideHome},
	"3776":    {Side: canon.SideAway},
	"3669":    {Side: canon.SideHome},
	"3670":    {Side: canon.SideDraw},
	"3671":    {Side: canon.SideAway},
	"1099466": {Side: canon.SideOver},
	"1099467": {Side: canon.SideUnder},
	"4674":    {Label: "1X"},
	"4675":    {Label: "X2"},
	"4676":    {Label: "12"},
	"4682":    {Label: "1X"},
	"4683":    {Label: "X2"},
	"4684":    {Label: "12"},
	"3790":    {Side: canon.SideYes},
	"3791":    {Side: canon.SideNo},
	"3793":    {Side: canon.SideYes},
	"3794":    {Side: canon.SideNo},
	"5007":    {Side: canon.SideOver},
	"5008":    {Side: canon.SideUnder},
	"5004":    {Side: canon.SideOver},
	"5005":    {Side: canon.SideUnder},
	"4965":    {Side: canon.SideOver},
	"4966":    {Side: canon.SideUnder},
	"4962":    {Side: canon.SideOver},
	"4963":    {Side: canon.SideUnder},
	"4983":    {Side: canon.SideOver},
	"4984":    {Side: canon.SideUnder},
	"4980":    {Side: canon.SideOver},
	"4981":    {Side: canon.SideUnder},

	"4792":    {Side: canon.SideHome},
	"4793":    {Side: canon.SideAway},
	"4698":    {Side: canon.SideHome},
	"4699":    {Side: canon.SideAway},
	"4701":    {Side: canon.SideHome},
	"4702":    {Side: canon.SideAway},
	"3778":    {Side: canon.SideHome},
	"3779":    {Side: canon.SideAway},
	"3748":    {Side: canon.SideHome},
	"3749":    {Side: canon.SideAway},
	"3757":    {Side: canon.SideHome},
	"3758":    {Side: canon.SideAway},
	"3754":    {Side: canon.SideHome},
	"3755":    {Side: canon.SideAway},
	"3763":    {Side: canon.SideHome},
	"3764":    {Side: canon.SideAway},
	"3769":    {Side: canon.SideHome},
	"3770":    {Side: canon.SideAway},
	"3772":    {Side: canon.SideHome},
	"3773":    {Side: canon.SideAway},
	"5010":    {Side: canon.SideOver},
	"5011":    {Side: canon.SideUnder},
	"4959":    {Side: canon.SideOver},
	"4960":    {Side: canon.SideUnder},
	"4977":    {Side: canon.SideOver},
	"4978":    {Side: canon.SideUnder},
	"4974":    {Side: canon.SideOver},
	"4975":    {Side: canon.SideUnder},
	"4989":    {Side: canon.SideOver},
	"4990":    {Side: canon.SideUnder},
	"4995":    {Side: canon.SideOver},
	"4996":    {Side: canon.SideUnder},
	"4998":    {Side: canon.SideOver},
	"4999":    {Side: canon.SideUnder},
	"2043821": {Side: canon.SideHome},
	"2043823": {Side: canon.SideAway},
	"4759":    {Side: canon.SideHome},
	"4760":    {Side: canon.SideAway},
	"4896":    {Side: canon.SideOver},
	"4897":    {Side: canon.SideUnder},
	"4881":    {Side: canon.SideOver},
	"4882":    {Side: canon.SideUnder},
}

var noScorerTypeIDs = map[string]bool{
	"129111": true,
	"130473": true,
}
