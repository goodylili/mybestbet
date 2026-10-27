package server

import "mybestbet/pkg/canon"

var halvesAndQuarters = []string{canon.PeriodFullTime, canon.PeriodFirstHalf, canon.PeriodSecondHalf, canon.PeriodQuarter1, canon.PeriodQuarter2, canon.PeriodQuarter3, canon.PeriodQuarter4}

var sportSegments = map[canon.Sport][]string{
	canon.SportFootball:         {canon.PeriodFullTime, canon.PeriodFirstHalf, canon.PeriodSecondHalf},
	canon.SportBasketball:       halvesAndQuarters,
	canon.SportAmericanFootball: halvesAndQuarters,
	canon.SportTennis:           {canon.PeriodFullTime, canon.PeriodFirstSet},
	canon.SportEsports:          {canon.PeriodFullTime, canon.PeriodFirstMap},
}

var segmentedMarkets = map[canon.MarketCode]bool{
	canon.CodeOU: true, canon.CodeBTTS: true, canon.CodeCornOU: true,
	canon.CodeMoneyline: true, canon.CodeSpread: true, canon.CodeTotal: true, canon.CodeTotalGames: true,
}

var fixedPeriods = map[canon.MarketCode][]string{
	canon.Code1H1X2:    {canon.PeriodFirstHalf},
	canon.PropMapKills: {canon.PeriodFullTime, canon.PeriodFirstMap, canon.PeriodMaps1To2},
}

func marketPeriods(sport canon.Sport, code canon.MarketCode) []string {
	if periods, ok := fixedPeriods[code]; ok {
		return periods
	}
	if segmentedMarkets[code] {
		return sportSegments[sport]
	}
	return []string{canon.PeriodFullTime}
}

func marketsOf(sport canon.Sport) []MarketInfo {
	codes := append(canon.MarketsFor(sport, canon.KindMatch), canon.MarketsFor(sport, canon.KindPlayer)...)
	markets := make([]MarketInfo, 0, len(codes))
	for _, code := range codes {
		spec, _ := canon.SpecOf(code)
		markets = append(markets, MarketInfo{
			Code:           code,
			Label:          spec.Label,
			Kind:           spec.Kind,
			Shape:          spec.Shape,
			Periods:        marketPeriods(sport, code),
			StatField:      spec.StatField,
			RegulationOnly: spec.RegulationOnly,
		})
	}
	return markets
}
