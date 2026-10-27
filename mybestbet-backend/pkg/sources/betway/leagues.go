package betway

import (
	"maps"
	"strings"
)

type sportID string

const (
	sportSoccer           sportID = "soccer"
	sportBasketball       sportID = "basketball"
	sportTennis           sportID = "tennis"
	sportAmericanFootball sportID = "american-football"
	sportCounterStrike    sportID = "esports-counter-strike"
	sportDota             sportID = "esports-dota"
	sportLeagueOfLegends  sportID = "esports-league-of-legends"
	sportValorant         sportID = "esports-valorant"
)

var catalogMarketTypes = map[sportID][]string{
	sportSoccer:           {"[Win/Draw/Win]"},
	sportBasketball:       {"[Basketball Win/Draw/Win]", "Winner (Incl. Overtime)"},
	sportTennis:           {"[Match Winner]"},
	sportAmericanFootball: {"[Money Line]"},
	sportCounterStrike:    {"Winner", "[Winner]"},
	sportDota:             {"Winner", "[Winner]"},
	sportLeagueOfLegends:  {"Winner", "[Winner]"},
	sportValorant:         {"Winner", "[Winner]"},
}

type leagueRef struct {
	sport  sportID
	league string
}

func (r leagueRef) coversWholeSport() bool { return r.league == "" }

func soccer(league string) leagueRef     { return leagueRef{sport: sportSoccer, league: league} }
func basketball(league string) leagueRef { return leagueRef{sport: sportBasketball, league: league} }
func tennis(league string) leagueRef     { return leagueRef{sport: sportTennis, league: league} }

var defaultLeagues = map[string]leagueRef{
	"WC":           soccer("international/fifa-world-cup"),
	"FRIENDLY":     soccer("international/int-friendly-games"),
	"UCL":          soccer("international-clubs/uefa-champions-league"),
	"UEL":          soccer("international-clubs/uefa-europa-league"),
	"UECL":         soccer("international-clubs/uefa-conference-league"),
	"UWCL":         soccer("international-clubs/uefa-champions-league-women"),
	"EPL":          soccer("england/premier-league"),
	"CHAMP":        soccer("england/championship"),
	"FACUP":        soccer("england/fa-cup"),
	"EFLCUP":       soccer("england/efl-cup"),
	"LALIGA":       soccer("spain/laliga"),
	"COPADELREY":   soccer("spain/copa-del-rey"),
	"SUPERCOPA":    soccer("spain/supercopa"),
	"SERIEA":       soccer("italy/serie-a"),
	"COPPAITALIA":  soccer("italy/coppa-italia"),
	"BUNDES":       soccer("germany/bundesliga"),
	"DFBPOKAL":     soccer("germany/dfb-pokal"),
	"LIGUE1":       soccer("france/ligue-1"),
	"CDF":          soccer("france/coupe-de-france"),
	"ERED":         soccer("netherlands/eredivisie"),
	"PRIMEIRA":     soccer("portugal/liga-portugal"),
	"SUPERLIG":     soccer("turkiye/super-lig"),
	"SPFL":         soccer("scotland/premiership"),
	"JPL":          soccer("belgium/pro-league"),
	"MLS":          soccer("usa/mls"),
	"LIGAMX":       soccer("mexico/liga-mx"),
	"BRA1":         soccer("brazil/brasileiro-serie-a"),
	"BRA2":         soccer("brazil/brasileiro-serie-b"),
	"ARG1":         soccer("argentina/primera-lpf"),
	"SPL":          soccer("saudi-arabia/saudi-pro-league"),
	"SSL":          soccer("switzerland/super-league"),
	"GSL":          soccer("greece/super-league"),
	"UPL":          soccer("ukraine/premier-league"),
	"RPL":          soccer("russia/premier-league"),
	"ELITESERIEN":  soccer("norway/eliteserien"),
	"DSL":          soccer("denmark/superliga"),
	"J1":           soccer("japan/j1-league"),
	"J2":           soccer("japan/j2-league"),
	"KLEAGUE":      soccer("republic-of-korea/k-league-1"),
	"CSL":          soccer("china/chinese-super-league"),
	"ISL":          soccer("india/indian-super-league"),
	"ALEAGUE":      soccer("australia/a-league"),
	"LEAGUESCUP":   soccer("international-clubs/leagues-cup"),
	"LIBERTADORES": soccer("international-clubs/conmebol-libertadores"),
	"SUDAMERICANA": soccer("international-clubs/conmebol-sudamericana"),

	"NBA":        basketball("usa/nba"),
	"NCAAB":      basketball("usa/ncaa-regular-season"),
	"WNBA":       basketball("usa/wnba"),
	"EUROLEAGUE": basketball("international/euroleague"),
	"EUROCUP":    basketball("international/eurocup"),
	"ACB":        basketball("spain/liga-acb"),
	"NBL":        basketball("australia/nbl"),

	"NFL":   {sport: sportAmericanFootball, league: "usa/nfl"},
	"NCAAF": {sport: sportAmericanFootball, league: "usa/ncaa-regular-season"},

	"ATP_MIAMI":       tennis("atp/atp-miami-usa-men-singles"),
	"ATP_RG":          tennis("atp/french-open-men-singles"),
	"ATP_WIM":         tennis("atp/wimbledon-men-singles"),
	"ATP_AO":          tennis("atp/australian-open-men-singles"),
	"ATP_USO":         tennis("atp/us-open-men-singles"),
	"ATP_CH_ZADAR":    tennis("challenger/atp-challenger-zadar-croatia-men-singles"),
	"ATP_CH_MURCIA":   tennis("challenger/atp-challenger-murcia-spain-men-singles"),
	"ATP_CH_MORELOS":  tennis("challenger/atp-challenger-morelos-mexico-men-singles"),
	"ATP_CH_ASUNCION": tennis("challenger/atp-challenger-asuncion-paraguay-men-singles"),
	"WTA_MIAMI":       tennis("wta/wta-miami-usa-women-singles"),
	"WTA_RG":          tennis("wta/french-open-women-singles"),
	"WTA_WIM":         tennis("wta/wimbledon-women-singles"),
	"WTA_AO":          tennis("wta/australian-open-women-singles"),
	"WTA_USO":         tennis("wta/us-open-women-singles"),

	"CS2":      {sport: sportCounterStrike},
	"DOTA2":    {sport: sportDota},
	"LOL":      {sport: sportLeagueOfLegends},
	"VALORANT": {sport: sportValorant},
}

func leaguesWithOverrides(overrides map[string]string) map[string]leagueRef {
	refs := maps.Clone(defaultLeagues)
	for code, league := range overrides {
		if ref, ok := refs[code]; ok {
			ref.league = league
			refs[code] = ref
		}
	}
	return refs
}

func splitLeagueKey(key string) (regionID, leagueID string) {
	regionID, leagueID, _ = strings.Cut(key, "/")
	return regionID, leagueID
}
