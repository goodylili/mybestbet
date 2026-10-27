package betpawa

import (
	"maps"
	"slices"
	"strings"
)

type categoryID string

const (
	categoryFootball   categoryID = "2"
	categoryBasketball categoryID = "3"
	categoryTennis     categoryID = "452"
)

const internationalRegion = "International"

type competitionRef struct {
	category categoryID
	region   string
	names    []string
	id       string
}

func (r competitionRef) matches(region string, c competition) bool {
	if r.id != "" {
		return c.ID == r.id
	}
	if !strings.EqualFold(strings.TrimSpace(region), r.region) {
		return false
	}
	name := strings.TrimSpace(c.Name)
	return slices.ContainsFunc(r.names, func(want string) bool { return strings.EqualFold(name, want) })
}

func football(region string, names ...string) competitionRef {
	return competitionRef{category: categoryFootball, region: region, names: names}
}

func basketball(region string, names ...string) competitionRef {
	return competitionRef{category: categoryBasketball, region: region, names: names}
}

func tennis(names ...string) competitionRef {
	return competitionRef{category: categoryTennis, region: internationalRegion, names: names}
}

var defaultCompetitions = map[string]competitionRef{
	"WC":           football(internationalRegion, "World Cup"),
	"FRIENDLY":     football(internationalRegion, "Int. Friendly Games"),
	"UCL":          football(internationalRegion, "UEFA Champions League"),
	"UEL":          football(internationalRegion, "UEFA Europa League"),
	"UECL":         football(internationalRegion, "UEFA Conference League"),
	"UWCL":         football(internationalRegion, "UEFA Champions League, Women", "UEFA Champions League Women"),
	"EPL":          football("England", "Premier League"),
	"CHAMP":        football("England", "Championship"),
	"FACUP":        football("England", "FA Cup"),
	"EFLCUP":       football("England", "EFL Cup"),
	"LALIGA":       football("Spain", "LaLiga"),
	"COPADELREY":   football("Spain", "Copa del Rey"),
	"SUPERCOPA":    football("Spain", "Supercopa", "Super Cup", "Supercopa de Espana"),
	"SERIEA":       football("Italy", "Serie A"),
	"COPPAITALIA":  football("Italy", "Coppa Italia"),
	"BUNDES":       football("Germany", "Bundesliga"),
	"DFBPOKAL":     football("Germany", "DFB Pokal"),
	"LIGUE1":       football("France", "Ligue 1"),
	"CDF":          football("France", "Coupe de France"),
	"ERED":         football("Netherlands", "Eredivisie"),
	"PRIMEIRA":     football("Portugal", "Liga Portugal"),
	"SUPERLIG":     football("Turkey", "Super Lig"),
	"SPFL":         football("Scotland", "Premiership"),
	"JPL":          football("Belgium", "Pro League"),
	"MLS":          football("USA", "Major League Soccer"),
	"LIGAMX":       football("Mexico", "Liga MX, Apertura", "Liga MX, Clausura"),
	"BRA1":         football("Brazil", "Brasileiro Serie A"),
	"BRA2":         football("Brazil", "Brasileiro Serie B"),
	"ARG1":         football("Argentina", "Liga Profesional"),
	"SPL":          football("Saudi Arabia", "Saudi Prof. League"),
	"SSL":          football("Switzerland", "Super League"),
	"GSL":          football("Greece", "Super League 1"),
	"UPL":          football("Ukraine", "Premier League"),
	"RPL":          football("Russian Federation", "Premier League"),
	"ELITESERIEN":  football("Norway", "Eliteserien"),
	"DSL":          football("Denmark", "Superliga"),
	"J1":           football("Japan", "J.League"),
	"J2":           football("Japan", "J.League 2"),
	"KLEAGUE":      football("Republic of Korea", "K-League 1"),
	"CSL":          football("China", "Chinese Super League"),
	"ISL":          football("India", "Indian Super League"),
	"ALEAGUE":      football("Australia", "A-League"),
	"LEAGUESCUP":   football(internationalRegion, "Leagues Cup"),
	"LIBERTADORES": football(internationalRegion, "Copa Libertadores"),
	"SUDAMERICANA": football(internationalRegion, "Copa Sudamericana"),

	"NBA":        basketball("USA", "NBA"),
	"NCAAB":      basketball("USA", "NCAA", "NCAA, Regular Season"),
	"WNBA":       basketball("USA", "WNBA"),
	"EUROLEAGUE": basketball(internationalRegion, "Euroleague"),
	"EUROCUP":    basketball(internationalRegion, "Eurocup"),
	"ACB":        basketball("Spain", "Liga ACB"),
	"NBL":        basketball("Australia", "NBL"),

	"ATP_MIAMI":       tennis("ATP Miami, USA Men Singles"),
	"ATP_RG":          tennis("French Open Men Singles", "ATP French Open, France Men Singles"),
	"ATP_WIM":         tennis("Wimbledon Men Singles", "ATP Wimbledon, Great Britain Men Singles"),
	"ATP_AO":          tennis("Australian Open Men Singles", "ATP Australian Open, Australia Men Singles"),
	"ATP_USO":         tennis("US Open Men Singles", "ATP US Open, USA Men Singles"),
	"ATP_CH_ZADAR":    tennis("ATP Challenger Zadar, Croatia Men Singles"),
	"ATP_CH_MURCIA":   tennis("ATP Challenger Murcia, Spain Men Singles"),
	"ATP_CH_MORELOS":  tennis("ATP Challenger Morelos, Mexico Men Singles"),
	"ATP_CH_ASUNCION": tennis("ATP Challenger Asuncion, Paraguay Men Singles"),
	"WTA_MIAMI":       tennis("WTA Miami, USA Women Singles"),
	"WTA_RG":          tennis("French Open Women Singles", "WTA French Open, France Women Singles"),
	"WTA_WIM":         tennis("Wimbledon Women Singles", "WTA Wimbledon, Great Britain Women Singles"),
	"WTA_AO":          tennis("Australian Open Women Singles", "WTA Australian Open, Australia Women Singles"),
	"WTA_USO":         tennis("US Open Women Singles", "WTA US Open, USA Women Singles"),
}

func competitionsWithOverrides(overrides map[string]string) map[string]competitionRef {
	refs := maps.Clone(defaultCompetitions)
	for code, id := range overrides {
		if ref, ok := refs[code]; ok {
			ref.id = id
			refs[code] = ref
		}
	}
	return refs
}
