package flashscore

import (
	"regexp"
	"strings"
)

const (
	tennisPathPrefix  = "/tennis/"
	slugWordSeparator = "-"
	codeWordSeparator = "_"
)

var Tournaments = map[string]string{
	"ATP_MIAMI":       "atp-singles/miami",
	"ATP_RG":          "atp-singles/french-open",
	"ATP_WIM":         "atp-singles/wimbledon",
	"ATP_AO":          "atp-singles/australian-open",
	"ATP_USO":         "atp-singles/us-open",
	"ATP_CH_ZADAR":    "challenger-men-singles/zadar",
	"ATP_CH_MURCIA":   "challenger-men-singles/murcia",
	"ATP_CH_MORELOS":  "challenger-men-singles/morelos",
	"ATP_CH_ASUNCION": "challenger-men-singles/asuncion",
	"WTA_MIAMI":       "wta-singles/miami",
	"WTA_RG":          "wta-singles/french-open",
	"WTA_WIM":         "wta-singles/wimbledon",
	"WTA_AO":          "wta-singles/australian-open",
	"WTA_USO":         "wta-singles/us-open",
}

type tour struct {
	pathPrefix string
	codePrefix string
	namePrefix string
}

var tours = []tour{
	{pathPrefix: "atp-singles/", codePrefix: "ATP_", namePrefix: "ATP "},
	{pathPrefix: "wta-singles/", codePrefix: "WTA_", namePrefix: "WTA "},
	{pathPrefix: "challenger-men-singles/", codePrefix: "ATP_CH_", namePrefix: "ATP Challenger "},
	{pathPrefix: "challenger-women-singles/", codePrefix: "WTA_CH_", namePrefix: "WTA Challenger "},
}

var sectionVenue = regexp.MustCompile(`^[^:]*:\s*(.+?)\s*\(([^()]+)\)`)

var leagueByTournamentPath = invert(Tournaments)

type Tournament struct {
	Code    string
	Name    string
	Country string
}

func invert(m map[string]string) map[string]string {
	inverted := make(map[string]string, len(m))
	for k, v := range m {
		inverted[v] = k
	}
	return inverted
}

func tournamentOfSection(header, sectionPath string) (Tournament, bool) {
	path := strings.Trim(strings.TrimPrefix(sectionPath, tennisPathPrefix), "/")
	for _, t := range tours {
		slug, isTour := strings.CutPrefix(path, t.pathPrefix)
		if !isTour || slug == "" || strings.Contains(slug, "/") {
			continue
		}
		code, configured := leagueByTournamentPath[path]
		if !configured {
			code = t.codePrefix + strings.ToUpper(strings.ReplaceAll(slug, slugWordSeparator, codeWordSeparator))
		}
		city, country := venueOf(header, slug)
		return Tournament{Code: code, Name: t.namePrefix + city, Country: country}, true
	}
	return Tournament{}, false
}

func venueOf(header, slug string) (city, country string) {
	if g := sectionVenue.FindStringSubmatch(header); g != nil {
		return g[1], strings.TrimSpace(g[2])
	}
	return titleWords(strings.Split(slug, slugWordSeparator)), ""
}
