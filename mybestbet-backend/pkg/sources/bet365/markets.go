package bet365

import "mybestbet/pkg/canon"

type marketShape int

const (
	shapeOutcomes marketShape = iota + 1
	shapeTwoWay
	shapeDoubleChance
	shapeYesNo
	shapeHalfTimeFullTime
	shapeTotals
	shapeAsianHandicap
	shapeCorrectScore
	shapeScorers
	shapePlayerThresholds
	shapePlayerYes
	shapePlayerOverUnder
	shapeGameLines
)

type marketSpec struct {
	code   canon.MarketCode
	shape  marketShape
	period string
}

var soccerMarkets = map[string]marketSpec{
	"full time result":           {code: canon.Code1X2, shape: shapeOutcomes},
	"double chance":              {code: canon.CodeDC, shape: shapeDoubleChance},
	"goals over/under":           {code: canon.CodeOU, shape: shapeTotals},
	"alternative total goals":    {code: canon.CodeOU, shape: shapeTotals},
	"both teams to score":        {code: canon.CodeBTTS, shape: shapeYesNo},
	"draw no bet":                {code: canon.CodeDNB, shape: shapeOutcomes},
	"asian handicap":             {code: canon.CodeAH, shape: shapeAsianHandicap},
	"alternative asian handicap": {code: canon.CodeAH, shape: shapeAsianHandicap},
	"correct score":              {code: canon.CodeCS, shape: shapeCorrectScore},
	"half time/full time":        {code: canon.CodeHTFT, shape: shapeHalfTimeFullTime},
	"half time result":           {code: canon.Code1H1X2, shape: shapeOutcomes, period: canon.PeriodFirstHalf},
	"corners 2-way":              {code: canon.CodeCornOU, shape: shapeTotals},
	"to qualify":                 {code: canon.CodeQualify, shape: shapeTwoWay},
	"goalscorers":                {shape: shapeScorers},
	"player shots":               {code: canon.PropShots, shape: shapePlayerThresholds},
	"player shots on target":     {code: canon.PropSOT, shape: shapePlayerThresholds},
	"player assists":             {code: canon.PropAssists, shape: shapePlayerThresholds},
	"player fouls committed":     {code: canon.PropFouls, shape: shapePlayerThresholds},
	"player tackles":             {code: canon.PropTackles, shape: shapePlayerThresholds},
	"player to be booked":        {code: canon.PropCard, shape: shapePlayerYes},
}

var usGameLineMarkets = map[string]marketSpec{
	"game lines":  {shape: shapeGameLines},
	"1st half":    {shape: shapeGameLines, period: canon.PeriodFirstHalf},
	"2nd half":    {shape: shapeGameLines, period: canon.PeriodSecondHalf},
	"1st quarter": {shape: shapeGameLines, period: canon.PeriodQuarter1},
	"2nd quarter": {shape: shapeGameLines, period: canon.PeriodQuarter2},
	"3rd quarter": {shape: shapeGameLines, period: canon.PeriodQuarter3},
	"4th quarter": {shape: shapeGameLines, period: canon.PeriodQuarter4},
}

var basketballMarkets = withGameLines(map[string]marketSpec{
	"player points":                     {code: canon.PropPoints, shape: shapePlayerOverUnder},
	"player rebounds":                   {code: canon.PropRebounds, shape: shapePlayerOverUnder},
	"player assists":                    {code: canon.PropBkAssists, shape: shapePlayerOverUnder},
	"player threes made":                {code: canon.PropThrees, shape: shapePlayerOverUnder},
	"player points, rebounds & assists": {code: canon.PropPtsRebAst, shape: shapePlayerOverUnder},
})

var americanFootballMarkets = withGameLines(map[string]marketSpec{
	"player passing yards":      {code: canon.PropPassYards, shape: shapePlayerOverUnder},
	"player rushing yards":      {code: canon.PropRushYards, shape: shapePlayerOverUnder},
	"player receiving yards":    {code: canon.PropRecYards, shape: shapePlayerOverUnder},
	"player receptions":         {code: canon.PropReceptions, shape: shapePlayerOverUnder},
	"player passing touchdowns": {code: canon.PropPassTDs, shape: shapePlayerOverUnder},
	"anytime touchdown scorer":  {code: canon.PropAnytimeTD, shape: shapePlayerYes},
})

var tennisMarkets = map[string]marketSpec{
	"to win match":         {code: canon.CodeMoneyline, shape: shapeTwoWay},
	"first set winner":     {code: canon.CodeMoneyline, shape: shapeTwoWay, period: canon.PeriodFirstSet},
	"set handicap":         {code: canon.CodeSetHcp, shape: shapeAsianHandicap},
	"total games 2-way":    {code: canon.CodeTotalGames, shape: shapeTotals},
	"player aces":          {code: canon.PropAces, shape: shapePlayerOverUnder},
	"player double faults": {code: canon.PropDoubleFlt, shape: shapePlayerOverUnder},
}

var esportsMarkets = map[string]marketSpec{
	"match winner":         {code: canon.CodeMoneyline, shape: shapeTwoWay},
	"map handicap":         {code: canon.CodeMapHcp, shape: shapeAsianHandicap},
	"total maps":           {code: canon.CodeTotalMaps, shape: shapeTotals},
	"map 1 - winner":       {code: canon.CodeMoneyline, shape: shapeTwoWay, period: canon.PeriodFirstMap},
	"map 1 - player kills": {code: canon.PropMapKills, shape: shapePlayerOverUnder, period: canon.PeriodFirstMap},
}

var marketsBySport = map[canon.Sport]map[string]marketSpec{
	canon.SportFootball:         soccerMarkets,
	canon.SportBasketball:       basketballMarkets,
	canon.SportAmericanFootball: americanFootballMarkets,
	canon.SportTennis:           tennisMarkets,
	canon.SportEsports:          esportsMarkets,
}

func withGameLines(props map[string]marketSpec) map[string]marketSpec {
	for name, spec := range usGameLineMarkets {
		props[name] = spec
	}
	return props
}

type gameLineColumn struct {
	code   canon.MarketCode
	isLine bool
}

var gameLineColumns = map[string]gameLineColumn{
	"spread":     {code: canon.CodeSpread, isLine: true},
	"handicap":   {code: canon.CodeSpread, isLine: true},
	"total":      {code: canon.CodeTotal, isLine: true},
	"money":      {code: canon.CodeMoneyline},
	"money line": {code: canon.CodeMoneyline},
}

var scorerCodesByColumn = map[string]canon.MarketCode{
	"first":   canon.PropFirst,
	"last":    canon.PropLast,
	"anytime": canon.PropAnytime,
}

var suppliedMarkets = []canon.MarketCode{
	canon.Code1X2, canon.CodeDC, canon.CodeOU, canon.CodeBTTS, canon.CodeDNB, canon.CodeAH, canon.CodeCS,
	canon.CodeHTFT, canon.Code1H1X2, canon.CodeCornOU, canon.CodeQualify,
	canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeSetHcp, canon.CodeTotalGames,
	canon.CodeMapHcp, canon.CodeTotalMaps,
	canon.PropAnytime, canon.PropFirst, canon.PropLast, canon.PropShots, canon.PropSOT, canon.PropAssists,
	canon.PropFouls, canon.PropTackles, canon.PropCard,
	canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees, canon.PropPtsRebAst,
	canon.PropPassYards, canon.PropRushYards, canon.PropRecYards, canon.PropReceptions, canon.PropPassTDs,
	canon.PropAnytimeTD, canon.PropAces, canon.PropDoubleFlt, canon.PropMapKills,
}
