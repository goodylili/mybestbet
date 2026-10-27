package bayse

import (
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
	shapeBuckets
	shapePlayerOverUnder
	shapePlayerAnytime
	shapePlayerRating
)

type marketSpec struct {
	code  canon.MarketCode
	shape marketShape
	rules canon.SettleRule
}

var (
	optaPlayerRule      = canon.SettleRule{RegulationOnly: true, VoidIfNoStart: true, Resolver: "opta"}
	sofascoreRatingRule = canon.SettleRule{RegulationOnly: true, VoidIfNoStart: true, Resolver: "sofascore"}
	kalshiPlayerRule    = canon.SettleRule{VoidIfNoStart: true}
)

var sportMarketSpecs = map[string]marketSpec{
	"TEAM_H2H_3WAY":           {canon.Code1X2, shapeOutcomeGroup, canon.StandardRule},
	"BOTH_TEAMS_TO_SCORE":     {canon.CodeBTTS, shapeYesNo, canon.StandardRule},
	"GOAL_SPREAD":             {canon.CodeAH, shapeHandicap, canon.StandardRule},
	"TOTAL_GOALS":             {canon.CodeOU, shapeOverUnder, canon.StandardRule},
	"TOTAL_CORNERS":           {canon.CodeCornOU, shapeOverUnder, canon.StandardRule},
	"TOTAL_CARDS":             {canon.TeamCards, shapeOverUnder, canon.StandardRule},
	"PLAYER_GOALSCORER":       {canon.PropAnytime, shapePlayerAnytime, optaPlayerRule},
	"PLAYER_GOALS":            {canon.PropGoals, shapePlayerOverUnder, optaPlayerRule},
	"PLAYER_SHOTS":            {canon.PropShots, shapePlayerOverUnder, optaPlayerRule},
	"PLAYER_ASSISTS":          {canon.PropAssists, shapePlayerOverUnder, optaPlayerRule},
	"PLAYER_TACKLES":          {canon.PropTackles, shapePlayerOverUnder, optaPlayerRule},
	"PLAYER_PASSING_ATTEMPTS": {canon.PropPasses, shapePlayerOverUnder, optaPlayerRule},
	"PLAYER_SAVES":            {canon.PropSaves, shapePlayerOverUnder, optaPlayerRule},
	"TOTAL_GOALS_HOME":        {canon.TeamGoals, shapeOverUnder, canon.StandardRule},
	"TOTAL_GOALS_AWAY":        {canon.TeamGoals, shapeOverUnder, canon.StandardRule},
}

var listedMarketSpecs = map[string]map[string]marketSpec{
	"NBA": {
		gamesHashtag: {canon.CodeMoneyline, shapeOutcomeGroup, canon.CodeMoneyline.DefaultRule()},
		"Points":     {canon.PropPoints, shapePlayerOverUnder, kalshiPlayerRule},
		"Rebounds":   {canon.PropRebounds, shapePlayerOverUnder, kalshiPlayerRule},
		"Assists":    {canon.PropBkAssists, shapePlayerOverUnder, kalshiPlayerRule},
		"Threes":     {canon.PropThrees, shapePlayerOverUnder, kalshiPlayerRule},
	},
}

type titledSpec struct {
	title *regexp.Regexp
	spec  marketSpec
}

var untypedMarketSpecs = []titledSpec{
	{regexp.MustCompile(`(?i)sofascore rating`), marketSpec{canon.PropRating, shapePlayerRating, sofascoreRatingRule}},
	{regexp.MustCompile(`(?i)how many passes`), marketSpec{canon.TeamPasses, shapeBuckets, canon.StandardRule}},
	{regexp.MustCompile(`(?i)total shots`), marketSpec{canon.TeamShots, shapeBuckets, canon.StandardRule}},
	{regexp.MustCompile(`(?i)total cards`), marketSpec{canon.TeamCards, shapeBuckets, canon.StandardRule}},
}

var supportedMarkets = []canon.MarketCode{
	canon.Code1X2, canon.CodeBTTS, canon.CodeAH, canon.CodeOU, canon.CodeCornOU,
	canon.PropAnytime, canon.PropGoals, canon.PropShots, canon.PropAssists, canon.PropTackles, canon.PropPasses,
	canon.PropSaves, canon.PropRating,
	canon.TeamGoals, canon.TeamShots, canon.TeamPasses, canon.TeamCards,
	canon.CodeMoneyline, canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees,
}

func specFor(ev event) (marketSpec, bool) {
	if ev.SportMarketType != "" {
		spec, ok := sportMarketSpecs[ev.SportMarketType]
		return spec, ok
	}
	if specs, isListed := listedSpecsOf(ev); isListed {
		return listedSpecFor(ev, specs)
	}
	for _, t := range untypedMarketSpecs {
		if t.title.MatchString(ev.Title) {
			return t.spec, true
		}
	}
	return marketSpec{}, false
}

