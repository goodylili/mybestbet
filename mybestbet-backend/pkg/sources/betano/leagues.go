package betano

import "strings"

type sportSlug string

const (
	sportFootball         sportSlug = "football"
	sportBasketball       sportSlug = "basketball"
	sportTennis           sportSlug = "tennis"
	sportAmericanFootball sportSlug = "american-football"
	sportEsports          sportSlug = "esports"
)

type leagueName struct {
	sport  sportSlug
	region string
	name   string
}

func (n leagueName) matches(l league) bool {
	if !strings.EqualFold(strings.TrimSpace(l.RegionName), n.region) {
		return false
	}
	return n.coversWholeRegion() || strings.EqualFold(strings.TrimSpace(l.Name), n.name)
}

func (n leagueName) coversWholeRegion() bool { return n.name == "" }

func football(region, name string) leagueName {
	return leagueName{sport: sportFootball, region: region, name: name}
}

func basketball(region, name string) leagueName {
	return leagueName{sport: sportBasketball, region: region, name: name}
}

func tennis(region, name string) leagueName {
	return leagueName{sport: sportTennis, region: region, name: name}
}

func esportsGame(region string) leagueName {
	return leagueName{sport: sportEsports, region: region}
}

var defaultLeagueNames = map[string]leagueName{
	"WC":           football("World", "World Cup"),
	"FRIENDLY":     football("World", "International Friendlies"),
	"UCL":          football("Europe", "Champions League"),
	"UEL":          football("Europe", "Europa League"),
	"UECL":         football("Europe", "Conference League"),
	"UWCL":         football("Europe", "Women's Champions League"),
	"EPL":          football("England", "Premier League"),
	"CHAMP":        football("England", "Championship"),
	"FACUP":        football("England", "FA Cup"),
	"EFLCUP":       football("England", "EFL Cup"),
	"LALIGA":       football("Spain", "LaLiga"),
	"COPADELREY":   football("Spain", "Copa del Rey"),
	"SUPERCOPA":    football("Spain", "Super Cup"),
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
	"MLS":          football("USA", "MLS"),
	"LIGAMX":       football("Mexico", "Liga MX"),
	"BRA1":         football("Brazil", "Brasileirao Serie A"),
	"BRA2":         football("Brazil", "Brasileirao Serie B"),
	"ARG1":         football("Argentina", "Liga Profesional"),
	"SPL":          football("Saudi Arabia", "Saudi Pro League"),
	"SSL":          football("Switzerland", "Super League"),
	"GSL":          football("Greece", "Super League"),
	"UPL":          football("Ukraine", "Premier League"),
	"RPL":          football("Russia", "Premier League"),
	"ELITESERIEN":  football("Norway", "Eliteserien"),
	"DSL":          football("Denmark", "Superliga"),
	"J1":           football("Japan", "J1 League"),
	"J2":           football("Japan", "J2 League"),
	"KLEAGUE":      football("South Korea", "K League 1"),
	"CSL":          football("China", "Super League"),
	"ISL":          football("India", "Indian Super League"),
	"ALEAGUE":      football("Australia", "A-League"),
	"LEAGUESCUP":   football("North & Central America", "Leagues Cup"),
	"LIBERTADORES": football("South America", "Copa Libertadores"),
	"SUDAMERICANA": football("South America", "Copa Sudamericana"),

	"NBA":        basketball("USA", "NBA"),
	"NCAAB":      basketball("USA", "NCAA"),
	"WNBA":       basketball("USA", "WNBA"),
	"EUROLEAGUE": basketball("Europe", "Euroleague"),
	"EUROCUP":    basketball("Europe", "Eurocup"),
	"ACB":        basketball("Spain", "Liga ACB"),
	"NBL":        basketball("Australia", "NBL"),

	"NFL":   {sport: sportAmericanFootball, region: "USA", name: "NFL"},
	"NCAAF": {sport: sportAmericanFootball, region: "USA", name: "NCAA"},

	"ATP_MIAMI":       tennis("ATP", "Miami"),
	"ATP_RG":          tennis("ATP", "French Open"),
	"ATP_WIM":         tennis("ATP", "Wimbledon"),
	"ATP_AO":          tennis("ATP", "Australian Open"),
	"ATP_USO":         tennis("ATP", "US Open"),
	"ATP_CH_ZADAR":    tennis("Challenger", "Zadar"),
	"ATP_CH_MURCIA":   tennis("Challenger", "Murcia"),
	"ATP_CH_MORELOS":  tennis("Challenger", "Morelos"),
	"ATP_CH_ASUNCION": tennis("Challenger", "Asuncion"),
	"WTA_MIAMI":       tennis("WTA", "Miami"),
	"WTA_RG":          tennis("WTA", "French Open"),
	"WTA_WIM":         tennis("WTA", "Wimbledon"),
	"WTA_AO":          tennis("WTA", "Australian Open"),
	"WTA_USO":         tennis("WTA", "US Open"),

	"CS2":      esportsGame("Counter-Strike"),
	"DOTA2":    esportsGame("Dota 2"),
	"VALORANT": esportsGame("Valorant"),
	"LOL":      esportsGame("League of Legends"),
}
