package predictfun

import (
	"maps"
	"regexp"
	"strconv"
	"strings"

	"mybestbet/pkg/canon"
)

type marketShape int

const (
	shapeOutcomeGroup marketShape = iota + 1
	shapeOverUnder
	shapeYesNo
	shapeHandicap
	shapeTeamBinary
	shapeFirstMapWinner
	shapePlayerOverUnder
	shapePlayerYesNo
	shapeTeamOverUnder
)

type marketSpec struct {
	code   canon.MarketCode
	period string
	shape  marketShape
}

var footballSpecs = map[string]marketSpec{
	"SPORTS_MONEYLINE":                       {canon.Code1X2, canon.PeriodFullTime, shapeOutcomeGroup},
	"SPORTS_HALFTIME_RESULT":                 {canon.Code1H1X2, canon.PeriodFirstHalf, shapeOutcomeGroup},
	"SPORTS_EXACT_SCORE":                     {canon.CodeCS, canon.PeriodFullTime, shapeOutcomeGroup},
	"SPORTS_TOTALS":                          {canon.CodeOU, canon.PeriodFullTime, shapeOverUnder},
	"SPORTS_FIRST_HALF_TOTALS":               {canon.CodeOU, canon.PeriodFirstHalf, shapeOverUnder},
	"SPORTS_SECOND_HALF_TOTALS":              {canon.CodeOU, canon.PeriodSecondHalf, shapeOverUnder},
	"SPORTS_BOTH_TEAMS_TO_SCORE":             {canon.CodeBTTS, canon.PeriodFullTime, shapeYesNo},
	"SPORTS_BOTH_TEAMS_TO_SCORE_FIRST_HALF":  {canon.CodeBTTS, canon.PeriodFirstHalf, shapeYesNo},
	"SPORTS_BOTH_TEAMS_TO_SCORE_SECOND_HALF": {canon.CodeBTTS, canon.PeriodSecondHalf, shapeYesNo},
	"SPORTS_SPREADS":                         {canon.CodeAH, canon.PeriodFullTime, shapeHandicap},
	"SPORTS_TOTAL_CORNERS":                   {canon.CodeCornOU, canon.PeriodFullTime, shapeOverUnder},
	"SPORTS_FIRST_HALF_TOTAL_CORNERS":        {canon.CodeCornOU, canon.PeriodFirstHalf, shapeOverUnder},
	"SPORTS_SECOND_HALF_TOTAL_CORNERS":       {canon.CodeCornOU, canon.PeriodSecondHalf, shapeOverUnder},
	"SPORTS_TEAM_TO_ADVANCE":                 {canon.CodeQualify, canon.PeriodFullTime, shapeTeamBinary},
	"SPORTS_PLAYER_GOALS":                    {canon.PropGoals, canon.PeriodFullTime, shapePlayerOverUnder},
	"SPORTS_PLAYER_SHOTS":                    {canon.PropShots, canon.PeriodFullTime, shapePlayerOverUnder},
	"SPORTS_PLAYER_SHOTS_ON_TARGET":          {canon.PropSOT, canon.PeriodFullTime, shapePlayerOverUnder},
	"SPORTS_PLAYER_ASSISTS":                  {canon.PropAssists, canon.PeriodFullTime, shapePlayerOverUnder},
	"SPORTS_PLAYER_GOALS_PLUS_ASSISTS":       {canon.PropGA, canon.PeriodFullTime, shapePlayerYesNo},
	"SPORTS_PLAYER_GOALKEEPER_SAVES":         {canon.PropSaves, canon.PeriodFullTime, shapePlayerOverUnder},
	"SPORTS_TEAM_TOTALS":                     {canon.TeamGoals, canon.PeriodFullTime, shapeTeamOverUnder},
}

var pointsSportSpecs = map[string]marketSpec{
	"SPORTS_MONEYLINE":          {canon.CodeMoneyline, canon.PeriodFullTime, shapeTeamBinary},
	"SPORTS_SPREADS":            {canon.CodeSpread, canon.PeriodFullTime, shapeHandicap},
	"SPORTS_TOTALS":             {canon.CodeTotal, canon.PeriodFullTime, shapeOverUnder},
	"SPORTS_FIRST_HALF_TOTALS":  {canon.CodeTotal, canon.PeriodFirstHalf, shapeOverUnder},
	"SPORTS_SECOND_HALF_TOTALS": {canon.CodeTotal, canon.PeriodSecondHalf, shapeOverUnder},
}

var gridironOnlySpecs = map[string]marketSpec{
	"SPORTS_ANYTIME_TOUCHDOWNS": {canon.PropAnytimeTD, canon.PeriodFullTime, shapePlayerYesNo},
}

var tennisSpecs = map[string]marketSpec{
	"SPORTS_MONEYLINE": {canon.CodeMoneyline, canon.PeriodFullTime, shapeTeamBinary},
}

