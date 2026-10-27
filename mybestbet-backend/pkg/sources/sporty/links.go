package sporty

import (
	"net/url"
	"path"
	"strings"

	"mybestbet/pkg/sources"
)

var eventPathPrefixes = map[string]string{
	SportyBet:   "/sport/",
	FootballCom: "/m/sport/",
}

var webSportNames = map[string]string{
	footballSportID:         "football",
	basketballSportID:       "basketball",
	tennisSportID:           "tennis",
	americanFootballSportID: "americanFootball",
	counterStrikeSportID:    "counterStrike",
	leagueOfLegendsSportID:  "leagueOfLegends",
	dotaSportID:             "dota2",
	valorantSportID:         "valorant",
}

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	c, ok := a.competitions[ref.League]
	if !ok || ref.SourceEventID == "" {
		return ""
	}
	base, err := url.Parse(a.deps.Config.BaseURL)
	if err != nil || base.Host == "" {
		return ""
	}
	segments := []string{
		webSportNames[c.sportID],
		titleSegment(ref.Country, "International"),
		titleSegment(ref.LeagueName, ref.League),
		titleSegment(ref.Home, "Home") + "_vs_" + titleSegment(ref.Away, "Away"),
		ref.SourceEventID,
	}
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return base.Scheme + "://" + base.Host + "/" + path.Base(base.Path) + eventPathPrefixes[a.name] + strings.Join(segments, "/")
}

func titleSegment(s, fallback string) string {
	if words := sources.SlugWords(s); len(words) > 0 {
		return strings.Join(words, "_")
	}
	return fallback
}
