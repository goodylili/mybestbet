package betway

import (
	"net/url"
	"strings"

	"mybestbet/pkg/sources"
)

const eventWebURL = "https://www.betway.com.ng/event/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	l, ok := a.leagues[ref.League]
	if !ok || ref.SourceEventID == "" {
		return ""
	}
	regionLeague := l.league
	if !strings.Contains(regionLeague, "/") {
		regionLeague = sources.Slug(ref.Country, "-") + "/" + sources.Slug(ref.LeagueName, "-")
	}
	return eventWebURL + string(l.sport) + "/" + regionLeague + "/" + ref.Matchup("-") + "?eventId=" + url.QueryEscape(ref.SourceEventID)
}
