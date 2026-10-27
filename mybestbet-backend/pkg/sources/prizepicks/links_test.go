package prizepicks_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "prizepicks", sources.Config{}, []sourcestest.LinkCase{
		{Name: "board fallback", Ref: sourcestest.EPLMatch("g1"), Want: "https://app.prizepicks.com/"},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
