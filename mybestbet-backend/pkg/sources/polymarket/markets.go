package polymarket

import (
	"regexp"
	"strings"

	"mybestbet/pkg/canon"
)

type marketShape int

const (
	shapeOutcomeGroup marketShape = iota + 1
	shapeOverUnder
	shapeYesNo
	shapeHandicap
	shapeTeamTwoWay
	shapePlayerOverUnder
	shapePlayerYesNo
	shapeTeamOverUnder
)

type marketSpec struct {
	code               canon.MarketCode
	period             string
	shape              marketShape
	requiredSlugSuffix string
}

type marketTable map[string]marketSpec

var footballMarkets = marketTable{
	"moneyline":                        {code: canon.Code1X2, shape: shapeOutcomeGroup},
	"soccer_halftime_result":           {code: canon.Code1H1X2, period: canon.PeriodFirstHalf, shape: shapeOutcomeGroup},
	"soccer_exact_score":               {code: canon.CodeCS, shape: shapeOutcomeGroup},
	"totals":                           {code: canon.CodeOU, shape: shapeOverUnder},
	"first_half_totals":                {code: canon.CodeOU, period: canon.PeriodFirstHalf, shape: shapeOverUnder},
	"second_half_totals":               {code: canon.CodeOU, period: canon.PeriodSecondHalf, shape: shapeOverUnder},
	"total_corners":                    {code: canon.CodeCornOU, shape: shapeOverUnder},
	"soccer_first_half_total_corners":  {code: canon.CodeCornOU, period: canon.PeriodFirstHalf, shape: shapeOverUnder},
	"soccer_second_half_total_corners": {code: canon.CodeCornOU, period: canon.PeriodSecondHalf, shape: shapeOverUnder},
	"both_teams_to_score":              {code: canon.CodeBTTS, shape: shapeYesNo},
	"both_teams_to_score_first_half":   {code: canon.CodeBTTS, period: canon.PeriodFirstHalf, shape: shapeYesNo},
	"both_teams_to_score_second_half":  {code: canon.CodeBTTS, period: canon.PeriodSecondHalf, shape: shapeYesNo},
	"spreads":                          {code: canon.CodeAH, shape: shapeHandicap},
	"soccer_anytime_goalscorer":        {code: canon.PropAnytime, shape: shapePlayerYesNo},
	"soccer_player_goals_plus_assists": {code: canon.PropGA, shape: shapePlayerYesNo},
	"soccer_player_goals":              {code: canon.PropGoals, shape: shapePlayerOverUnder},
	"soccer_player_shots":              {code: canon.PropShots, shape: shapePlayerOverUnder},
	"soccer_player_shots_on_target":    {code: canon.PropSOT, shape: shapePlayerOverUnder},
	"soccer_player_assists":            {code: canon.PropAssists, shape: shapePlayerOverUnder},
	"soccer_player_goalkeeper_saves":   {code: canon.PropSaves, shape: shapePlayerOverUnder},
	"soccer_team_totals":               {code: canon.TeamGoals, shape: shapeTeamOverUnder},
	"soccer_first_half_team_totals":    {code: canon.TeamGoals, period: canon.PeriodFirstHalf, shape: shapeTeamOverUnder},
	"soccer_second_half_team_totals":   {code: canon.TeamGoals, period: canon.PeriodSecondHalf, shape: shapeTeamOverUnder},
}

var periodMarketPrefixes = map[string]string{
	"":             canon.PeriodFullTime,
	"first_half_":  canon.PeriodFirstHalf,
	"second_half_": canon.PeriodSecondHalf,
	"q1_":          canon.PeriodQuarter1,
	"q2_":          canon.PeriodQuarter2,
	"q3_":          canon.PeriodQuarter3,
	"q4_":          canon.PeriodQuarter4,
}

func pointsGameMarkets(props marketTable) marketTable {
	table := marketTable{}
	for prefix, period := range periodMarketPrefixes {
		table[prefix+"moneyline"] = marketSpec{code: canon.CodeMoneyline, period: period, shape: shapeTeamTwoWay}
		table[prefix+"spreads"] = marketSpec{code: canon.CodeSpread, period: period, shape: shapeHandicap}
		table[prefix+"totals"] = marketSpec{code: canon.CodeTotal, period: period, shape: shapeOverUnder}
	}
	for marketType, spec := range props {
		table[marketType] = spec
	}
	return table
}

var basketballMarkets = pointsGameMarkets(marketTable{
	"points":   {code: canon.PropPoints, shape: shapePlayerOverUnder},
	"rebounds": {code: canon.PropRebounds, shape: shapePlayerOverUnder},
	"assists":  {code: canon.PropBkAssists, shape: shapePlayerOverUnder},
	"threes":   {code: canon.PropThrees, shape: shapePlayerOverUnder},
})

