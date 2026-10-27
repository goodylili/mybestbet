package onexbet

import (
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

type gameScope int

const (
	fullMatch gameScope = iota + 1
	subPeriod
	corners
	playerStats
	aces
	doubleFaults
	secondHalf
)

type betShape int

const (
	fixedOutcomes betShape = iota + 1
	totalLine
	handicapLine
	correctScore
	playerYes
	playerOver
	competitorTotal
	teamTotal
)

type pick struct {
	side  canon.Side
	label string
}

type groupKey struct {
	sport canon.Sport
	scope gameScope
	group int
}

type groupRule struct {
	code            canon.MarketCode
	shape           betShape
	picks           map[int]pick
	lineWhenMissing float64
	competitorSide  int
}

const (
	correctScoreType      = 731
	otherCorrectScoreType = 3786
	otherScoreLabel       = "Other"
	levelHandicapGroup    = 2
)

var (
	homeDrawAway    = map[int]pick{1: {side: canon.SideHome}, 2: {side: canon.SideDraw}, 3: {side: canon.SideAway}}
	homeOrAway      = map[int]pick{1: {side: canon.SideHome}, 3: {side: canon.SideAway}}
	overUnder       = map[int]pick{9: {side: canon.SideOver}, 10: {side: canon.SideUnder}}
	handicapSides   = map[int]pick{7: {side: canon.SideHome}, 8: {side: canon.SideAway}}
	homeTotalSides  = map[int]pick{11: {side: canon.SideOver}, 12: {side: canon.SideUnder}}
	awayTotalSides  = map[int]pick{13: {side: canon.SideOver}, 14: {side: canon.SideUnder}}
	individualTotal = map[int]struct {
		side  int
		picks map[int]pick
	}{15: {homeTeamSide, homeTotalSides}, 62: {awayTeamSide, awayTotalSides}}
)

func overAndUnder(over, under int) map[int]pick {
	return map[int]pick{over: {side: canon.SideOver}, under: {side: canon.SideUnder}}
}

var (
	overUnderGoals      = groupRule{code: canon.CodeOU, shape: totalLine, picks: overUnder}
	asianOverUnderGoals = groupRule{code: canon.CodeOU, shape: totalLine, picks: overAndUnder(3827, 3828)}
	bothTeamsScore      = groupRule{code: canon.CodeBTTS, shape: fixedOutcomes, picks: map[int]pick{180: {side: canon.SideYes}, 181: {side: canon.SideNo}}}
)

func footballHalfRules(scopes ...gameScope) map[groupKey]groupRule {
	rules := map[groupKey]groupRule{}
	for _, scope := range scopes {
		rules[groupKey{canon.SportFootball, scope, 17}] = overUnderGoals
		rules[groupKey{canon.SportFootball, scope, 99}] = asianOverUnderGoals
		rules[groupKey{canon.SportFootball, scope, 19}] = bothTeamsScore
	}
	return rules
}

func footballTeamTotalRules(scopes ...gameScope) map[groupKey]groupRule {
	rules := map[groupKey]groupRule{}
	for _, scope := range scopes {
		for group, total := range individualTotal {
			rules[groupKey{canon.SportFootball, scope, group}] = groupRule{code: canon.TeamGoals, shape: teamTotal, picks: total.picks, competitorSide: total.side}
		}
	}
	return rules
}

var footballRules = map[groupKey]groupRule{
	{canon.SportFootball, fullMatch, 1}:       {code: canon.Code1X2, shape: fixedOutcomes, picks: homeDrawAway},
	{canon.SportFootball, fullMatch, 8}:       {code: canon.CodeDC, shape: fixedOutcomes, picks: map[int]pick{4: {label: "1X"}, 5: {label: "12"}, 6: {label: "X2"}}},
	{canon.SportFootball, fullMatch, 17}:      overUnderGoals,
	{canon.SportFootball, fullMatch, 99}:      asianOverUnderGoals,
	{canon.SportFootball, fullMatch, 19}:      bothTeamsScore,
	{canon.SportFootball, fullMatch, 2}:       {code: canon.CodeAH, shape: handicapLine, picks: handicapSides},
	{canon.SportFootball, fullMatch, 2854}:    {code: canon.CodeAH, shape: handicapLine, picks: map[int]pick{3829: {side: canon.SideHome}, 3830: {side: canon.SideAway}}},
	{canon.SportFootball, fullMatch, 136}:     {code: canon.CodeCS, shape: correctScore},
	{canon.SportFootball, fullMatch, 100}:     {code: canon.CodeQualify, shape: fixedOutcomes, picks: map[int]pick{794: {side: canon.SideHome}, 795: {side: canon.SideAway}}},
	{canon.SportFootball, subPeriod, 1}:       {code: canon.Code1H1X2, shape: fixedOutcomes, picks: homeDrawAway},
	{canon.SportFootball, corners, 17}:        {code: canon.CodeCornOU, shape: totalLine, picks: overUnder},
	{canon.SportFootball, playerStats, 119}:   {code: canon.PropAnytime, shape: playerYes, picks: map[int]pick{5869: {side: canon.SideYes}, 5870: {side: canon.SideNo}}},
	{canon.SportFootball, playerStats, 117}:   {code: canon.PropFirst, shape: playerYes, picks: map[int]pick{833: {side: canon.SideYes}}},
	{canon.SportFootball, playerStats, 118}:   {code: canon.PropLast, shape: playerYes, picks: map[int]pick{834: {side: canon.SideYes}}},
	{canon.SportFootball, playerStats, 3357}:  {code: canon.PropShots, shape: playerOver, picks: map[int]pick{4650: {side: canon.SideOver}}},
	{canon.SportFootball, playerStats, 3217}:  {code: canon.PropSOT, shape: playerOver, picks: map[int]pick{4416: {side: canon.SideOver}}},
	{canon.SportFootball, playerStats, 9120}:  {code: canon.PropTackles, shape: playerOver, picks: overAndUnder(9290, 9291)},
	{canon.SportFootball, playerStats, 11143}: {code: canon.PropAssists, shape: playerOver, picks: map[int]pick{15621: {side: canon.SideOver}}, lineWhenMissing: 0.5},
}

func pointsSportRules(sport canon.Sport, props map[int]groupRule) map[groupKey]groupRule {
	rules := map[groupKey]groupRule{
		{sport, fullMatch, 101}: {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: map[int]pick{401: {side: canon.SideHome}, 402: {side: canon.SideAway}}},
		{sport, fullMatch, 2}:   {code: canon.CodeSpread, shape: handicapLine, picks: handicapSides},
		{sport, fullMatch, 17}:  {code: canon.CodeTotal, shape: totalLine, picks: overUnder},
		{sport, subPeriod, 2}:   {code: canon.CodeSpread, shape: handicapLine, picks: handicapSides},
		{sport, subPeriod, 17}:  {code: canon.CodeTotal, shape: totalLine, picks: overUnder},
	}
	for group, rule := range props {
		rules[groupKey{sport, playerStats, group}] = rule
	}
	return rules
}

var basketballRules = pointsSportRules(canon.SportBasketball, map[int]groupRule{
	850:  {code: canon.PropPoints, shape: playerOver, picks: overAndUnder(1807, 1806)},
	749:  {code: canon.PropRebounds, shape: playerOver, picks: overAndUnder(1489, 1490)},
	751:  {code: canon.PropBkAssists, shape: playerOver, picks: overAndUnder(1491, 1492)},
	755:  {code: canon.PropThrees, shape: playerOver, picks: overAndUnder(1495, 1496)},
	7039: {code: canon.PropPtsRebAst, shape: playerOver, picks: overAndUnder(5675, 5676)},
})

var americanFootballRules = pointsSportRules(canon.SportAmericanFootball, map[int]groupRule{
	8707: {code: canon.PropPassYards, shape: playerOver, picks: overAndUnder(8397, 8398)},
	7525: {code: canon.PropPassTDs, shape: playerOver, picks: overAndUnder(6376, 6377)},
	8711: {code: canon.PropRushYards, shape: playerOver, picks: overAndUnder(8393, 8394)},
	8709: {code: canon.PropRecYards, shape: playerOver, picks: overAndUnder(8395, 8396)},
	8713: {code: canon.PropReceptions, shape: playerOver, picks: overAndUnder(8399, 8400)},
	7407: {code: canon.PropAnytimeTD, shape: playerYes, picks: map[int]pick{6229: {side: canon.SideYes}}},
})

var tennisRules = withCompetitorTotals(map[groupKey]groupRule{
	{canon.SportTennis, fullMatch, 1}:   {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: homeOrAway},
	{canon.SportTennis, fullMatch, 109}: {code: canon.CodeSetHcp, shape: handicapLine, picks: map[int]pick{732: {side: canon.SideHome}, 733: {side: canon.SideAway}}},
	{canon.SportTennis, fullMatch, 17}:  {code: canon.CodeTotalGames, shape: totalLine, picks: overUnder},
	{canon.SportTennis, subPeriod, 1}:   {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: homeOrAway},
	{canon.SportTennis, subPeriod, 17}:  {code: canon.CodeTotalGames, shape: totalLine, picks: overUnder},
}, map[gameScope]canon.MarketCode{aces: canon.PropAces, doubleFaults: canon.PropDoubleFlt})

func withCompetitorTotals(rules map[groupKey]groupRule, codeByScope map[gameScope]canon.MarketCode) map[groupKey]groupRule {
	for scope, code := range codeByScope {
		for group, total := range individualTotal {
			rules[groupKey{canon.SportTennis, scope, group}] = groupRule{code: code, shape: competitorTotal, picks: total.picks, competitorSide: total.side}
		}
	}
	return rules
}

var esportsRules = map[groupKey]groupRule{
	{canon.SportEsports, fullMatch, 1}:      {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: homeOrAway},
	{canon.SportEsports, fullMatch, 2438}:   {code: canon.CodeMapHcp, shape: handicapLine, picks: map[int]pick{2826: {side: canon.SideHome}, 2827: {side: canon.SideAway}}},
	{canon.SportEsports, fullMatch, 2436}:   {code: canon.CodeTotalMaps, shape: totalLine, picks: overAndUnder(2824, 2825)},
	{canon.SportEsports, subPeriod, 1}:      {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: homeOrAway},
	{canon.SportEsports, playerStats, 9562}: {code: canon.PropMapKills, shape: playerOver, picks: overAndUnder(10816, 10817)},
}

var groupRules = mergedRules(
	footballRules,
	footballHalfRules(subPeriod, secondHalf),
	footballTeamTotalRules(fullMatch, subPeriod, secondHalf),
	basketballRules,
	americanFootballRules,
	tennisRules,
	esportsRules,
)

func mergedRules(tables ...map[groupKey]groupRule) map[groupKey]groupRule {
	out := map[groupKey]groupRule{}
	for _, table := range tables {
		maps.Copy(out, table)
	}
	return out
}

func suppliedCodes() []canon.MarketCode {
	mapped := map[canon.MarketCode]bool{canon.CodeDNB: true, canon.CodeMoneyline: true}
	for _, rule := range groupRules {
		mapped[rule.code] = true
	}
	return canon.InCanonicalOrder(mapped)
}

type placedBet struct {
	line      *float64
	player    *canon.SourcePlayer
	team      string
	selection canon.Selection
}

func (r groupRule) place(b bet, g game, observedAt time.Time) (placedBet, bool) {
	price := canon.NewPrice(b.Odds, canon.FormDecimal, observedAt)
	if r.shape == correctScore {
		label, ok := correctScoreLabel(b)
		return placedBet{selection: canon.Selection{Label: label, Price: price}}, ok
	}
	p, ok := r.picks[b.Type]
	if !ok {
		return placedBet{}, false
	}
	placed := placedBet{selection: canon.Selection{Side: p.side, Label: p.label, Price: price}}
	switch r.shape {
	case totalLine:
		placed.line = b.Param
		return placed, b.Param != nil
	case handicapLine:
		placed.line = canon.Line(homeHandicap(p.side, b.Param))
	case playerYes, playerOver:
		if b.Player == nil {
			return placedBet{}, false
		}
		placed.player = sourcePlayer(*b.Player, g)
		if r.shape == playerOver {
			placed.line = r.overLine(b.Param)
		}
		return placed, placed.player != nil
	case competitorTotal:
		placed.player = g.competitor(r.competitorSide)
		placed.line = b.Param
		return placed, b.Param != nil && placed.player != nil
	case teamTotal:
		placed.team = g.teamName(r.competitorSide)
		placed.line = b.Param
		return placed, b.Param != nil && placed.team != ""
	case fixedOutcomes, correctScore:
	}
	return placed, true
}

func (r groupRule) overLine(param *float64) *float64 {
	if param != nil {
		return param
	}
	return canon.Line(r.lineWhenMissing)
}

func homeHandicap(side canon.Side, param *float64) float64 {
	if param == nil || *param == 0 {
		return 0
	}
	if side == canon.SideAway {
		return -*param
	}
	return *param
}

func correctScoreLabel(b bet) (string, bool) {
	switch b.Type {
	case otherCorrectScoreType:
		return otherScoreLabel, true
	case correctScoreType:
		encoded := 0.0
		if b.Param != nil {
			encoded = *b.Param
		}
		home := math.Floor(encoded)
		away := math.Round((encoded - home) * 1000)
		return fmt.Sprintf("%d-%d", int(home), int(away)), true
	}
	return "", false
}

func sourcePlayer(pl linePlayer, g game) *canon.SourcePlayer {
	if pl.ID == 0 || pl.Name == "" {
		return nil
	}
	return &canon.SourcePlayer{SourceID: strconv.FormatInt(pl.ID, 10), Name: pl.Name, Team: g.teamName(pl.TeamSide)}
}

type accumulatedMarket struct {
	group int
	raw   canon.RawMarket
}

type marketAccumulator struct {
	order []string
	byID  map[string]*accumulatedMarket
}

func newMarketAccumulator() *marketAccumulator {
	return &marketAccumulator{byID: map[string]*accumulatedMarket{}}
}

func (acc *marketAccumulator) add(eventID, period string, g game, groupNumber int, rule groupRule, placed placedBet) {
	sourceID := marketSourceID(g.ID, groupNumber, placed)
	entry, ok := acc.byID[sourceID]
	if !ok {
		entry = &accumulatedMarket{group: groupNumber, raw: canon.RawMarket{
			EventSourceID:  eventID,
			Code:           rule.code,
			Line:           placed.line,
			Player:         placed.player,
			Team:           placed.team,
			Period:         period,
			Rules:          rule.code.DefaultRule(),
			SourceMarketID: sourceID,
		}}
		acc.byID[sourceID] = entry
		acc.order = append(acc.order, sourceID)
	}
	entry.raw.Selections = append(entry.raw.Selections, placed.selection)
}

func marketSourceID(gameID int64, groupNumber int, placed placedBet) string {
	id := fmt.Sprintf("%d:%d", gameID, groupNumber)
	if placed.line != nil {
		id += "@" + strconv.FormatFloat(*placed.line, 'f', -1, 64)
	}
	if placed.player != nil {
		id += "#" + placed.player.SourceID
	}
	return id
}

func (acc *marketAccumulator) rawMarkets() []canon.RawMarket {
	out := make([]canon.RawMarket, 0, len(acc.order)+1)
	for _, id := range acc.order {
		entry := acc.byID[id]
		out = append(out, entry.raw)
		if code, ok := entry.levelHandicapEquivalent(); ok {
			out = append(out, levelHandicapCopy(entry.raw, code))
		}
	}
	return out
}

func (m accumulatedMarket) levelHandicapEquivalent() (canon.MarketCode, bool) {
	if m.group != levelHandicapGroup || m.raw.Line == nil || *m.raw.Line != 0 {
		return "", false
	}
	switch {
	case m.raw.Code == canon.CodeAH:
		return canon.CodeDNB, true
	case m.raw.Code == canon.CodeSpread && m.raw.Period != "":
		return canon.CodeMoneyline, true
	}
	return "", false
}

func levelHandicapCopy(levelHandicap canon.RawMarket, code canon.MarketCode) canon.RawMarket {
	copied := levelHandicap.Clone()
	copied.Code = code
	copied.Line = nil
	copied.Rules = code.DefaultRule()
	copied.SourceMarketID += "/" + strings.ToLower(string(code))
	return copied
}
