package betking

import (
	"net/url"

	"mybestbet/pkg/sources"
)

const eventWebURL = "https://m.betking.com/en-ng/sports/prematch/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	if ref.SourceEventID == "" {
		return ""
	}
	return eventWebURL + url.PathEscape(ref.SourceEventID) + "/" + ref.Matchup("-vs-")
}
