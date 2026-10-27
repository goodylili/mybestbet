package betano_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "betano", sources.Config{BaseURL: "https://www.betano.ng"}, []sourcestest.LinkCase{
		{Name: "premier league match", Ref: sourcestest.EPLMatch("74510237"), Want: "https://www.betano.ng/odds/chelsea-manchester-united/74510237/"},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
