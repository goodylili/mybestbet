package betway_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "betway", sources.Config{}, []sourcestest.LinkCase{
		{Name: "premier league match", Ref: sourcestest.EPLMatch("72221358"), Want: "https://www.betway.com.ng/event/soccer/england/premier-league/chelsea-manchester-united?eventId=72221358"},
		{Name: "unknown league", Ref: sources.EventRef{SourceEventID: "1", League: "NOPE"}, Want: ""},
	})
}
