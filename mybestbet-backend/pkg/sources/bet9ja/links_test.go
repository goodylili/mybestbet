package bet9ja_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "bet9ja", sources.Config{}, []sourcestest.LinkCase{
		{Name: "premier league match", Ref: sourcestest.EPLMatch("852457626"), Want: "https://sports.bet9ja.com/event/852457626"},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
