package betano

import (
	"net/url"

	"mybestbet/pkg/sources"
)

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	origin := sources.Origin(a.deps.Config.BaseURL)
	if origin == "" || ref.SourceEventID == "" {
		return ""
	}
	return origin + "/odds/" + ref.Matchup("-") + "/" + url.PathEscape(ref.SourceEventID) + "/"
}
