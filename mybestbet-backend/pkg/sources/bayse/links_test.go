package bayse_test

import (
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	sourcestest.AssertEventURLs(t, "bayse", sources.Config{}, []sourcestest.LinkCase{
		{Name: "nba game market", Ref: sources.EventRef{SourceEventID: "detroit-vs-new-york-october-28-2026", Sport: canon.SportBasketball, League: "NBA"}, Want: "https://bayse.markets/market/detroit-vs-new-york-october-28-2026"},
		{Name: "football game links through its markets instead", Ref: sourcestest.EPLMatch("bm-game-20261031-che-mun"), Want: ""},
		{Name: "no event id", Ref: sources.EventRef{League: "EPL"}, Want: ""},
	})
}