var esportsSpecs = map[string]marketSpec{
	"SPORTS_MONEYLINE":       {canon.CodeMoneyline, canon.PeriodFullTime, shapeTeamBinary},
	"SPORTS_CHILD_MONEYLINE": {canon.CodeMoneyline, canon.PeriodFirstMap, shapeFirstMapWinner},
	"SPORTS_TOTALS":          {canon.CodeTotalMaps, canon.PeriodFullTime, shapeOverUnder},
	"SPORTS_MAP_HANDICAP":    {canon.CodeMapHcp, canon.PeriodFullTime, shapeHandicap},
}

var marketSpecsBySport = map[canon.Sport]map[string]marketSpec{
	canon.SportFootball:         footballSpecs,
	canon.SportBasketball:       pointsSportSpecs,
	canon.SportAmericanFootball: mergedSpecs(pointsSportSpecs, gridironOnlySpecs),
	canon.SportTennis:           tennisSpecs,
	canon.SportEsports:          esportsSpecs,
}

func mergedSpecs(base, extra map[string]marketSpec) map[string]marketSpec {
	merged := maps.Clone(base)
	maps.Copy(merged, extra)
	return merged
}

var supportedMarkets = func() []canon.MarketCode {
	supplied := map[canon.MarketCode]bool{}
	for _, specs := range marketSpecsBySport {
		for _, spec := range specs {
			supplied[spec.code] = true
		}
	}
	return canon.InCanonicalOrder(supplied)
}()

var footballCategorySuffixes = []string{"", "-halftime-result", "-exact-score", "-total-corners", "-more-markets", "-player-props"}

var headToHeadCategorySuffixes = []string{"", "-more-markets", "-player-props"}

func categorySuffixesFor(sport canon.Sport) []string {
	if sport == canon.SportFootball {
		return footballCategorySuffixes
	}
	return headToHeadCategorySuffixes
}

const (
	openTradingStatus = "OPEN"
	drawTitle         = "Draw"
	anyOtherScore     = "Any Other Score"
	otherScoreLabel   = "Other"
	underPrefix       = "under"
	yesNoPlayerLine   = 0.5
	binaryOutcomes    = 2
	firstOutcome      = 0
	firstMap          = 1
	firstSlugTeamPart = 1
	minMatchSlugParts = 6
)

var (
	exactScoreInTitle    = regexp.MustCompile(`(\d+) - (\d+)`)
	handicapInTitle      = regexp.MustCompile(`\(([+-]?\d+(?:\.\d+)?)\)`)
	totalInTitle         = regexp.MustCompile(`O/U (\d+(?:\.\d+)?)`)
	gameNumberInTitle    = regexp.MustCompile(`(?:Map|Game) (\d+) Winner`)
	lineParsersByShape   = map[marketShape]*regexp.Regexp{shapeHandicap: handicapInTitle, shapeOverUnder: totalInTitle, shapeTeamOverUnder: totalInTitle}
	lineRequiredByShapes = map[marketShape]bool{shapeHandicap: true, shapeOverUnder: true, shapePlayerOverUnder: true, shapeTeamOverUnder: true}
)

const teamTotalCut = " O/U "

func teamTotalName(title string) (string, bool) {
	name, _, found := strings.Cut(title, teamTotalCut)
	name = strings.TrimSpace(name)
	return name, found && name != ""
}

func exactScoreLabel(title string) (string, bool) {
	if title == anyOtherScore {
		return otherScoreLabel, true
	}
	m := exactScoreInTitle.FindStringSubmatch(title)
	if m == nil {
		return "", false
	}
	return m[1] + "-" + m[2], true
}

func lineOf(m market, shape marketShape) (float64, bool) {
	if line := m.VariantDetails.Sports.Line; line != nil {
		return *line, true
	}
	parser, parsable := lineParsersByShape[shape]
	if !parsable {
		return 0, false
	}
	match := parser.FindStringSubmatch(m.Title)
	if match == nil {
		return 0, false
	}
	line, err := strconv.ParseFloat(match[1], 64)
	return line, err == nil
}

func gameNumberOf(m market) int {
	if n := m.VariantDetails.Sports.GameNumber; n != nil {
		return *n
	}
	match := gameNumberInTitle.FindStringSubmatch(m.Title)
	if match == nil {
		return 0
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return n
}

func listsUnderFirst(m market) bool {
	return len(m.Outcomes) == binaryOutcomes && strings.HasPrefix(strings.ToLower(m.Outcomes[firstOutcome].Name), underPrefix)
}

func playerOf(m market) (*canon.SourcePlayer, bool) {
	name, _, found := strings.Cut(m.Question, ":")
	name = strings.TrimSpace(name)
	if !found || name == "" {
		return nil, false
	}
	return &canon.SourcePlayer{SourceID: name, Name: name}, true
}
