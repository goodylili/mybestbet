package onexbet

import (
	"maps"
	"strings"

	"mybestbet/pkg/canon"
)

const (
	footballSportID         = 1
	basketballSportID       = 3
	tennisSportID           = 4
	americanFootballSportID = 13
	esportsSportID          = 40
)

var sportOfLineFeedID = map[int]canon.Sport{
	footballSportID:         canon.SportFootball,
	basketballSportID:       canon.SportBasketball,
	tennisSportID:           canon.SportTennis,
	americanFootballSportID: canon.SportAmericanFootball,
	esportsSportID:          canon.SportEsports,
}

type competition struct {
	sportID        int
	championshipID string
	byName         *championshipName
}

type championshipName struct {
	exact  string
	prefix string
}

var outrightOrSpecialMarkers = []string{". Winner", "Special", "Statistic", "Results", "Alternative"}

func (n championshipName) matches(name string) bool {
	if n.exact != "" {
		return name == n.exact
	}
	if !strings.HasPrefix(name, n.prefix) {
		return false
	}
	for _, marker := range outrightOrSpecialMarkers {
		if strings.Contains(name, marker) {
			return false
		}
	}
	return true
}

func fixedChampionship(sportID int, id string) competition {
	return competition{sportID: sportID, championshipID: id}
}

func championshipNamed(sportID int, exact string) competition {
	return competition{sportID: sportID, byName: &championshipName{exact: exact}}
}

func championshipsPrefixed(prefix string) competition {
	return competition{sportID: esportsSportID, byName: &championshipName{prefix: prefix}}
}

var defaultCompetitions = map[string]competition{
	"EPL":          fixedChampionship(footballSportID, "88637"),
	"CHAMP":        fixedChampionship(footballSportID, "105759"),
	"LALIGA":       fixedChampionship(footballSportID, "127733"),
	"SERIEA":       fixedChampionship(footballSportID, "110163"),
	"BUNDES":       fixedChampionship(footballSportID, "96463"),
	"LIGUE1":       fixedChampionship(footballSportID, "12821"),
	"ERED":         fixedChampionship(footballSportID, "2018750"),
	"PRIMEIRA":     fixedChampionship(footballSportID, "3007689"),
	"SPL":          fixedChampionship(footballSportID, "16819"),
	"MLS":          fixedChampionship(footballSportID, "828065"),
	"UCL":          fixedChampionship(footballSportID, "118587"),
	"UEL":          fixedChampionship(footballSportID, "118593"),
	"UECL":         fixedChampionship(footballSportID, "2252762"),
	"FRIENDLY":     fixedChampionship(footballSportID, "125983"),
	"EFLCUP":       fixedChampionship(footballSportID, "119237"),
	"DFBPOKAL":     fixedChampionship(footballSportID, "119235"),
	"SUPERLIG":     fixedChampionship(footballSportID, "11113"),
	"SPFL":         fixedChampionship(footballSportID, "13521"),
	"JPL":          fixedChampionship(footballSportID, "28787"),
	"LIGAMX":       fixedChampionship(footballSportID, "2306111"),
	"BRA1":         fixedChampionship(footballSportID, "1268397"),
	"BRA2":         fixedChampionship(footballSportID, "57265"),
	"ARG1":         fixedChampionship(footballSportID, "119599"),
	"SSL":          fixedChampionship(footballSportID, "27695"),
	"GSL":          fixedChampionship(footballSportID, "8777"),
	"UPL":          fixedChampionship(footballSportID, "29949"),
	"RPL":          fixedChampionship(footballSportID, "225733"),
	"ELITESERIEN":  fixedChampionship(footballSportID, "1793471"),
	"DSL":          fixedChampionship(footballSportID, "8773"),
	"J1":           fixedChampionship(footballSportID, "118737"),
	"J2":           fixedChampionship(footballSportID, "30005"),
	"KLEAGUE":      fixedChampionship(footballSportID, "30467"),
	"CSL":          fixedChampionship(footballSportID, "58043"),
	"ISL":          fixedChampionship(footballSportID, "1122087"),
	"ALEAGUE":      fixedChampionship(footballSportID, "2905446"),
	"LIBERTADORES": fixedChampionship(footballSportID, "142091"),
	"SUDAMERICANA": fixedChampionship(footballSportID, "1528791"),
	"COPADELREY":   fixedChampionship(footballSportID, "119243"),
	"WC":           championshipNamed(footballSportID, "FIFA World Cup"),
	"UWCL":         championshipNamed(footballSportID, "UEFA Champions League. Women"),
	"FACUP":        championshipNamed(footballSportID, "England. FA Cup"),
	"COPPAITALIA":  championshipNamed(footballSportID, "Italy. Coppa Italia"),
	"CDF":          championshipNamed(footballSportID, "France. Coupe de France"),
	"LEAGUESCUP":   championshipNamed(footballSportID, "Leagues Cup"),
	"SUPERCOPA":    championshipNamed(footballSportID, "Spain. Supercopa"),

	"NBA":        fixedChampionship(basketballSportID, "13589"),
	"WNBA":       fixedChampionship(basketballSportID, "197289"),
	"EUROLEAGUE": fixedChampionship(basketballSportID, "139460"),
	"EUROCUP":    fixedChampionship(basketballSportID, "140411"),
	"ACB":        fixedChampionship(basketballSportID, "110509"),
	"NBL":        fixedChampionship(basketballSportID, "220263"),
	"NCAAB":      championshipNamed(basketballSportID, "NCAA"),

	"NFL":   fixedChampionship(americanFootballSportID, "68315"),
	"NCAAF": fixedChampionship(americanFootballSportID, "93369"),

	"ATP_MIAMI":       championshipNamed(tennisSportID, "ATP. Miami"),
	"ATP_RG":          championshipNamed(tennisSportID, "ATP. French Open"),
	"ATP_WIM":         championshipNamed(tennisSportID, "ATP. Wimbledon"),
	"ATP_AO":          championshipNamed(tennisSportID, "ATP. Australian Open"),
	"ATP_USO":         championshipNamed(tennisSportID, "ATP. US Open"),
	"ATP_CH_ZADAR":    championshipNamed(tennisSportID, "Challenger. Zadar"),
	"ATP_CH_MURCIA":   championshipNamed(tennisSportID, "Challenger. Murcia"),
	"ATP_CH_MORELOS":  championshipNamed(tennisSportID, "Challenger. Morelos"),
	"ATP_CH_ASUNCION": championshipNamed(tennisSportID, "Challenger. Asuncion"),
	"WTA_MIAMI":       championshipNamed(tennisSportID, "WTA. Miami"),
	"WTA_RG":          championshipNamed(tennisSportID, "WTA. French Open"),
	"WTA_WIM":         championshipNamed(tennisSportID, "WTA. Wimbledon"),
	"WTA_AO":          championshipNamed(tennisSportID, "WTA. Australian Open"),
	"WTA_USO":         championshipNamed(tennisSportID, "WTA. US Open"),

	"CS2":      championshipsPrefixed("CS 2. "),
	"DOTA2":    championshipsPrefixed("Dota 2. "),
	"LOL":      championshipsPrefixed("League of Legends. "),
	"VALORANT": championshipsPrefixed("Valorant. "),
}

func competitionsWithOverrides(leagueIDs map[string]string) map[string]competition {
	out := maps.Clone(defaultCompetitions)
	for code, championshipID := range leagueIDs {
		c, ok := out[code]
		if !ok {
			c.sportID = footballSportID
		}
		out[code] = fixedChampionship(c.sportID, championshipID)
	}
	return out
}
