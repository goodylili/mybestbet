package bet9ja

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

type selectionShape int

const (
	fixedOutcomes selectionShape = iota + 1
	correctScoreOutcomes
)

type pick struct {
	side  canon.Side
	label string
}

type marketRule struct {
	code    canon.MarketCode
	period  string
	hasLine bool
	shape   selectionShape
	picks   map[string]pick
}

var (
	homeDrawAway = map[string]pick{"1": {side: canon.SideHome}, "X": {side: canon.SideDraw}, "2": {side: canon.SideAway}}
	overUnder    = map[string]pick{"O": {side: canon.SideOver}, "U": {side: canon.SideUnder}}
	homeAway     = map[string]pick{"1": {side: canon.SideHome}, "2": {side: canon.SideAway}}
	yesNo        = map[string]pick{"Y": {side: canon.SideYes}, "N": {side: canon.SideNo}}
	doubleChance = map[string]pick{"1X": {label: "1X"}, "12": {label: "12"}, "X2": {label: "X2"}}
)

func twoWay(home, away string) map[string]pick {
	return map[string]pick{home: {side: canon.SideHome}, away: {side: canon.SideAway}}
}

func fixed(code canon.MarketCode, period string, picks map[string]pick) marketRule {
	return marketRule{code: code, period: period, shape: fixedOutcomes, picks: picks}
}

func lined(code canon.MarketCode, period string, picks map[string]pick) marketRule {
	return marketRule{code: code, period: period, hasLine: true, shape: fixedOutcomes, picks: picks}
}

