package prizepicks

import "mybestbet/pkg/sources"

const boardWebURL = "https://app.prizepicks.com/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	if ref.SourceEventID == "" {
		return ""
	}
	return boardWebURL
}
