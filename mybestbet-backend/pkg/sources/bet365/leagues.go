package bet365

import (
	"maps"
	"slices"
	"strings"

	"mybestbet/pkg/canon"
)

const (
	couponPathFormat    = "#/AC/%s/C1/D1002/G40/J99/Q1/F^72/"
	matchPagePathFormat = "#/AC/%s/C1/D8/E%s/F3/"
	eventIDSeparator    = "-"
	competitionDivider  = " - "
)

type sportClass string

const (
	classSoccer           sportClass = "B1"
	classBasketball       sportClass = "B18"
	classTennis           sportClass = "B13"
	classAmericanFootball sportClass = "B12"
	classEsports          sportClass = "B151"
)

var classBySport = map[canon.Sport]sportClass{
	canon.SportFootball:         classSoccer,
	canon.SportBasketball:       classBasketball,
	canon.SportTennis:           classTennis,
	canon.SportAmericanFootball: classAmericanFootball,
	canon.SportEsports:          classEsports,
}

var sportByClass = func() map[sportClass]canon.Sport {
	m := make(map[sportClass]canon.Sport, len(classBySport))
	for sport, class := range classBySport {
		m[class] = sport
	}
	return m
}()

func eventSourceID(class sportClass, fixtureID string) string {
	if class == classSoccer {
		return fixtureID
	}
	return string(class) + eventIDSeparator + fixtureID
}

func splitEventSourceID(eventID string) (sportClass, string) {
	prefix, fixtureID, ok := strings.Cut(eventID, eventIDSeparator)
	if _, known := sportByClass[sportClass(prefix)]; ok && known {
		return sportClass(prefix), fixtureID
	}
	return classSoccer, eventID
}

var sportByLeague = map[string]canon.Sport{
	"WC": canon.SportFootball, "FRIENDLY": canon.SportFootball, "UCL": canon.SportFootball, "UEL": canon.SportFootball,
	"UECL": canon.SportFootball, "UWCL": canon.SportFootball, "EPL": canon.SportFootball, "CHAMP": canon.SportFootball,
	"FACUP": canon.SportFootball, "EFLCUP": canon.SportFootball, "LALIGA": canon.SportFootball, "COPADELREY": canon.SportFootball,
	"SERIEA": canon.SportFootball, "COPPAITALIA": canon.SportFootball, "BUNDES": canon.SportFootball, "DFBPOKAL": canon.SportFootball,
	"LIGUE1": canon.SportFootball, "CDF": canon.SportFootball, "ERED": canon.SportFootball, "PRIMEIRA": canon.SportFootball,
	"SUPERLIG": canon.SportFootball, "SPFL": canon.SportFootball, "JPL": canon.SportFootball, "MLS": canon.SportFootball,
	"LIGAMX": canon.SportFootball, "BRA1": canon.SportFootball, "BRA2": canon.SportFootball, "ARG1": canon.SportFootball,
	"SPL": canon.SportFootball, "SSL": canon.SportFootball, "GSL": canon.SportFootball, "UPL": canon.SportFootball,
	"RPL": canon.SportFootball, "ELITESERIEN": canon.SportFootball, "DSL": canon.SportFootball, "J1": canon.SportFootball,
	"J2": canon.SportFootball, "KLEAGUE": canon.SportFootball, "CSL": canon.SportFootball, "ISL": canon.SportFootball,
	"ALEAGUE": canon.SportFootball, "LEAGUESCUP": canon.SportFootball, "LIBERTADORES": canon.SportFootball,
	"SUDAMERICANA": canon.SportFootball, "SUPERCOPA": canon.SportFootball,
	"NBA": canon.SportBasketball, "NCAAB": canon.SportBasketball, "WNBA": canon.SportBasketball,
	"EUROLEAGUE": canon.SportBasketball, "EUROCUP": canon.SportBasketball, "ACB": canon.SportBasketball, "NBL": canon.SportBasketball,
	"ATP_MIAMI": canon.SportTennis, "ATP_RG": canon.SportTennis, "ATP_WIM": canon.SportTennis, "ATP_AO": canon.SportTennis,
	"ATP_USO": canon.SportTennis, "ATP_CH_ZADAR": canon.SportTennis, "ATP_CH_MURCIA": canon.SportTennis,
	"ATP_CH_MORELOS": canon.SportTennis, "ATP_CH_ASUNCION": canon.SportTennis, "WTA_MIAMI": canon.SportTennis,
	"WTA_RG": canon.SportTennis, "WTA_WIM": canon.SportTennis, "WTA_AO": canon.SportTennis, "WTA_USO": canon.SportTennis,
	"NFL": canon.SportAmericanFootball, "NCAAF": canon.SportAmericanFootball,
	"CS2": canon.SportEsports, "DOTA2": canon.SportEsports, "VALORANT": canon.SportEsports, "LOL": canon.SportEsports,
}

