package betking

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

type sportID int

const (
	sportFootball         sportID = 1
	sportBasketball       sportID = 2
	sportTennis           sportID = 5
	sportAmericanFootball sportID = 16
)

type tournamentRef struct {
	sport    sportID
	category string
	names    []string
	id       string
}

func (r tournamentRef) matches(category string, t menuTournament) bool {
	if r.id != "" {
		return strconv.FormatInt(t.ItemID, 10) == r.id
	}
	if !strings.EqualFold(strings.TrimSpace(category), r.category) {
		return false
	}
	name := strings.TrimSpace(t.ItemName)
	return slices.ContainsFunc(r.names, func(want string) bool { return strings.EqualFold(name, want) })
}

func football(category string, names ...string) tournamentRef {
	return tournamentRef{sport: sportFootball, category: category, names: names}
}

func basketball(category string, names ...string) tournamentRef {
	return tournamentRef{sport: sportBasketball, category: category, names: names}
}

func americanFootball(category string, names ...string) tournamentRef {
	return tournamentRef{sport: sportAmericanFootball, category: category, names: names}
}

func tennis(category string, names ...string) tournamentRef {
	return tournamentRef{sport: sportTennis, category: category, names: names}
}

var defaultTournaments = map[string]tournamentRef{
	"WC":           football("International", "World Cup"),
	"FRIENDLY":     football("International", "Int. Friendly Games", "International Friendlies"),
	"UCL":          football("UEFA", "Champions League"),
	"UEL":          football("UEFA", "Europa League"),
	"UECL":         football("UEFA", "Conference League"),
	"UWCL":         football("UEFA", "Champions League, Women", "Champions League Women"),
	"EPL":          football("England", "Premier League"),
	"CHAMP":        football("England", "Championship"),
	"FACUP":        football("England", "FA Cup"),
	"EFLCUP":       football("England", "EFL Cup"),
	"LALIGA":       football("Spain", "LaLiga"),
	"COPADELREY":   football("Spain", "Copa del Rey"),
	"SUPERCOPA":    football("Spain", "Supercopa", "Super Cup"),
	"SERIEA":       football("Italy", "Serie A"),
	"COPPAITALIA":  football("Italy", "Coppa Italia"),
	"BUNDES":       football("Germany", "Bundesliga"),
	"DFBPOKAL":     football("Germany", "DFB Pokal"),
	"LIGUE1":       football("France", "Ligue 1"),
	"CDF":          football("France", "Coupe de France"),
	"ERED":         football("Netherlands", "Eredivisie"),
	"PRIMEIRA":     football("Portugal", "Primeira Liga"),
	"SUPERLIG":     football("Türkiye", "Super Lig"),
	"SPFL":         football("Scotland", "Premiership"),
	"JPL":          football("Belgium", "Pro League"),
	"MLS":          football("USA", "Major League Soccer"),
	"LIGAMX":       football("Mexico", "Liga MX, Apertura", "Liga MX, Clausura"),
	"BRA1":         football("Brazil", "Brasileiro A"),
	"BRA2":         football("Brazil", "Brasileiro B"),
	"ARG1":         football("Argentina", "Liga Profesional"),
	"SPL":          football("Saudi Arabia", "Professional League"),
	"SSL":          football("Switzerland", "Super League"),
	"GSL":          football("Greece", "Super League"),
	"UPL":          football("Ukraine", "Premier League"),
	"RPL":          football("Russia", "Premier League"),
	"ELITESERIEN":  football("Norway", "Eliteserien"),
	"DSL":          football("Denmark", "Superligaen"),
	"J1":           football("Japan", "J.League"),
	"J2":           football("Japan", "J.League 2", "J2/J3 League"),
	"KLEAGUE":      football("South Korea", "K-League 1"),
	"CSL":          football("China", "Super League"),
	"ISL":          football("India", "Super League"),
	"ALEAGUE":      football("Australia", "A - League"),
	"LEAGUESCUP":   football("International Club", "Leagues Cup"),
	"LIBERTADORES": football("International Club", "Copa Libertadores"),
	"SUDAMERICANA": football("International Club", "Copa Sudamericana"),

	"NBA":        basketball("USA", "NBA"),
	"NCAAB":      basketball("USA", "NCAA"),
	"WNBA":       basketball("USA", "WNBA"),
	"EUROLEAGUE": basketball("Euroleague", "Euroleague"),
	"EUROCUP":    basketball("Eurocup", "Eurocup"),
	"ACB":        basketball("Spain", "ACB"),
	"NBL":        basketball("Australia", "NBL"),

	"NFL":   americanFootball("USA", "NFL"),
	"NCAAF": americanFootball("USA", "NCAA"),

	"ATP_MIAMI":       tennis("ATP", "Miami"),
	"ATP_RG":          tennis("ATP", "French Open", "Roland Garros"),
	"ATP_WIM":         tennis("ATP", "Wimbledon"),
	"ATP_AO":          tennis("ATP", "Australian Open"),
	"ATP_USO":         tennis("ATP", "US Open"),
	"ATP_CH_ZADAR":    tennis("Challenger", "Zadar"),
	"ATP_CH_MURCIA":   tennis("Challenger", "Murcia"),
	"ATP_CH_MORELOS":  tennis("Challenger", "Morelos"),
	"ATP_CH_ASUNCION": tennis("Challenger", "Asuncion"),
	"WTA_MIAMI":       tennis("WTA", "Miami"),
	"WTA_RG":          tennis("WTA", "French Open", "Roland Garros"),
	"WTA_WIM":         tennis("WTA", "Wimbledon"),
	"WTA_AO":          tennis("WTA", "Australian Open"),
	"WTA_USO":         tennis("WTA", "US Open"),
}

func tournamentsWithOverrides(overrides map[string]string) map[string]tournamentRef {
	refs := maps.Clone(defaultTournaments)
	for code, id := range overrides {
		if ref, ok := refs[code]; ok {
			ref.id = id
			refs[code] = ref
		}
	}
	return refs
}