func listedSpecsOf(ev event) (map[string]marketSpec, bool) {
	for league, specs := range listedMarketSpecs {
		if hasHashtag(ev, league) {
			return specs, true
		}
	}
	return nil, false
}

func listedSpecFor(ev event, specs map[string]marketSpec) (marketSpec, bool) {
	for _, tag := range ev.Hashtags {
		if spec, ok := specs[tag]; ok {
			return spec, true
		}
	}
	return marketSpec{}, false
}

const (
	drawTitle       = "Draw"
	anytimeLine     = 0.5
	wholeToHalfLine = 0.5
)

var (
	gameSlugCodes      = regexp.MustCompile(`^bm-game-\d{8}-([a-z0-9]+)-([a-z0-9]+)$`)
	countPlusInTitle   = regexp.MustCompile(`(\d+)\+`)
	overUnderInTitle   = regexp.MustCompile(`O/U (\d+(?:\.\d+)?)`)
	ratingTitle        = regexp.MustCompile(`(?i)^(.+?)\s+(\d+(?:\.\d+)?)\+\s+sofascore rating`)
	bucketBelow        = regexp.MustCompile(`(?i)^less than (\d+)$`)
	bucketBetween      = regexp.MustCompile(`^(\d+)\s*-\s*(\d+)$`)
	bucketAtLeast      = regexp.MustCompile(`(?i)^(\d+)(?:\+| or more)$`)
	passesTeamInTitle  = regexp.MustCompile(`(?i)how many passes will (.+?) get vs`)
	goalsTeamInTitle   = regexp.MustCompile(`(?i)^(.+?):\s*total goals$`)
	winningMarginRules = regexp.MustCompile(`^If (.+?) wins by more than (\d+(?:\.\d+)?) goals in the (.+?) vs (.+?) professional`)
)

func overLineOf(m market) (*float64, bool) {
	if m.PropLine != nil {
		return canon.Line(*m.PropLine), true
	}
	if g := overUnderInTitle.FindStringSubmatch(m.Title); g != nil {
		v, err := strconv.ParseFloat(g[1], 64)
		return canon.Line(v), err == nil
	}
	if g := countPlusInTitle.FindStringSubmatch(m.Title); g != nil {
		v, err := strconv.ParseFloat(g[1], 64)
		return canon.Line(v - wholeToHalfLine), err == nil
	}
	return nil, false
}

func bucketLabel(title string) (string, bool) {
	title = strings.TrimSpace(title)
	if g := bucketBelow.FindStringSubmatch(title); g != nil {
		return "<" + g[1], true
	}
	if g := bucketBetween.FindStringSubmatch(title); g != nil {
		return g[1] + "-" + g[2], true
	}
	if g := bucketAtLeast.FindStringSubmatch(title); g != nil {
		return g[1] + "+", true
	}
	return "", false
}

func statPlayer(ev event, m market) (*canon.SourcePlayer, bool) {
	source := ev.Title
	if strings.Contains(m.Title, ":") {
		source = m.Title
	}
	name, _, found := strings.Cut(source, ":")
	return namedPlayer(name, found)
}

func ratingPlayer(ev event) (*canon.SourcePlayer, *float64, bool) {
	g := ratingTitle.FindStringSubmatch(strings.TrimSpace(ev.Title))
	if g == nil {
		return nil, nil, false
	}
	threshold, err := strconv.ParseFloat(g[2], 64)
	player, ok := namedPlayer(g[1], err == nil)
	return player, canon.Line(threshold), ok
}

func namedPlayer(name string, found bool) (*canon.SourcePlayer, bool) {
	name = strings.TrimSpace(name)
	if !found || name == "" {
		return nil, false
	}
	return &canon.SourcePlayer{SourceID: name, Name: name}, true
}

type winningMargin struct {
	side   canon.Side
	margin float64
}

func winningMarginOf(rules string) (winningMargin, bool) {
	g := winningMarginRules.FindStringSubmatch(strings.TrimSpace(rules))
	if g == nil {
		return winningMargin{}, false
	}
	margin, err := strconv.ParseFloat(g[2], 64)
	if err != nil {
		return winningMargin{}, false
	}
	switch winner := g[1]; {
	case strings.EqualFold(winner, g[3]):
		return winningMargin{side: canon.SideHome, margin: margin}, true
	case strings.EqualFold(winner, g[4]):
		return winningMargin{side: canon.SideAway, margin: margin}, true
	}
	return winningMargin{}, false
}

var teamInTitle = map[canon.MarketCode]*regexp.Regexp{
	canon.TeamPasses: passesTeamInTitle,
	canon.TeamGoals:  goalsTeamInTitle,
}

func teamOfTeamStat(code canon.MarketCode, title string) string {
	pattern, ok := teamInTitle[code]
	if !ok {
		return ""
	}
	if g := pattern.FindStringSubmatch(strings.TrimSpace(title)); g != nil {
		return strings.TrimSpace(g[1])
	}
	return ""
}

func isMissingTeam(code canon.MarketCode, team string) bool {
	_, needsTeam := teamInTitle[code]
	return needsTeam && team == ""
}
