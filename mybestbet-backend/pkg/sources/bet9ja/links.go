package bet9ja

import (
	"net/url"

	"mybestbet/pkg/sources"
)

const eventWebURL = "https://sports.bet9ja.com/event/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	if ref.SourceEventID == "" {
		return ""
	}
	return eventWebURL + url.PathEscape(ref.SourceEventID)
}
