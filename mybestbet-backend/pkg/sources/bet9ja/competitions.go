package bet9ja

import (
	"maps"
	"slices"
	"strings"
)

type competition struct {
	groupID string
	byName  *groupName
}

type groupName struct {
	sport    string
	category string
	group    string
}

const (
	soccer           = "Soccer"
	basketball       = "Basketball"
	tennis           = "Tennis"
	americanFootball = "American Football"
)

func fixedGroup(id string) competition { return competition{groupID: id} }

func namedGroup(sport, category, group string) competition {
	return competition{byName: &groupName{sport: sport, category: category, group: group}}
}

var defaultCompetitions = map[string]competition{
	"EPL":      fixedGroup("170880"),
	"CHAMP":    fixedGroup("170881"),
	"LALIGA":   fixedGroup("180928"),
	"SERIEA":   fixedGroup("167856"),
	"BUNDES":   fixedGroup("180923"),
	"LIGUE1":   fixedGroup("950503"),
	"ERED":     fixedGroup("1016657"),
	"PRIMEIRA": fixedGroup("180967"),
	"SPL":      fixedGroup("971216"),
	"MLS":      fixedGroup("1387346"),

	"WC":           namedGroup(soccer, "International", "World Cup"),
	"FRIENDLY":     namedGroup(soccer, "International", "Friendlies"),
	"UCL":          namedGroup(soccer, "International Clubs", "UEFA Champions League"),
	"UEL":          namedGroup(soccer, "International Clubs", "UEFA Europa League"),
	"UECL":         namedGroup(soccer, "International Clubs", "UEFA Conference League"),
	"UWCL":         namedGroup(soccer, "International Clubs", "UEFA Champions League, Women"),
	"LIBERTADORES": namedGroup(soccer, "International Clubs", "Copa Libertadores"),
	"SUDAMERICANA": namedGroup(soccer, "International Clubs", "Copa Sudamericana"),
	"LEAGUESCUP":   namedGroup(soccer, "International Clubs", "Leagues Cup"),
	"FACUP":        namedGroup(soccer, "England", "FA Cup"),
	"EFLCUP":       namedGroup(soccer, "England", "EFL Cup"),
	"COPADELREY":   namedGroup(soccer, "Spain", "Copa del Rey"),
	"SUPERCOPA":    namedGroup(soccer, "Spain", "Supercopa"),
	"COPPAITALIA":  namedGroup(soccer, "Italy", "Coppa Italia"),
	"DFBPOKAL":     namedGroup(soccer, "Germany", "DFB Pokal"),
	"CDF":          namedGroup(soccer, "France", "Coupe de France"),
	"SUPERLIG":     namedGroup(soccer, "Turkey", "Super Lig"),
	"SPFL":         namedGroup(soccer, "Scotland", "Premiership"),
	"JPL":          namedGroup(soccer, "Belgium", "Pro League"),
	"LIGAMX":       namedGroup(soccer, "Mexico", "Liga MX"),
	"BRA1":         namedGroup(soccer, "Brazil", "Brasileiro Serie A"),
	"BRA2":         namedGroup(soccer, "Brazil", "Brasileiro Serie B"),
	"ARG1":         namedGroup(soccer, "Argentina", "Primera LPF"),
	"SSL":          namedGroup(soccer, "Switzerland", "Super League"),
	"GSL":          namedGroup(soccer, "Greece", "Super League 1"),
	"UPL":          namedGroup(soccer, "Ukraine", "Premier League"),
	"RPL":          namedGroup(soccer, "Russia", "Premier League"),
	"ELITESERIEN":  namedGroup(soccer, "Norway", "Eliteserien"),
	"DSL":          namedGroup(soccer, "Denmark", "Superligaen"),
	"J1":           namedGroup(soccer, "Japan", "J.League"),
	"J2":           namedGroup(soccer, "Japan", "J.League 2"),
	"KLEAGUE":      namedGroup(soccer, "South Korea", "K-League 1"),
	"CSL":          namedGroup(soccer, "China", "Super League"),
	"ISL":          namedGroup(soccer, "India", "Indian Super League"),
	"ALEAGUE":      namedGroup(soccer, "Australia", "A-League"),

	"NBA":        namedGroup(basketball, "USA", "NBA"),
	"NCAAB":      namedGroup(basketball, "USA", "NCAA"),
	"WNBA":       namedGroup(basketball, "USA", "WNBA"),
	"EUROLEAGUE": namedGroup(basketball, "International", "Euroleague"),
	"EUROCUP":    namedGroup(basketball, "International", "Eurocup"),
	"ACB":        namedGroup(basketball, "Spain", "Liga ACB"),
	"NBL":        namedGroup(basketball, "Australia", "NBL"),

	"NFL":   namedGroup(americanFootball, "USA", "NFL"),
	"NCAAF": namedGroup(americanFootball, "USA", "NCAA"),

	"ATP_MIAMI":       namedGroup(tennis, "ATP", "ATP Miami"),
	"ATP_RG":          namedGroup(tennis, "ATP", "ATP French Open"),
	"ATP_WIM":         namedGroup(tennis, "ATP", "ATP Wimbledon"),
	"ATP_AO":          namedGroup(tennis, "ATP", "ATP Australian Open"),
	"ATP_USO":         namedGroup(tennis, "ATP", "ATP US Open"),
	"ATP_CH_ZADAR":    namedGroup(tennis, "Challenger", "ATP Challenger Zadar"),
	"ATP_CH_MURCIA":   namedGroup(tennis, "Challenger", "ATP Challenger Murcia"),
	"ATP_CH_MORELOS":  namedGroup(tennis, "Challenger", "ATP Challenger Morelos"),
	"ATP_CH_ASUNCION": namedGroup(tennis, "Challenger", "ATP Challenger Asuncion"),
	"WTA_MIAMI":       namedGroup(tennis, "WTA", "WTA Miami"),
	"WTA_RG":          namedGroup(tennis, "WTA", "WTA French Open"),
	"WTA_WIM":         namedGroup(tennis, "WTA", "WTA Wimbledon"),
	"WTA_AO":          namedGroup(tennis, "WTA", "WTA Australian Open"),
	"WTA_USO":         namedGroup(tennis, "WTA", "WTA US Open"),
}

func competitionsWithOverrides(leagueIDs map[string]string) map[string]competition {
	out := maps.Clone(defaultCompetitions)
	for code, groupID := range leagueIDs {
		out[code] = fixedGroup(groupID)
	}
	return out
}

func (t sportsTree) groupIDsNamed(name groupName) []string {
	var ids []string
	for _, sport := range t.Sports {
		if sport.Name != name.sport {
			continue
		}
		for _, category := range sport.Categories {
			if category.Name != name.category {
				continue
			}
			for _, id := range slices.Sorted(maps.Keys(category.Groups)) {
				if name.matches(category.Groups[id].Name) {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

const (
	stageSeparator = ", "
	womenStage     = "Women"
	doublesMarker  = "Doubles"
)

func (n groupName) matches(desc string) bool {
	if desc == n.group {
		return true
	}
	stage, ok := strings.CutPrefix(desc, n.group+stageSeparator)
	if !ok {
		return false
	}
	return !strings.Contains(stage, ",") && stage != womenStage && !strings.Contains(stage, doublesMarker)
}
