package bet365_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "bet365", sources.Config{}, []sourcestest.LinkCase{
		{Name: "sports home fallback", Ref: sourcestest.EPLMatch("E123"), Want: "https://www.bet365.com/#/HO/"},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
