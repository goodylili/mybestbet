package betpawa

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
	return origin + "/event/" + url.PathEscape(ref.SourceEventID)
}
