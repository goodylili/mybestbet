package onexbet

import (
	"cmp"
	"net/url"

	"mybestbet/pkg/sources"
)

const unknownChampionshipID = "0"

var webSportSlugs = map[int]string{
	footballSportID:         "football",
	basketballSportID:       "basketball",
	tennisSportID:           "tennis",
	americanFootballSportID: "american-football",
	esportsSportID:          "esports",
}

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	if ref.SourceEventID == "" {
		return ""
	}
	c, ok := a.competitions[ref.League]
	if !ok {
		return ""
	}
	championship := cmp.Or(ref.SourceLeagueID, c.championshipID, unknownChampionshipID) + "-" + sources.Slug(ref.LeagueTitle(), "-")
	game := url.PathEscape(ref.SourceEventID) + "-" + ref.Matchup("-")
	return a.webURL + "/en/line/" + webSportSlugs[c.sportID] + "/" + championship + "/" + game
}