var marketRules = map[string]marketRule{
	"S_1X2":       fixed(canon.Code1X2, canon.PeriodFullTime, homeDrawAway),
	"S_DC":        fixed(canon.CodeDC, canon.PeriodFullTime, doubleChance),
	"S_DC1T":      fixed(canon.CodeDC, canon.PeriodFirstHalf, doubleChance),
	"S_DC2T":      fixed(canon.CodeDC, canon.PeriodSecondHalf, doubleChance),
	"S_OU":        lined(canon.CodeOU, canon.PeriodFullTime, overUnder),
	"S_OUA":       lined(canon.CodeOU, canon.PeriodFullTime, overUnder),
	"S_OU1T":      lined(canon.CodeOU, canon.PeriodFirstHalf, overUnder),
	"S_OU2T":      lined(canon.CodeOU, canon.PeriodSecondHalf, overUnder),
	"S_GGNG":      fixed(canon.CodeBTTS, canon.PeriodFullTime, yesNo),
	"S_GGNG1T":    fixed(canon.CodeBTTS, canon.PeriodFirstHalf, yesNo),
	"S_GGNG2T":    fixed(canon.CodeBTTS, canon.PeriodSecondHalf, yesNo),
	"S_DNB":       fixed(canon.CodeDNB, canon.PeriodFullTime, homeAway),
	"S_DNB1T":     fixed(canon.CodeDNB, canon.PeriodFirstHalf, homeAway),
	"S_DNB2T":     fixed(canon.CodeDNB, canon.PeriodSecondHalf, homeAway),
	"S_AH":        lined(canon.CodeAH, canon.PeriodFullTime, homeAway),
	"S_CSFT":      {code: canon.CodeCS, shape: correctScoreOutcomes},
	"S_HTFT":      fixed(canon.CodeHTFT, canon.PeriodFullTime, halfTimeFullTime),
	"S_1X21T":     fixed(canon.Code1H1X2, canon.PeriodFirstHalf, homeDrawAway),
	"S_12QUALIFY": fixed(canon.CodeQualify, canon.PeriodFullTime, homeAway),
	"S_OUCORNERS": lined(canon.CodeCornOU, canon.PeriodFullTime, overUnder),
	"S_C1HTOU":    lined(canon.CodeCornOU, canon.PeriodFirstHalf, overUnder),
	"S_C2HTOU":    lined(canon.CodeCornOU, canon.PeriodSecondHalf, overUnder),

	"B_12":    fixed(canon.CodeMoneyline, canon.PeriodFullTime, homeAway),
	"B_H":     lined(canon.CodeSpread, canon.PeriodFullTime, homeAway),
	"B_OUN":   lined(canon.CodeTotal, canon.PeriodFullTime, overUnder),
	"B_DNB1T": fixed(canon.CodeMoneyline, canon.PeriodFirstHalf, twoWay("1DNBHT", "2DNBHT")),
	"B_1HH":   lined(canon.CodeSpread, canon.PeriodFirstHalf, homeAway),
	"B_1HOU":  lined(canon.CodeTotal, canon.PeriodFirstHalf, overUnder),
	"B_2HDNB": fixed(canon.CodeMoneyline, canon.PeriodSecondHalf, twoWay("1D2", "2D2")),
	"B_2HH":   lined(canon.CodeSpread, canon.PeriodSecondHalf, homeAway),
	"B_2HOU":  lined(canon.CodeTotal, canon.PeriodSecondHalf, overUnder),
	"B_1QD":   fixed(canon.CodeMoneyline, canon.PeriodQuarter1, twoWay("1D", "2D")),
	"B_1QH":   lined(canon.CodeSpread, canon.PeriodQuarter1, homeAway),
	"B_1QOU":  lined(canon.CodeTotal, canon.PeriodQuarter1, overUnder),
	"B_2QD":   fixed(canon.CodeMoneyline, canon.PeriodQuarter2, twoWay("1D", "2D")),
	"B_2QH":   lined(canon.CodeSpread, canon.PeriodQuarter2, homeAway),
	"B_2QOU":  lined(canon.CodeTotal, canon.PeriodQuarter2, overUnder),
	"B_3QD":   fixed(canon.CodeMoneyline, canon.PeriodQuarter3, twoWay("1D", "2D")),
	"B_3QH":   lined(canon.CodeSpread, canon.PeriodQuarter3, homeAway),
	"B_3QOU":  lined(canon.CodeTotal, canon.PeriodQuarter3, overUnder),
	"B_4QD":   fixed(canon.CodeMoneyline, canon.PeriodQuarter4, twoWay("1D", "2D")),
	"B_4QH":   lined(canon.CodeSpread, canon.PeriodQuarter4, homeAway),
	"B_4QOU":  lined(canon.CodeTotal, canon.PeriodQuarter4, overUnder),

	"AF_121":  fixed(canon.CodeMoneyline, canon.PeriodFullTime, homeAway),
	"AF_H":    lined(canon.CodeSpread, canon.PeriodFullTime, homeAway),
	"AF_T":    lined(canon.CodeTotal, canon.PeriodFullTime, overUnder),
	"AF_1HD":  fixed(canon.CodeMoneyline, canon.PeriodFirstHalf, homeAway),
	"AF_1HH":  lined(canon.CodeSpread, canon.PeriodFirstHalf, homeAway),
	"AF_1HT":  lined(canon.CodeTotal, canon.PeriodFirstHalf, overUnder),
	"AF_2HDF": fixed(canon.CodeMoneyline, canon.PeriodSecondHalf, homeAway),
	"AF_2HH":  lined(canon.CodeSpread, canon.PeriodSecondHalf, homeAway),
	"AF_2HTF": lined(canon.CodeTotal, canon.PeriodSecondHalf, overUnder),
	"AF_1QD":  fixed(canon.CodeMoneyline, canon.PeriodQuarter1, homeAway),
	"AF_1QH":  lined(canon.CodeSpread, canon.PeriodQuarter1, homeAway),
	"AF_1QT":  lined(canon.CodeTotal, canon.PeriodQuarter1, overUnder),
	"AF_2QD":  fixed(canon.CodeMoneyline, canon.PeriodQuarter2, homeAway),
	"AF_2QH":  lined(canon.CodeSpread, canon.PeriodQuarter2, homeAway),
	"AF_2QT":  lined(canon.CodeTotal, canon.PeriodQuarter2, overUnder),
	"AF_3QD":  fixed(canon.CodeMoneyline, canon.PeriodQuarter3, homeAway),
	"AF_3QH":  lined(canon.CodeSpread, canon.PeriodQuarter3, homeAway),
	"AF_3QT":  lined(canon.CodeTotal, canon.PeriodQuarter3, overUnder),
	"AF_4QDR": fixed(canon.CodeMoneyline, canon.PeriodQuarter4, homeAway),
	"AF_4QHR": lined(canon.CodeSpread, canon.PeriodQuarter4, homeAway),
	"AF_4QTR": lined(canon.CodeTotal, canon.PeriodQuarter4, overUnder),

	"T_12":   fixed(canon.CodeMoneyline, canon.PeriodFullTime, homeAway),
	"T_121P": fixed(canon.CodeMoneyline, canon.PeriodFirstSet, homeAway),
	"T_SH":   lined(canon.CodeSetHcp, canon.PeriodFullTime, homeAway),
	"T_OUG":  lined(canon.CodeTotalGames, canon.PeriodFullTime, overUnder),
}