var gridironMarkets = pointsGameMarkets(marketTable{
	"passing_yards":      {code: canon.PropPassYards, shape: shapePlayerOverUnder},
	"rushing_yards":      {code: canon.PropRushYards, shape: shapePlayerOverUnder},
	"receiving_yards":    {code: canon.PropRecYards, shape: shapePlayerOverUnder},
	"receptions":         {code: canon.PropReceptions, shape: shapePlayerOverUnder},
	"passing_touchdowns": {code: canon.PropPassTDs, shape: shapePlayerOverUnder},
	"anytime_touchdowns": {code: canon.PropAnytimeTD, shape: shapePlayerYesNo},
})

var tennisMarkets = marketTable{
	"moneyline":               {code: canon.CodeMoneyline, shape: shapeTeamTwoWay},
	"tennis_first_set_winner": {code: canon.CodeMoneyline, period: canon.PeriodFirstSet, shape: shapeTeamTwoWay},
	"tennis_set_handicap":     {code: canon.CodeSetHcp, shape: shapeHandicap},
	"tennis_match_totals":     {code: canon.CodeTotalGames, shape: shapeOverUnder},
	"tennis_first_set_totals": {code: canon.CodeTotalGames, period: canon.PeriodFirstSet, shape: shapeOverUnder},
}

var esportsMarkets = marketTable{
	"moneyline":       {code: canon.CodeMoneyline, shape: shapeTeamTwoWay},
	"child_moneyline": {code: canon.CodeMoneyline, period: canon.PeriodFirstMap, shape: shapeTeamTwoWay, requiredSlugSuffix: "-game1"},
	"map_handicap":    {code: canon.CodeMapHcp, shape: shapeHandicap},
	"totals":          {code: canon.CodeTotalMaps, shape: shapeOverUnder},
}

var marketsBySport = map[canon.Sport]marketTable{
	canon.SportFootball:         footballMarkets,
	canon.SportBasketball:       basketballMarkets,
	canon.SportAmericanFootball: gridironMarkets,
	canon.SportTennis:           tennisMarkets,
	canon.SportEsports:          esportsMarkets,
}

func (t marketTable) specFor(m gammaMarket) (marketSpec, bool) {
	spec, known := t[m.SportsMarketType]
	if !known || !strings.HasSuffix(m.Slug, spec.requiredSlugSuffix) {
		return marketSpec{}, false
	}
	return spec, true
}

var supportedMarkets = func() []canon.MarketCode {
	supplied := map[canon.MarketCode]bool{}
	for _, table := range marketsBySport {
		for _, spec := range table {
			supplied[spec.code] = true
		}
	}
	return canon.InCanonicalOrder(supplied)
}()

var matchEventSuffixes = []string{"", "-halftime-result", "-exact-score", "-total-corners", "-more-markets", "-player-props"}

const (
	drawSuffix            = "draw"
	homeSuffix            = "home"
	awaySuffix            = "away"
	anyOtherScoreSuffix   = "any-other"
	otherScoreLabel       = "Other"
	yesNoPlayerLine       = 0.5
	firstOutcome          = 0
	tokensPerBinaryMarket = 2
	playerStatNameCut     = " Total "
)

var (
	exactScoreSuffix = regexp.MustCompile(`^\d+-\d+$`)
	playerIDInSlug   = regexp.MustCompile(`-(\d+)(?:-gte\d+)?$`)
	teamTotalInSlug  = regexp.MustCompile(`-team-total-(home|away)-`)
)

func teamTotalSuffix(slug string) (string, bool) {
	g := teamTotalInSlug.FindStringSubmatch(slug)
	if g == nil {
		return "", false
	}
	return g[1], true
}

func exactScoreLabel(suffix string) (string, bool) {
	if suffix == anyOtherScoreSuffix {
		return otherScoreLabel, true
	}
	return suffix, exactScoreSuffix.MatchString(suffix)
}

func playerOf(m gammaMarket) (*canon.SourcePlayer, bool) {
	prefix, _, found := strings.Cut(m.Question, ":")
	name, _, _ := strings.Cut(prefix, playerStatNameCut)
	name = strings.TrimSpace(name)
	if !found || name == "" {
		return nil, false
	}
	id := name
	if numeric := playerIDInSlug.FindStringSubmatch(m.Slug); numeric != nil {
		id = numeric[1]
	}
	return &canon.SourcePlayer{SourceID: id, Name: name}, true
}
