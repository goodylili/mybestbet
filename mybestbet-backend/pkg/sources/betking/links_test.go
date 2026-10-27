package betking_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "betking", sources.Config{}, []sourcestest.LinkCase{
		{Name: "premier league match", Ref: sourcestest.EPLMatch("1005767843"), Want: "https://m.betking.com/en-ng/sports/prematch/1005767843/chelsea-vs-manchester-united"},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
