package betpawa_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "betpawa", sources.Config{BaseURL: "https://www.betpawa.ng/api/sportsbook/v4"}, []sourcestest.LinkCase{
		{Name: "premier league match", Ref: sourcestest.EPLMatch("38781890"), Want: "https://www.betpawa.ng/event/38781890"},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
