package bayse

import (
	"net/url"
	"strings"

	"mybestbet/pkg/sources"
)

const (
	webURL         = "https://bayse.markets"
	gameSlugPrefix = "bm-game-"
)

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	switch {
	case ref.SourceEventID == "":
		return ""
	case strings.HasPrefix(ref.SourceEventID, gameSlugPrefix):
		return ""
	default:
		return marketURL(ref.SourceEventID)
	}
}

func marketURL(slug string) string {
	if slug == "" {
		return ""
	}
	return webURL + "/market/" + url.PathEscape(slug)
}
