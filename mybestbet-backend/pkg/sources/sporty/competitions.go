package sporty

import (
	"maps"
	"strings"

	"mybestbet/pkg/canon"
)

const (
	footballSportID         = "sr:sport:1"
	basketballSportID       = "sr:sport:2"
	tennisSportID           = "sr:sport:5"
	americanFootballSportID = "sr:sport:16"
	counterStrikeSportID    = "sr:sport:109"
	leagueOfLegendsSportID  = "sr:sport:110"
	dotaSportID             = "sr:sport:111"
	valorantSportID         = "sr:sport:194"
	tournamentIDPrefix      = "sr:tournament:"
	qualifyingRoundMarker   = "Qualif"
)

var sportOfSportradarID = map[string]canon.Sport{
	footballSportID:         canon.SportFootball,
	basketballSportID:       canon.SportBasketball,
	tennisSportID:           canon.SportTennis,
	americanFootballSportID: canon.SportAmericanFootball,
	counterStrikeSportID:    canon.SportEsports,
	leagueOfLegendsSportID:  canon.SportEsports,
	dotaSportID:             canon.SportEsports,
	valorantSportID:         canon.SportEsports,
}

type competition struct {
	sportID      string
	tournamentID string
	byName       *tournamentName
}

type tournamentName struct {
	category string
	contains []string
}

func (n tournamentName) matches(category, name string) bool {
	if n.category != "" && n.category != category {
		return false
	}
	if strings.Contains(name, qualifyingRoundMarker) {
		return false
	}
	for _, part := range n.contains {
		if !strings.Contains(name, part) {
			return false
		}
	}
	return true
}

func football(tournament string) competition {
	return competition{sportID: footballSportID, tournamentID: tournamentIDPrefix + tournament}
}

func tournamentOf(sportID, tournament string) competition {
	return competition{sportID: sportID, tournamentID: tournamentIDPrefix + tournament}
}

func named(sportID, category string, contains ...string) competition {
	return competition{sportID: sportID, byName: &tournamentName{category: category, contains: contains}}
}

func wholeSport(sportID string) competition { return competition{sportID: sportID} }

var defaultCompetitions = map[string]competition{
	"WC":           football("16"),
	"FRIENDLY":     football("851"),
	"UCL":          football("7"),
	"UEL":          football("679"),
	"UECL":         football("34480"),
	"UWCL":         football("696"),
	"EPL":          football("17"),
	"CHAMP":        football("18"),
	"FACUP":        football("19"),
	"EFLCUP":       football("21"),
	"LALIGA":       football("8"),
	"COPADELREY":   football("329"),
	"SERIEA":       football("23"),
	"COPPAITALIA":  football("328"),
	"BUNDES":       football("35"),
	"DFBPOKAL":     football("217"),
	"LIGUE1":       football("34"),
	"CDF":          football("335"),
	"ERED":         football("37"),
	"PRIMEIRA":     football("238"),
	"SUPERLIG":     football("52"),
	"SPFL":         football("36"),
	"JPL":          football("38"),
	"MLS":          football("242"),
	"LIGAMX":       football("27464"),
	"BRA1":         football("325"),
	"BRA2":         football("390"),
	"ARG1":         football("155"),
	"SPL":          football("955"),
	"SSL":          football("215"),
	"GSL":          football("185"),
	"UPL":          football("218"),
	"RPL":          football("203"),
	"ELITESERIEN":  football("20"),
	"DSL":          football("39"),
	"J1":           football("196"),
	"J2":           football("402"),
	"KLEAGUE":      football("410"),
	"CSL":          football("649"),
	"ISL":          football("1900"),
	"ALEAGUE":      football("136"),
	"LIBERTADORES": football("384"),
	"SUDAMERICANA": football("480"),
	"LEAGUESCUP":   named(footballSportID, "", "Leagues Cup"),
	"SUPERCOPA":    named(footballSportID, "Spain", "Supercopa"),

	"NBA":        tournamentOf(basketballSportID, "132"),
	"NCAAB":      tournamentOf(basketballSportID, "648"),
	"WNBA":       tournamentOf(basketballSportID, "486"),
	"EUROLEAGUE": tournamentOf(basketballSportID, "138"),
	"EUROCUP":    tournamentOf(basketballSportID, "141"),
	"ACB":        tournamentOf(basketballSportID, "264"),
	"NBL":        tournamentOf(basketballSportID, "1524"),

	"ATP_AO":          tournamentOf(tennisSportID, "2567"),
	"ATP_RG":          tournamentOf(tennisSportID, "2579"),
	"ATP_USO":         tournamentOf(tennisSportID, "2591"),
	"ATP_WIM":         tournamentOf(tennisSportID, "2555"),
	"WTA_AO":          tournamentOf(tennisSportID, "2571"),
	"WTA_RG":          tournamentOf(tennisSportID, "2583"),
	"WTA_USO":         tournamentOf(tennisSportID, "2595"),
	"WTA_WIM":         tournamentOf(tennisSportID, "2559"),
	"ATP_MIAMI":       named(tennisSportID, "ATP", "ATP Miami", "Singles"),
	"WTA_MIAMI":       named(tennisSportID, "WTA", "WTA Miami", "Singles"),
	"ATP_CH_ZADAR":    named(tennisSportID, "Challenger", "Zadar", "Singles"),
	"ATP_CH_MURCIA":   named(tennisSportID, "Challenger", "Murcia", "Singles"),
	"ATP_CH_MORELOS":  named(tennisSportID, "Challenger", "Morelos", "Singles"),
	"ATP_CH_ASUNCION": named(tennisSportID, "Challenger", "Asuncion", "Singles"),

	"NFL":   tournamentOf(americanFootballSportID, "31"),
	"NCAAF": tournamentOf(americanFootballSportID, "27653"),

	"CS2":      wholeSport(counterStrikeSportID),
	"LOL":      wholeSport(leagueOfLegendsSportID),
	"DOTA2":    wholeSport(dotaSportID),
	"VALORANT": wholeSport(valorantSportID),
}

func competitionsWithOverrides(leagueIDs map[string]string) map[string]competition {
	out := maps.Clone(defaultCompetitions)
	for code, tournamentID := range leagueIDs {
		c, ok := out[code]
		if !ok {
			c.sportID = footballSportID
		}
		out[code] = competition{sportID: c.sportID, tournamentID: tournamentID}
	}
	return out
}
