package predictfun

import (
	"maps"
	"slices"
	"strings"

	"mybestbet/pkg/canon"
)

type leagueRoute struct {
	key   string
	sport canon.Sport
}

const tournamentSeparator = "/"

func footballLeague(key string) leagueRoute   { return leagueRoute{key, canon.SportFootball} }
func basketballLeague(key string) leagueRoute { return leagueRoute{key, canon.SportBasketball} }
func gridironLeague(key string) leagueRoute   { return leagueRoute{key, canon.SportAmericanFootball} }
func tennisLeague(key string) leagueRoute     { return leagueRoute{key, canon.SportTennis} }
func esportsLeague(key string) leagueRoute    { return leagueRoute{key, canon.SportEsports} }

var defaultLeagueRoutes = map[string]leagueRoute{
	"WC":              footballLeague("fifwc"),
	"FRIENDLY":        footballLeague("fif"),
	"UCL":             footballLeague("ucl"),
	"UEL":             footballLeague("uel"),
	"UWCL":            footballLeague("uwcl"),
	"EPL":             footballLeague("epl"),
	"CHAMP":           footballLeague("elc"),
	"FACUP":           footballLeague("efa"),
	"EFLCUP":          footballLeague("efl"),
	"LALIGA":          footballLeague("lal"),
	"COPADELREY":      footballLeague("cdr"),
	"SERIEA":          footballLeague("sea"),
	"COPPAITALIA":     footballLeague("itc"),
	"BUNDES":          footballLeague("bun"),
	"DFBPOKAL":        footballLeague("dfb"),
	"LIGUE1":          footballLeague("fl1"),
	"CDF":             footballLeague("cde"),
	"ERED":            footballLeague("ere"),
	"PRIMEIRA":        footballLeague("por"),
	"SUPERLIG":        footballLeague("tur"),
	"SPFL":            footballLeague("scop"),
	"JPL":             footballLeague("bel1"),
	"MLS":             footballLeague("mls"),
	"LIGAMX":          footballLeague("mex"),
	"BRA1":            footballLeague("bra"),
	"BRA2":            footballLeague("bra2"),
	"ARG1":            footballLeague("arg"),
	"SPL":             footballLeague("spl"),
	"SSL":             footballLeague("sui"),
	"GSL":             footballLeague("gre1"),
	"UPL":             footballLeague("ukr1"),
	"RPL":             footballLeague("rus"),
	"ELITESERIEN":     footballLeague("nor"),
	"DSL":             footballLeague("den"),
	"J1":              footballLeague("jap"),
	"J2":              footballLeague("ja2"),
	"KLEAGUE":         footballLeague("kor"),
	"CSL":             footballLeague("chi"),
	"ISL":             footballLeague("isp"),
	"ALEAGUE":         footballLeague("aus"),
	"LEAGUESCUP":      footballLeague("lec"),
	"LIBERTADORES":    footballLeague("lib"),
	"SUDAMERICANA":    footballLeague("sud"),
	"SUPERCOPA":       footballLeague("ssc"),
	"NBA":             basketballLeague("nba"),
	"NCAAB":           basketballLeague("cbb"),
	"WNBA":            basketballLeague("wnba"),
	"EUROLEAGUE":      basketballLeague("euroleague"),
	"EUROCUP":         basketballLeague("bkeurocup"),
	"NBL":             basketballLeague("bknbl"),
	"NFL":             gridironLeague("nfl"),
	"NCAAF":           gridironLeague("cfb"),
	"ATP_MIAMI":       tennisLeague("atp/miami open"),
	"ATP_RG":          tennisLeague("atp/roland garros"),
	"ATP_WIM":         tennisLeague("atp/wimbledon"),
	"ATP_AO":          tennisLeague("atp/australian open"),
	"ATP_USO":         tennisLeague("atp/us open"),
	"ATP_CH_ZADAR":    tennisLeague("atp/zadar"),
	"ATP_CH_MURCIA":   tennisLeague("atp/murcia"),
	"ATP_CH_MORELOS":  tennisLeague("atp/morelos"),
	"ATP_CH_ASUNCION": tennisLeague("atp/asuncion"),
	"WTA_MIAMI":       tennisLeague("wta/miami open"),
	"WTA_RG":          tennisLeague("wta/roland garros"),
	"WTA_WIM":         tennisLeague("wta/wimbledon"),
	"WTA_AO":          tennisLeague("wta/australian open"),
	"WTA_USO":         tennisLeague("wta/us open"),
	"CS2":             esportsLeague("cs2"),
	"DOTA2":           esportsLeague("dota2"),
	"VALORANT":        esportsLeague("val"),
	"LOL":             esportsLeague("lol"),
}

type leagueTable struct {
	codeByKey     map[string]string
	sportByPrefix map[string]canon.Sport
}

func newLeagueTable(overrides map[string]string) leagueTable {
	routes := maps.Clone(defaultLeagueRoutes)
	for code, key := range overrides {
		if route, known := routes[code]; known {
			routes[code] = leagueRoute{key: strings.ToLower(key), sport: route.sport}
		}
	}
	t := leagueTable{codeByKey: map[string]string{}, sportByPrefix: map[string]canon.Sport{}}
	for code, route := range routes {
		t.codeByKey[route.key] = code
		prefix, _, _ := strings.Cut(route.key, tournamentSeparator)
		t.sportByPrefix[prefix] = route.sport
	}
	return t
}

func (t leagueTable) codes() []string {
	return slices.Sorted(maps.Values(t.codeByKey))
}

func slugPrefix(slug string) string {
	prefix, _, _ := strings.Cut(slug, "-")
	return prefix
}

func (t leagueTable) sportOf(slug string) (canon.Sport, bool) {
	sport, known := t.sportByPrefix[slugPrefix(slug)]
	return sport, known
}

func (t leagueTable) leagueOf(c category) (string, bool) {
	sport, known := t.sportOf(c.Slug)
	if !known {
		return "", false
	}
	key := slugPrefix(c.Slug)
	if sport == canon.SportTennis {
		tournament, _, found := strings.Cut(c.Title, ":")
		if !found {
			return "", false
		}
		key += tournamentSeparator + strings.ToLower(strings.TrimSpace(tournament))
	}
	code, mapped := t.codeByKey[key]
	return code, mapped
}