var halfTimeFullTime = map[string]pick{
	"1/1": {label: "1/1"}, "1/X": {label: "1/X"}, "1/2": {label: "1/2"},
	"X/1": {label: "X/1"}, "X/X": {label: "X/X"}, "X/2": {label: "X/2"},
	"2/1": {label: "2/1"}, "2/X": {label: "2/X"}, "2/2": {label: "2/2"},
}

const (
	otherScoreOutcome = "OTH"
	otherScoreLabel   = "Other"
)

func suppliedCodes() []canon.MarketCode {
	mapped := map[canon.MarketCode]bool{}
	for _, rule := range marketRules {
		mapped[rule.code] = true
	}
	return canon.InCanonicalOrder(mapped)
}

type oddsKey struct {
	market    string
	specifier string
	outcome   string
}

func parseOddsKey(key string) (oddsKey, bool) {
	marketAndSpecifier, outcome, ok := cutLast(key, "_")
	if !ok || outcome == "" {
		return oddsKey{}, false
	}
	market, specifier, _ := strings.Cut(marketAndSpecifier, "@")
	return oddsKey{market: market, specifier: specifier, outcome: outcome}, true
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func (k oddsKey) sourceMarketID() string {
	if k.specifier == "" {
		return k.market
	}
	return k.market + "@" + k.specifier
}

type nativeMarket struct {
	raw  canon.RawMarket
	keys []string
}

func (a *Adapter) rawMarkets(ev event, observedAt time.Time) []canon.RawMarket {
	native := a.nativeMarkets(ev, observedAt)
	out := make([]canon.RawMarket, 0, len(native))
	for _, m := range native {
		out = append(out, m.raw)
	}
	return out
}

func (a *Adapter) nativeMarkets(ev event, observedAt time.Time) []nativeMarket {
	eventID := strconv.FormatInt(ev.ID, 10)
	byMarket := map[string]*nativeMarket{}
	var order []string
	dropped := map[string]bool{}
	for _, key := range slices.Sorted(maps.Keys(ev.Odds)) {
		parsed, ok := parseOddsKey(key)
		if !ok {
			continue
		}
		rule, ok := marketRules[parsed.market]
		if !ok {
			if !dropped[parsed.market] {
				dropped[parsed.market] = true
				a.deps.DropUnknownMarket(Name, parsed.market)
			}
			continue
		}
		price := float64(ev.Odds[key])
		p, ok := rule.pickFor(parsed.outcome)
		if !ok || price <= 1 {
			continue
		}
		sourceID := parsed.sourceMarketID()
		m, seen := byMarket[sourceID]
		if !seen {
			raw, ok := newRawMarket(eventID, rule, parsed, sourceID)
			if !ok {
				continue
			}
			m = &nativeMarket{raw: *raw}
			byMarket[sourceID] = m
			order = append(order, sourceID)
		}
		m.raw.Selections = append(m.raw.Selections, canon.Selection{Side: p.side, Label: p.label, Price: canon.NewPrice(price, canon.FormDecimal, observedAt)})
		m.keys = append(m.keys, key)
	}
	out := make([]nativeMarket, 0, len(order))
	for _, id := range order {
		out = append(out, *byMarket[id])
	}
	return out
}

func newRawMarket(eventID string, rule marketRule, key oddsKey, sourceID string) (*canon.RawMarket, bool) {
	raw := &canon.RawMarket{EventSourceID: eventID, Code: rule.code, Period: rule.period, Rules: rule.code.DefaultRule(), SourceMarketID: sourceID}
	if !rule.hasLine {
		return raw, true
	}
	line, err := strconv.ParseFloat(key.specifier, 64)
	if err != nil {
		return nil, false
	}
	raw.Line = canon.Line(line)
	return raw, true
}

func (r marketRule) pickFor(outcome string) (pick, bool) {
	if r.shape != correctScoreOutcomes {
		p, ok := r.picks[outcome]
		return p, ok
	}
	if outcome == otherScoreOutcome {
		return pick{label: otherScoreLabel}, true
	}
	home, away, ok := strings.Cut(outcome, ":")
	return pick{label: home + "-" + away}, ok
}
