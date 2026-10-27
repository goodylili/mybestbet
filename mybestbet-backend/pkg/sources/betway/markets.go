package betway

import (
	"regexp"
	"strings"

	"mybestbet/pkg/canon"
)

type selectionSource int

const (
	fromOutcomeCode selectionSource = iota
	fromScoreName
	playerYes
	playerAtLeastCount
)

type marketSpec struct {
	code              canon.MarketCode
	period            string
	hasLine           bool
	selections        selectionSource
	specifier         string
	periodFromQuarter bool
	teamTotal         bool
}

const (
	firstGoalSpecifier = "goalnr=1"
	firstSetSpecifier  = "setnr=1"
	firstMapSpecifier  = "mapnr=1"
	quarterSpecifier   = "quarternr="
)

var quarterPeriods = map[string]string{
	"1": canon.PeriodQuarter1,
	"2": canon.PeriodQuarter2,
	"3": canon.PeriodQuarter3,
	"4": canon.PeriodQuarter4,
}

var headToHeadMarkerTypes = []string{"186", "219", "223", "225"}

var headToHeadSpecs = map[string]marketSpec{
	"219": {code: canon.CodeMoneyline},
	"223": {code: canon.CodeSpread, hasLine: true},
	"225": {code: canon.CodeTotal, hasLine: true},
	"64":  {code: canon.CodeMoneyline, period: canon.PeriodFirstHalf},
	"66":  {code: canon.CodeSpread, period: canon.PeriodFirstHalf, hasLine: true},
	"68":  {code: canon.CodeTotal, period: canon.PeriodFirstHalf, hasLine: true},
	"231": {code: canon.CodeSpread, period: canon.PeriodSecondHalf, hasLine: true},
	"232": {code: canon.CodeTotal, period: canon.PeriodSecondHalf, hasLine: true},
	"302": {code: canon.CodeMoneyline, periodFromQuarter: true},
	"303": {code: canon.CodeSpread, hasLine: true, periodFromQuarter: true},
	"236": {code: canon.CodeTotal, hasLine: true, periodFromQuarter: true},
	"186": {code: canon.CodeMoneyline},
	"202": {code: canon.CodeMoneyline, period: canon.PeriodFirstSet, specifier: firstSetSpecifier},
	"188": {code: canon.CodeSetHcp, hasLine: true},
	"189": {code: canon.CodeTotalGames, hasLine: true},
	"327": {code: canon.CodeMapHcp, hasLine: true},
	"328": {code: canon.CodeTotalMaps, hasLine: true},
	"330": {code: canon.CodeMoneyline, period: canon.PeriodFirstMap, specifier: firstMapSpecifier},
}

var footballSpecs = map[string]marketSpec{
	"1":    {code: canon.Code1X2},
	"10":   {code: canon.CodeDC},
	"18":   {code: canon.CodeOU, hasLine: true},
	"29":   {code: canon.CodeBTTS},
	"11":   {code: canon.CodeDNB},
	"16":   {code: canon.CodeAH, hasLine: true},
	"45":   {code: canon.CodeCS, selections: fromScoreName},
	"47":   {code: canon.CodeHTFT},
	"60":   {code: canon.Code1H1X2, period: canon.PeriodFirstHalf},
	"166":  {code: canon.CodeCornOU, hasLine: true},
	"2":    {code: canon.CodeQualify},
	"63":   {code: canon.CodeDC, period: canon.PeriodFirstHalf},
	"85":   {code: canon.CodeDC, period: canon.PeriodSecondHalf},
	"64":   {code: canon.CodeDNB, period: canon.PeriodFirstHalf},
	"86":   {code: canon.CodeDNB, period: canon.PeriodSecondHalf},
	"75":   {code: canon.CodeBTTS, period: canon.PeriodFirstHalf},
	"95":   {code: canon.CodeBTTS, period: canon.PeriodSecondHalf},
	"66":   {code: canon.CodeAH, period: canon.PeriodFirstHalf, hasLine: true},
	"88":   {code: canon.CodeAH, period: canon.PeriodSecondHalf, hasLine: true},
	"68":   {code: canon.CodeOU, period: canon.PeriodFirstHalf, hasLine: true},
	"90":   {code: canon.CodeOU, period: canon.PeriodSecondHalf, hasLine: true},
	"19":   {code: canon.TeamGoals, hasLine: true, teamTotal: true},
	"20":   {code: canon.TeamGoals, hasLine: true, teamTotal: true},
	"69":   {code: canon.TeamGoals, period: canon.PeriodFirstHalf, hasLine: true, teamTotal: true},
	"70":   {code: canon.TeamGoals, period: canon.PeriodFirstHalf, hasLine: true, teamTotal: true},
	"91":   {code: canon.TeamGoals, period: canon.PeriodSecondHalf, hasLine: true, teamTotal: true},
	"92":   {code: canon.TeamGoals, period: canon.PeriodSecondHalf, hasLine: true, teamTotal: true},
	"40":   {code: canon.PropAnytime, selections: playerYes},
	"38":   {code: canon.PropFirst, selections: playerYes, specifier: firstGoalSpecifier},
	"39":   {code: canon.PropLast, selections: playerYes},
	"1412": {code: canon.PropShots, selections: playerAtLeastCount},
	"1413": {code: canon.PropSOT, selections: playerAtLeastCount},
	"1415": {code: canon.PropAssists, selections: playerAtLeastCount},
}

var outcomeSelections = map[string]canon.Selection{
	"1":    {Side: canon.SideHome},
	"2":    {Side: canon.SideDraw},
	"3":    {Side: canon.SideAway},
	"9":    {Label: "1X"},
	"10":   {Label: "12"},
	"11":   {Label: "X2"},
	"4":    {Side: canon.SideHome},
	"5":    {Side: canon.SideAway},
	"12":   {Side: canon.SideOver},
	"13":   {Side: canon.SideUnder},
	"74":   {Side: canon.SideYes},
	"76":   {Side: canon.SideNo},
	"1714": {Side: canon.SideHome},
	"1715": {Side: canon.SideAway},
	"418":  {Label: "1/1"},
	"420":  {Label: "1/X"},
	"422":  {Label: "1/2"},
	"424":  {Label: "X/1"},
	"426":  {Label: "X/X"},
	"428":  {Label: "X/2"},
	"430":  {Label: "2/1"},
	"432":  {Label: "2/X"},
	"434":  {Label: "2/2"},
}

const noGoalOutcomeCode = "1716"

var teamTotalName = regexp.MustCompile(`^(?:(?:1st|2nd) Half - )?(.+) Total \([^)]*\)$`)

func scoreLabel(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), ":", "-")
}

func teamOfTotal(displayName string) (string, bool) {
	g := teamTotalName.FindStringSubmatch(strings.TrimSpace(displayName))
	if g == nil {
		return "", false
	}
	team := strings.TrimSpace(g[1])
	return team, team != ""
}
