package bet365

import "mybestbet/pkg/sources"

const sportsWebURL = "https://www.bet365.com/#/HO/"

var _ sources.EventLinker = (*Adapter)(nil)

func (a *Adapter) EventURL(ref sources.EventRef) string {
	if ref.SourceEventID == "" {
		return ""
	}
	return sportsWebURL
}