var leaguesByCompetition = map[string]string{
	"world cup 2026":                "WC",
	"fifa world cup":                "WC",
	"international friendlies":      "FRIENDLY",
	"uefa champions league":         "UCL",
	"uefa europa league":            "UEL",
	"uefa conference league":        "UECL",
	"uefa europa conference league": "UECL",
	"uefa women's champions league": "UWCL",
	"england premier league":        "EPL",
	"england championship":          "CHAMP",
	"england fa cup":                "FACUP",
	"england efl cup":               "EFLCUP",
	"spain la liga":                 "LALIGA",
	"spain copa del rey":            "COPADELREY",
	"spain super cup":               "SUPERCOPA",
	"italy serie a":                 "SERIEA",
	"italy coppa italia":            "COPPAITALIA",
	"germany bundesliga i":          "BUNDES",
	"germany dfb pokal":             "DFBPOKAL",
	"france ligue 1":                "LIGUE1",
	"france coupe de france":        "CDF",
	"netherlands eredivisie":        "ERED",
	"portugal primeira liga":        "PRIMEIRA",
	"turkey super lig":              "SUPERLIG",
	"scotland premiership":          "SPFL",
	"belgium first division a":      "JPL",
	"usa mls":                       "MLS",
	"mexico liga mx":                "LIGAMX",
	"brazil serie a":                "BRA1",
	"brazil serie b":                "BRA2",
	"argentina primera division":    "ARG1",
	"saudi arabia pro league":       "SPL",
	"switzerland super league":      "SSL",
	"greece super league":           "GSL",
	"ukraine premier league":        "UPL",
	"russia premier league":         "RPL",
	"norway eliteserien":            "ELITESERIEN",
	"denmark superliga":             "DSL",
	"japan j-league":                "J1",
	"japan j2 league":               "J2",
	"south korea k-league 1":        "KLEAGUE",
	"china super league":            "CSL",
	"india super league":            "ISL",
	"australia a-league":            "ALEAGUE",
	"leagues cup":                   "LEAGUESCUP",
	"copa libertadores":             "LIBERTADORES",
	"copa sudamericana":             "SUDAMERICANA",
	"nba":                           "NBA",
	"ncaab":                         "NCAAB",
	"wnba":                          "WNBA",
	"euroleague":                    "EUROLEAGUE",
	"eurocup":                       "EUROCUP",
	"spain acb":                     "ACB",
	"australia nbl":                 "NBL",
	"atp miami":                     "ATP_MIAMI",
	"atp french open":               "ATP_RG",
	"atp roland garros":             "ATP_RG",
	"atp wimbledon":                 "ATP_WIM",
	"atp australian open":           "ATP_AO",
	"atp us open":                   "ATP_USO",
	"challenger zadar":              "ATP_CH_ZADAR",
	"challenger murcia":             "ATP_CH_MURCIA",
	"challenger morelos":            "ATP_CH_MORELOS",
	"challenger asuncion":           "ATP_CH_ASUNCION",
	"wta miami":                     "WTA_MIAMI",
	"wta french open":               "WTA_RG",
	"wta roland garros":             "WTA_RG",
	"wta wimbledon":                 "WTA_WIM",
	"wta australian open":           "WTA_AO",
	"wta us open":                   "WTA_USO",
	"nfl":                           "NFL",
	"ncaaf":                         "NCAAF",
	"cs2":                           "CS2",
	"counter-strike 2":              "CS2",
	"dota 2":                        "DOTA2",
	"valorant":                      "VALORANT",
	"lol":                           "LOL",
	"league of legends":             "LOL",
	"atp challenger zadar":          "ATP_CH_ZADAR",
	"atp challenger murcia":         "ATP_CH_MURCIA",
	"atp challenger morelos":        "ATP_CH_MORELOS",
	"atp challenger asuncion":       "ATP_CH_ASUNCION",
	"argentina liga profesional":    "ARG1",
	"belgium pro league":            "JPL",
	"australia a-league men":        "ALEAGUE",
}

func leagueOfCompetition(competition string) string {
	name := strings.ToLower(strings.TrimSpace(competition))
	if league := leaguesByCompetition[name]; league != "" {
		return league
	}
	family, _, hasEvent := strings.Cut(name, competitionDivider)
	if hasEvent {
		return leaguesByCompetition[family]
	}
	return ""
}

func supportedLeagues() []string {
	return slices.Sorted(maps.Keys(sportByLeague))
}
