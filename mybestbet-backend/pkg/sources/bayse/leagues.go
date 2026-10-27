package bayse

import (
	"maps"
	"slices"
)

var defaultGameLeagueNames = map[string]string{
	"EPL":         "England - Premier League",
	"FACUP":       "England - FA Cup",
	"EFLCUP":      "England - EFL Cup",
	"LALIGA":      "Spain - La Liga",
	"COPADELREY":  "Spain - Copa del Rey",
	"SUPERCOPA":   "Spain - Supercopa",
	"SERIEA":      "Italy - Serie A",
	"COPPAITALIA": "Italy - Coppa Italia",
	"BUNDES":      "Germany - Bundesliga",
	"DFBPOKAL":    "Germany - DFB Pokal",
	"LIGUE1":      "France - Ligue 1",
	"CDF":         "France - Coupe de France",
	"UCL":         "UEFA - Champions League",
	"UEL":         "UEFA - Europa League",
	"UECL":        "UEFA - Europa Conference League",
	"SPL":         "Saudi Arabia - Saudi League",
	"MLS":         "USA - Major League Soccer",
	"LEAGUESCUP":  "North America - Leagues Cup",
	"WC":          "FIFA - World Cup",
	"FRIENDLY":    "International - Friendlies",
}

var defaultSubcategories = map[string]string{
	"NBA": "NBA",
}

type leagueDirectory struct {
	gameLeagues   map[string]string
	subcategories map[string]string
}

func leagueDirectoryWithOverrides(overrides map[string]string) leagueDirectory {
	dir := leagueDirectory{gameLeagues: maps.Clone(defaultGameLeagueNames), subcategories: maps.Clone(defaultSubcategories)}
	for code, sourceName := range overrides {
		if _, isSubcategory := dir.subcategories[code]; isSubcategory {
			dir.subcategories[code] = sourceName
			continue
		}
		dir.gameLeagues[code] = sourceName
	}
	return dir
}

func (d leagueDirectory) codes() []string {
	all := maps.Clone(d.gameLeagues)
	maps.Copy(all, d.subcategories)
	return slices.Sorted(maps.Keys(all))
}
