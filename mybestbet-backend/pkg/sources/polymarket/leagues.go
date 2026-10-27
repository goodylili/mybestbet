package polymarket

import (
	"maps"
	"strings"

	"mybestbet/pkg/canon"
)

const tournamentSeparator = ":"

type venueLeague struct {
	sportCode  string
	tournament string
	sport      canon.Sport
}

func footballLeague(sportCode string) venueLeague {
	return venueLeague{sportCode: sportCode, sport: canon.SportFootball}
}

func basketballLeague(sportCode string) venueLeague {
	return venueLeague{sportCode: sportCode, sport: canon.SportBasketball}
}

func gridironLeague(sportCode string) venueLeague {
	return venueLeague{sportCode: sportCode, sport: canon.SportAmericanFootball}
}

func esportsLeague(sportCode string) venueLeague {
	return venueLeague{sportCode: sportCode, sport: canon.SportEsports}
}

func tennisTournament(tour, tournament string) venueLeague {
	return venueLeague{sportCode: tour, tournament: tournament, sport: canon.SportTennis}
}

var defaultLeagues = map[string]venueLeague{
	"WC":           footballLeague("fifwc"),
	"FRIENDLY":     footballLeague("fif"),
	"UCL":          footballLeague("ucl"),
	"UEL":          footballLeague("uel"),
	"UECL":         footballLeague("col"),
	"UWCL":         footballLeague("uwcl"),
	"EPL":          footballLeague("epl"),
	"CHAMP":        footballLeague("elc"),
	"FACUP":        footballLeague("efa"),
	"EFLCUP":       footballLeague("efl"),
	"LALIGA":       footballLeague("lal"),
	"COPADELREY":   footballLeague("cdr"),
	"SERIEA":       footballLeague("sea"),
	"COPPAITALIA":  footballLeague("itc"),
	"BUNDES":       footballLeague("bun"),
	"DFBPOKAL":     footballLeague("dfb"),
	"LIGUE1":       footballLeague("fl1"),
	"CDF":          footballLeague("cde"),
	"ERED":         footballLeague("ere"),
	"PRIMEIRA":     footballLeague("por"),
	"SUPERLIG":     footballLeague("tur"),
	"SPFL":         footballLeague("scop"),
	"JPL":          footballLeague("bel1"),
	"MLS":          footballLeague("mls"),
	"LIGAMX":       footballLeague("mex"),
	"BRA1":         footballLeague("bra"),
	"BRA2":         footballLeague("bra2"),
	"ARG1":         footballLeague("arg"),
	"SPL":          footballLeague("spl"),
	"SSL":          footballLeague("sui"),
	"GSL":          footballLeague("gre1"),
	"UPL":          footballLeague("ukr1"),
	"RPL":          footballLeague("rus"),
	"ELITESERIEN":  footballLeague("nor"),
	"DSL":          footballLeague("den"),
	"J1":           footballLeague("jap"),
	"J2":           footballLeague("ja2"),
	"KLEAGUE":      footballLeague("kor"),
	"CSL":          footballLeague("chi"),
	"ISL":          footballLeague("isp"),
	"ALEAGUE":      footballLeague("aus"),
	"LEAGUESCUP":   footballLeague("lec"),
	"LIBERTADORES": footballLeague("lib"),
	"SUDAMERICANA": footballLeague("sud"),
	"SUPERCOPA":    footballLeague("ssc"),

	"NBA":        basketballLeague("nba"),
	"NCAAB":      basketballLeague("cbb"),
	"WNBA":       basketballLeague("wnba"),
	"EUROLEAGUE": basketballLeague("euroleague"),
	"EUROCUP":    basketballLeague("bkeurocup"),
	"ACB":        basketballLeague("bkligend"),
	"NBL":        basketballLeague("bknbl"),

	"ATP_MIAMI":       tennisTournament("atp", "Miami Open"),
	"ATP_RG":          tennisTournament("atp", "Roland Garros"),
	"ATP_WIM":         tennisTournament("atp", "Wimbledon"),
	"ATP_AO":          tennisTournament("atp", "Australian Open"),
	"ATP_USO":         tennisTournament("atp", "US Open"),
	"ATP_CH_ZADAR":    tennisTournament("atp", "Zadar"),
	"ATP_CH_MURCIA":   tennisTournament("atp", "Murcia"),
	"ATP_CH_MORELOS":  tennisTournament("atp", "Morelos"),
	"ATP_CH_ASUNCION": tennisTournament("atp", "Asuncion"),
	"WTA_MIAMI":       tennisTournament("wta", "Miami Open"),
	"WTA_RG":          tennisTournament("wta", "Roland Garros"),
	"WTA_WIM":         tennisTournament("wta", "Wimbledon"),
	"WTA_AO":          tennisTournament("wta", "Australian Open"),
	"WTA_USO":         tennisTournament("wta", "US Open"),

	"NFL":   gridironLeague("nfl"),
	"NCAAF": gridironLeague("cfb"),

	"CS2":      esportsLeague("cs2"),
	"DOTA2":    esportsLeague("dota2"),
	"VALORANT": esportsLeague("val"),
	"LOL":      esportsLeague("lol"),
}

func leaguesWithOverrides(overrides map[string]string) map[string]venueLeague {
	leagues := maps.Clone(defaultLeagues)
	for code, override := range overrides {
		league, known := leagues[code]
		if !known {
			continue
		}
		league.sportCode, league.tournament, _ = strings.Cut(override, tournamentSeparator)
		leagues[code] = league
	}
	return leagues
}

func sportsBySportCode(leagues map[string]venueLeague) map[string]canon.Sport {
	sports := make(map[string]canon.Sport, len(leagues))
	for _, league := range leagues {
		sports[league.sportCode] = league.sport
	}
	return sports
}

func (l venueLeague) lists(ev gammaEvent) bool {
	return l.tournament == "" || strings.EqualFold(tournamentOf(ev), l.tournament)
}

var tourSuffixes = []string{" ATP", " WTA"}

func tournamentOf(ev gammaEvent) string {
	name := ev.Metadata.League
	if name == "" {
		name, _, _ = strings.Cut(ev.Title, tournamentSeparator)
	}
	name = strings.TrimSpace(name)
	for _, suffix := range tourSuffixes {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

func sportCodeOfSlug(slug string) string {
	code, _, _ := strings.Cut(slug, "-")
	return code
}
