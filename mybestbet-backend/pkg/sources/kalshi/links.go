package kalshi

import (
	"net/url"
	"strings"

	"mybestbet/pkg/sources"
)

const marketsWebURL = "https://kalshi.com/markets/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	ticker := strings.ToLower(ref.SourceEventID)
	series, _, ok := strings.Cut(ticker, "-")
	if !ok || series == "" {
		return ""
	}
	return marketsWebURL + url.PathEscape(series) + "/" + ref.Matchup("-vs-") + "/" + url.PathEscape(ticker)
}
