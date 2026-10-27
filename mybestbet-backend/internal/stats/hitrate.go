package stats

import (
	"fmt"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/canon"
)

const (
	MinQualifyingMinutes = 45
	yesNoLine            = 0.5
)

var qualifyingMinutesBySport = map[canon.Sport]int{
	canon.SportFootball:   MinQualifyingMinutes,
	canon.SportBasketball: 10,
}

func QualifyingMinutes(code canon.MarketCode) int {
	spec, ok := canon.SpecOf(code)
	if !ok || len(spec.Sports) == 0 {
		return MinQualifyingMinutes
	}
	return qualifyingMinutesBySport[spec.Sports[0]]
}

var DefaultWindows = []int{5, 10, 20}

type statReading func(MatchStat) (value float64, ok bool)

type statRule struct {
	statCode canon.MarketCode
	read     statReading
	isYesNo  bool
}

func count(pick func(MatchStat) int) statReading {
	return func(m MatchStat) (float64, bool) { return float64(pick(m)), true }
}

func rating(m MatchStat) (float64, bool) {
	if m.Rating == nil {
		return 0, false
	}
	return *m.Rating, true
}

func sportValue(keys ...statkey.Key) statReading {
	return func(m MatchStat) (float64, bool) {
		total := 0.0
		for _, k := range keys {
			value, ok := m.Stats[k]
			if !ok {
				return 0, false
			}
			total += value
		}
		return total, true
	}
}

func killsPerMap(m MatchStat) (float64, bool) {
	kills, hasKills := m.Stats[statkey.Kills]
	maps := m.Stats[statkey.Maps]
	if !hasKills || maps <= 0 {
		return 0, false
	}
	return kills / maps, true
}

var (
	goals         = count(func(m MatchStat) int { return m.Goals })
	goalsOrAssist = count(func(m MatchStat) int { return m.Goals + m.Assists })
	cards         = count(func(m MatchStat) int { return m.Yellow + m.Red })
)

var statRules = map[canon.MarketCode]statRule{
	canon.PropGoals:   {statCode: canon.PropGoals, read: goals},
	canon.PropAnytime: {statCode: canon.PropGoals, read: goals, isYesNo: true},
	canon.PropFirst:   {statCode: canon.PropGoals, read: goals, isYesNo: true},
	canon.PropLast:    {statCode: canon.PropGoals, read: goals, isYesNo: true},
	canon.PropShots:   {statCode: canon.PropShots, read: count(func(m MatchStat) int { return m.Shots })},
	canon.PropSOT:     {statCode: canon.PropSOT, read: count(func(m MatchStat) int { return m.ShotsOnTarget })},
	canon.PropAssists: {statCode: canon.PropAssists, read: count(func(m MatchStat) int { return m.Assists })},
	canon.PropGA:      {statCode: canon.PropGA, read: goalsOrAssist, isYesNo: true},
	canon.PropCard:    {statCode: canon.PropCard, read: cards, isYesNo: true},
	canon.PropFouls:   {statCode: canon.PropFouls, read: count(func(m MatchStat) int { return m.Fouls })},
	canon.PropTackles: {statCode: canon.PropTackles, read: count(func(m MatchStat) int { return m.Tackles })},
	canon.PropRating:  {statCode: canon.PropRating, read: rating},
	canon.PropSaves:   {statCode: canon.PropSaves, read: sportValue(statkey.Saves)},
	canon.PropPasses:  {statCode: canon.PropPasses, read: sportValue(statkey.Passes)},

	canon.PropPoints:     {statCode: canon.PropPoints, read: sportValue(statkey.Points)},
	canon.PropRebounds:   {statCode: canon.PropRebounds, read: sportValue(statkey.Rebounds)},
	canon.PropBkAssists:  {statCode: canon.PropBkAssists, read: sportValue(statkey.Assists)},
	canon.PropThrees:     {statCode: canon.PropThrees, read: sportValue(statkey.Threes)},
	canon.PropPtsRebAst:  {statCode: canon.PropPtsRebAst, read: sportValue(statkey.Points, statkey.Rebounds, statkey.Assists)},
	canon.PropPassYards:  {statCode: canon.PropPassYards, read: sportValue(statkey.PassYards)},
	canon.PropRushYards:  {statCode: canon.PropRushYards, read: sportValue(statkey.RushYards)},
	canon.PropRecYards:   {statCode: canon.PropRecYards, read: sportValue(statkey.RecYards)},
	canon.PropReceptions: {statCode: canon.PropReceptions, read: sportValue(statkey.Receptions)},
	canon.PropPassTDs:    {statCode: canon.PropPassTDs, read: sportValue(statkey.PassTDs)},
	canon.PropAnytimeTD:  {statCode: canon.PropAnytimeTD, read: sportValue(statkey.RushTDs, statkey.RecTDs), isYesNo: true},
	canon.PropAces:       {statCode: canon.PropAces, read: sportValue(statkey.Aces)},
	canon.PropDoubleFlt:  {statCode: canon.PropDoubleFlt, read: sportValue(statkey.DoubleFaults)},
	canon.PropMapKills:   {statCode: canon.PropMapKills, read: killsPerMap},
}

func ruleFor(code canon.MarketCode, line float64) (statRule, float64, error) {
	rule, ok := statRules[code]
	if !ok {
		return statRule{}, 0, fmt.Errorf("%s: %w", code, ErrUnsupportedCode)
	}
	if rule.isYesNo {
		return rule, yesNoLine, nil
	}
	return rule, line, nil
}

func hitRateOver(matches []MatchStat, read statReading, line float64, window int) HitRate {
	rate := HitRate{Window: window}
	for _, m := range matches {
		value, ok := read(m)
		if !ok {
			continue
		}
		rate.Matches++
		if value > line {
			rate.Hits++
		}
	}
	if rate.Matches > 0 {
		rate.Rate = float64(rate.Hits) / float64(rate.Matches)
	}
	return rate
}

func computeHitRates(newestFirst []MatchStat, rule statRule, line float64, windows []int) HitRates {
	rates := HitRates{StatCode: rule.statCode, Line: line, Windows: make([]HitRate, 0, len(windows))}
	for _, w := range windows {
		rates.Windows = append(rates.Windows, hitRateOver(newestFirst[:min(w, len(newestFirst))], rule.read, line, w))
	}
	widest := newestFirst[:min(maxWindow(windows), len(newestFirst))]
	home, away := splitHomeAway(widest)
	rates.Home = hitRateOver(home, rule.read, line, len(home))
	rates.Away = hitRateOver(away, rule.read, line, len(away))
	return rates
}

func splitHomeAway(matches []MatchStat) (home, away []MatchStat) {
	for _, m := range matches {
		if m.IsHome {
			home = append(home, m)
		} else {
			away = append(away, m)
		}
	}
	return home, away
}

func maxWindow(windows []int) int {
	widest := 0
	for _, w := range windows {
		widest = max(widest, w)
	}
	return widest
}
