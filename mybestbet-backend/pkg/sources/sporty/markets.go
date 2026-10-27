package sporty

import (
	"maps"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

type selectionShape int

const (
	fixedOutcomes selectionShape = iota + 1
	correctScoreOutcomes
	playerYes
	playerOverLadder
	playerFirstRungYes
	playerOverUnder
)

type pick struct {
	side  canon.Side
	label string
}

type marketRule struct {
	code      canon.MarketCode
	period    string
	periodKey string
	periods   map[string]string
	lineKey   string
	shape     selectionShape
	picks     map[string]pick
	team      canon.Side
}

const (
	totalSpecifier    = "total"
	handicapSpecifier = "hcp"
	playerSpecifier   = "player"
	quarterSpecifier  = "quarternr"
	setSpecifier      = "setnr"
)

var (
	homeDrawAway  = map[string]pick{"1": {side: canon.SideHome}, "2": {side: canon.SideDraw}, "3": {side: canon.SideAway}}
	homeOrAway    = map[string]pick{"4": {side: canon.SideHome}, "5": {side: canon.SideAway}}
	overUnder     = map[string]pick{"12": {side: canon.SideOver}, "13": {side: canon.SideUnder}}
	handicapSides = map[string]pick{"1714": {side: canon.SideHome}, "1715": {side: canon.SideAway}}
	doubleChance  = map[string]pick{"9": {label: "1X"}, "10": {label: "12"}, "11": {label: "X2"}}
	yesNo         = map[string]pick{"74": {side: canon.SideYes}, "76": {side: canon.SideNo}}
	quarters      = map[string]string{"1": canon.PeriodQuarter1, "2": canon.PeriodQuarter2, "3": canon.PeriodQuarter3, "4": canon.PeriodQuarter4}
	firstSetOnly  = map[string]string{"1": canon.PeriodFirstSet}
)

var footballRules = map[string]marketRule{
	"1":      {code: canon.Code1X2, shape: fixedOutcomes, picks: homeDrawAway},
	"10":     {code: canon.CodeDC, shape: fixedOutcomes, picks: doubleChance},
	"11":     {code: canon.CodeDNB, shape: fixedOutcomes, picks: homeOrAway},
	"16":     {code: canon.CodeAH, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"18":     {code: canon.CodeOU, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"29":     {code: canon.CodeBTTS, shape: fixedOutcomes, picks: yesNo},
	"45":     {code: canon.CodeCS, shape: correctScoreOutcomes},
	"47":     {code: canon.CodeHTFT, shape: fixedOutcomes, picks: halfTimeFullTime},
	"60":     {code: canon.Code1H1X2, period: canon.PeriodFirstHalf, shape: fixedOutcomes, picks: homeDrawAway},
	"63":     {code: canon.CodeDC, period: canon.PeriodFirstHalf, shape: fixedOutcomes, picks: doubleChance},
	"85":     {code: canon.CodeDC, period: canon.PeriodSecondHalf, shape: fixedOutcomes, picks: doubleChance},
	"64":     {code: canon.CodeDNB, period: canon.PeriodFirstHalf, shape: fixedOutcomes, picks: homeOrAway},
	"86":     {code: canon.CodeDNB, period: canon.PeriodSecondHalf, shape: fixedOutcomes, picks: homeOrAway},
	"66":     {code: canon.CodeAH, period: canon.PeriodFirstHalf, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"88":     {code: canon.CodeAH, period: canon.PeriodSecondHalf, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"68":     {code: canon.CodeOU, period: canon.PeriodFirstHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"90":     {code: canon.CodeOU, period: canon.PeriodSecondHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"75":     {code: canon.CodeBTTS, period: canon.PeriodFirstHalf, shape: fixedOutcomes, picks: yesNo},
	"95":     {code: canon.CodeBTTS, period: canon.PeriodSecondHalf, shape: fixedOutcomes, picks: yesNo},
	"81":     {code: canon.CodeCS, period: canon.PeriodFirstHalf, shape: correctScoreOutcomes},
	"98":     {code: canon.CodeCS, period: canon.PeriodSecondHalf, shape: correctScoreOutcomes},
	"166":    {code: canon.CodeCornOU, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"177":    {code: canon.CodeCornOU, period: canon.PeriodFirstHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"2":      {code: canon.CodeQualify, shape: fixedOutcomes, picks: homeOrAway},
	"38":     {code: canon.PropFirst, shape: playerYes},
	"39":     {code: canon.PropLast, shape: playerYes},
	"40":     {code: canon.PropAnytime, shape: playerYes},
	"800289": {code: canon.PropGA, shape: playerYes},
	"800296": {code: canon.PropCard, shape: playerYes},
	"775":    {code: canon.PropGoals, shape: playerOverLadder},
	"776":    {code: canon.PropShots, shape: playerOverLadder},
	"777":    {code: canon.PropSOT, shape: playerOverLadder},
	"770":    {code: canon.PropAssists, shape: playerOverLadder},
	"780":    {code: canon.PropTackles, shape: playerOverLadder},
	"800291": {code: canon.PropFouls, shape: playerOverLadder},
	"778":    {code: canon.PropPasses, shape: playerOverLadder},
	"800297": {code: canon.PropSaves, shape: playerOverLadder},
	"19":     {code: canon.TeamGoals, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder, team: canon.SideHome},
	"20":     {code: canon.TeamGoals, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder, team: canon.SideAway},
	"69":     {code: canon.TeamGoals, period: canon.PeriodFirstHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder, team: canon.SideHome},
	"70":     {code: canon.TeamGoals, period: canon.PeriodFirstHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder, team: canon.SideAway},
	"91":     {code: canon.TeamGoals, period: canon.PeriodSecondHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder, team: canon.SideHome},
	"92":     {code: canon.TeamGoals, period: canon.PeriodSecondHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder, team: canon.SideAway},
}

func (ev event) teamName(side canon.Side) string {
	switch side {
	case canon.SideHome:
		return ev.HomeTeamName
	case canon.SideAway:
		return ev.AwayTeamName
	case canon.SideDraw, canon.SideOver, canon.SideUnder, canon.SideYes, canon.SideNo:
	}
	return ""
}

var halfTimeFullTime = map[string]pick{
	"418": {label: "1/1"}, "420": {label: "1/X"}, "422": {label: "1/2"},
	"424": {label: "X/1"}, "426": {label: "X/X"}, "428": {label: "X/2"},
	"430": {label: "2/1"}, "432": {label: "2/X"}, "434": {label: "2/2"},
}

var periodLineRules = map[string]marketRule{
	"219": {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: homeOrAway},
	"223": {code: canon.CodeSpread, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"225": {code: canon.CodeTotal, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"64":  {code: canon.CodeMoneyline, period: canon.PeriodFirstHalf, shape: fixedOutcomes, picks: homeOrAway},
	"66":  {code: canon.CodeSpread, period: canon.PeriodFirstHalf, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"68":  {code: canon.CodeTotal, period: canon.PeriodFirstHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"86":  {code: canon.CodeMoneyline, period: canon.PeriodSecondHalf, shape: fixedOutcomes, picks: homeOrAway},
	"88":  {code: canon.CodeSpread, period: canon.PeriodSecondHalf, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"90":  {code: canon.CodeTotal, period: canon.PeriodSecondHalf, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"302": {code: canon.CodeMoneyline, periodKey: quarterSpecifier, periods: quarters, shape: fixedOutcomes, picks: homeOrAway},
	"303": {code: canon.CodeSpread, periodKey: quarterSpecifier, periods: quarters, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"236": {code: canon.CodeTotal, periodKey: quarterSpecifier, periods: quarters, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
}

var basketballRules = withRules(periodLineRules, map[string]marketRule{
	"921": {code: canon.PropPoints, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"922": {code: canon.PropBkAssists, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"923": {code: canon.PropRebounds, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"924": {code: canon.PropThrees, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"768": {code: canon.PropPoints, shape: playerOverLadder},
	"770": {code: canon.PropBkAssists, shape: playerOverLadder},
	"772": {code: canon.PropRebounds, shape: playerOverLadder},
	"774": {code: canon.PropThrees, shape: playerOverLadder},
})

var americanFootballRules = withRules(periodLineRules, map[string]marketRule{
	"914":  {code: canon.PropPassYards, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"916":  {code: canon.PropPassTDs, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"918":  {code: canon.PropRushYards, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"919":  {code: canon.PropRecYards, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"920":  {code: canon.PropReceptions, lineKey: totalSpecifier, shape: playerOverUnder, picks: overUnder},
	"1235": {code: canon.PropAnytimeTD, shape: playerFirstRungYes},
})

var tennisRules = map[string]marketRule{
	"186": {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: homeOrAway},
	"188": {code: canon.CodeSetHcp, lineKey: handicapSpecifier, shape: fixedOutcomes, picks: handicapSides},
	"189": {code: canon.CodeTotalGames, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
	"202": {code: canon.CodeMoneyline, periodKey: setSpecifier, periods: firstSetOnly, shape: fixedOutcomes, picks: homeOrAway},
	"204": {code: canon.CodeTotalGames, periodKey: setSpecifier, periods: firstSetOnly, lineKey: totalSpecifier, shape: fixedOutcomes, picks: overUnder},
}

var esportsRules = map[string]marketRule{
	"186": {code: canon.CodeMoneyline, shape: fixedOutcomes, picks: homeOrAway},
}

var rulesBySport = map[canon.Sport]map[string]marketRule{
	canon.SportFootball:         footballRules,
	canon.SportBasketball:       basketballRules,
	canon.SportAmericanFootball: americanFootballRules,
	canon.SportTennis:           tennisRules,
	canon.SportEsports:          esportsRules,
}

func withRules(base, extra map[string]marketRule) map[string]marketRule {
	out := maps.Clone(base)
	maps.Copy(out, extra)
	return out
}

const otherScoreDesc = "Other"

func suppliedCodes() []canon.MarketCode {
	mapped := map[canon.MarketCode]bool{}
	for _, rules := range rulesBySport {
		for _, rule := range rules {
			mapped[rule.code] = true
		}
	}
	return canon.InCanonicalOrder(mapped)
}

func (a *Adapter) rawMarkets(ev event, observedAt time.Time) []canon.RawMarket {
	rules := rulesBySport[sportOfSportradarID[ev.Sport.ID]]
	var out, rungs []canon.RawMarket
	for _, m := range ev.Markets {
		rule, ok := rules[m.ID]
		if !ok {
			a.deps.DropUnknownMarket(a.name, m.sourceID())
			continue
		}
		if m.Status != marketOpen {
			continue
		}
		period, ok := rule.periodOf(m)
		if !ok {
			continue
		}
		switch rule.shape {
		case fixedOutcomes, correctScoreOutcomes, playerOverUnder:
			if raw, ok := matchMarket(ev.EventID, rule, period, m, observedAt); ok {
				raw.Team = ev.teamName(rule.team)
				out = append(out, raw)
			}
		case playerOverLadder:
			rungs = append(rungs, playerMarkets(ev.EventID, rule, m, observedAt)...)
		case playerYes, playerFirstRungYes:
			out = append(out, playerMarkets(ev.EventID, rule, m, observedAt)...)
		}
	}
	return append(out, rungsNotQuoted(rungs, out)...)
}

type playerLineKey struct {
	code     canon.MarketCode
	playerID string
	line     float64
	period   string
}

func playerLineKeyOf(m canon.RawMarket) (playerLineKey, bool) {
	if m.Player == nil || m.Line == nil {
		return playerLineKey{}, false
	}
	return playerLineKey{code: m.Code, playerID: m.Player.SourceID, line: *m.Line, period: m.Period}, true
}

func rungsNotQuoted(rungs, quoted []canon.RawMarket) []canon.RawMarket {
	twoSided := map[playerLineKey]bool{}
	for _, m := range quoted {
		if key, ok := playerLineKeyOf(m); ok {
			twoSided[key] = true
		}
	}
	kept := rungs[:0]
	for _, m := range rungs {
		if key, ok := playerLineKeyOf(m); ok && twoSided[key] {
			continue
		}
		kept = append(kept, m)
	}
	return kept
}

func (r marketRule) periodOf(m market) (string, bool) {
	if r.periodKey == "" {
		return r.period, true
	}
	value, ok := m.specifierValue(r.periodKey)
	if !ok {
		return "", false
	}
	period, ok := r.periods[value]
	return period, ok
}

func matchMarket(eventID string, rule marketRule, period string, m market, observedAt time.Time) (canon.RawMarket, bool) {
	raw := canon.RawMarket{EventSourceID: eventID, Code: rule.code, Period: period, Rules: rule.code.DefaultRule(), SourceMarketID: m.sourceID()}
	if rule.lineKey != "" {
		line, ok := m.lineValue(rule.lineKey)
		if !ok {
			return raw, false
		}
		raw.Line = canon.Line(line)
	}
	if rule.shape == playerOverUnder {
		player, ok := playerFromSpecifier(m)
		if !ok {
			return raw, false
		}
		raw.Player = player
	}
	for _, o := range m.Outcomes {
		price, ok := activePrice(o, observedAt)
		if !ok {
			continue
		}
		p, ok := rule.pickFor(o)
		if !ok {
			continue
		}
		raw.Selections = append(raw.Selections, canon.Selection{Side: p.side, Label: p.label, Price: price})
	}
	return raw, len(raw.Selections) > 0
}

func (m market) lineValue(key string) (float64, bool) {
	value, ok := m.specifierValue(key)
	if !ok {
		return 0, false
	}
	line, err := strconv.ParseFloat(value, 64)
	return line, err == nil
}

func (r marketRule) pickFor(o outcome) (pick, bool) {
	if r.shape == correctScoreOutcomes {
		if o.Desc == otherScoreDesc {
			return pick{label: otherScoreDesc}, true
		}
		home, away, ok := strings.Cut(o.Desc, ":")
		return pick{label: home + "-" + away}, ok
	}
	p, ok := r.picks[o.ID]
	return p, ok
}

func playerMarkets(eventID string, rule marketRule, m market, observedAt time.Time) []canon.RawMarket {
	var out []canon.RawMarket
	for _, o := range m.Outcomes {
		price, ok := activePrice(o, observedAt)
		if !ok {
			continue
		}
		ref, ok := parsePlayerOutcomeID(o.ID)
		if !ok {
			continue
		}
		raw := canon.RawMarket{
			EventSourceID:  eventID,
			Code:           rule.code,
			Player:         playerFromDesc(ref.playerID, o.Desc),
			Rules:          rule.code.DefaultRule(),
			SourceMarketID: m.sourceID() + "#" + o.ID,
			Selections:     []canon.Selection{{Side: canon.SideYes, Price: price}},
		}
		switch rule.shape {
		case playerOverLadder:
			if ref.atLeast < 1 {
				continue
			}
			raw.Line = canon.Line(float64(ref.atLeast) - 0.5)
			raw.Selections[0].Side = canon.SideOver
		case playerFirstRungYes:
			if ref.atLeast != 1 {
				continue
			}
		case fixedOutcomes, correctScoreOutcomes, playerYes, playerOverUnder:
		}
		out = append(out, raw)
	}
	return out
}

func activePrice(o outcome, observedAt time.Time) (canon.Price, bool) {
	if o.IsActive != outcomeActive {
		return canon.Price{}, false
	}
	odds, err := strconv.ParseFloat(o.Odds, 64)
	if err != nil || odds <= 1 {
		return canon.Price{}, false
	}
	return canon.NewPrice(odds, canon.FormDecimal, observedAt), true
}
