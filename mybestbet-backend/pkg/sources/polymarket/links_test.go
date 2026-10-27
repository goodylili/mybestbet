package polymarket_test

import (
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "polymarket", sources.Config{}, []sourcestest.LinkCase{
		{Name: "match slug", Ref: sourcestest.EPLMatch("epl-che-mun-2026-10-31"), Want: "https://polymarket.com/event/epl-che-mun-2026-10-31"},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
