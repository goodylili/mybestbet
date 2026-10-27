package onexbet

import "mybestbet/pkg/canon"

const (
	playerStatsTopic  = "Players' stats"
	cornersTopic      = "Corners"
	acesTopic         = "Aces"
	doubleFaultsTopic = "Double Faults"
	noPeriod          = 0
	firstHalfOfFour   = 11
	secondHalfOfFour  = 12
)

type subGameSlot struct {
	topic  string
	period int
}

func (s subGame) slot() subGameSlot {
	slot := subGameSlot{topic: s.Topic}
	if s.Period != nil {
		slot.period = *s.Period
	}
	return slot
}

type sportScope struct {
	sport  canon.Sport
	scope  gameScope
	period string
}

type subGameTarget struct {
	scope  gameScope
	period string
}

var pointsSportSubGames = map[subGameSlot]subGameTarget{
	{"", firstHalfOfFour}:        {subPeriod, canon.PeriodFirstHalf},
	{"", secondHalfOfFour}:       {subPeriod, canon.PeriodSecondHalf},
	{"", 1}:                      {subPeriod, canon.PeriodQuarter1},
	{"", 2}:                      {subPeriod, canon.PeriodQuarter2},
	{"", 3}:                      {subPeriod, canon.PeriodQuarter3},
	{"", 4}:                      {subPeriod, canon.PeriodQuarter4},
	{playerStatsTopic, noPeriod}: {playerStats, ""},
}

var subGameTargets = map[canon.Sport]map[subGameSlot]subGameTarget{
	canon.SportFootball: {
		{"", 1}:                      {subPeriod, canon.PeriodFirstHalf},
		{"", 2}:                      {secondHalf, canon.PeriodSecondHalf},
		{cornersTopic, noPeriod}:     {corners, ""},
		{playerStatsTopic, noPeriod}: {playerStats, ""},
	},
	canon.SportBasketball:       pointsSportSubGames,
	canon.SportAmericanFootball: pointsSportSubGames,
	canon.SportTennis: {
		{"", 1}:                       {subPeriod, canon.PeriodFirstSet},
		{acesTopic, noPeriod}:         {aces, ""},
		{doubleFaultsTopic, noPeriod}: {doubleFaults, ""},
	},
	canon.SportEsports: {
		{"", 1}:               {subPeriod, canon.PeriodFirstMap},
		{playerStatsTopic, 1}: {playerStats, canon.PeriodFirstMap},
	},
}
