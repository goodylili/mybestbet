package predictfun

import (
	"net/url"

	"mybestbet/pkg/sources"
)

const marketWebURL = "https://predict.fun/market/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	if ref.SourceEventID == "" {
		return ""
	}
	return marketWebURL + url.PathEscape(ref.SourceEventID)
}
