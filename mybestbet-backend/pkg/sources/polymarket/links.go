package polymarket

import (
	"net/url"

	"mybestbet/pkg/sources"
)

const eventWebURL = "https://polymarket.com/event/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	if ref.SourceEventID == "" {
		return ""
	}
	return eventWebURL + url.PathEscape(ref.SourceEventID)
}
