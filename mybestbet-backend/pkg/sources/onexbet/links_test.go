package onexbet_test

import (
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	cup := sources.EventRef{
		SourceEventID: "765000001",
		Sport:         canon.SportFootball,
		League:        "FACUP",
		Country:       "England",
		LeagueName:    "FA Cup",
		Home:          "Arsenal",
		Away:          "Brighton & Hove Albion",
	}
	sourcestest.AssertEventURLs(t, "22bet", sources.Config{}, []sourcestest.LinkCase{
		{
			Name: "fixed championship",
			Ref:  sourcestest.EPLMatch("762548030"),
			Want: "https://22bet.ng/en/line/football/88637-england-premier-league/762548030-chelsea-manchester-united",
		},
		{
			Name: "championship found by name has no fixed id",
			Ref:  cup,
			Want: "https://22bet.ng/en/line/football/0-england-fa-cup/765000001-arsenal-brighton-hove-albion",
		},
		{
			Name: "championship id stored from the feed",
			Ref:  func() sources.EventRef { ref := cup; ref.SourceLeagueID = "2911523"; return ref }(),
			Want: "https://22bet.ng/en/line/football/2911523-england-fa-cup/765000001-arsenal-brighton-hove-albion",
		},
		{Name: "unknown league", Ref: sources.EventRef{SourceEventID: "1", League: "NOPE"}, Want: ""},
	})
	sourcestest.AssertEventURLs(t, "onexbet", sources.Config{Extra: map[string]string{"webUrl": "https://1xbet.example"}}, []sourcestest.LinkCase{
		{
			Name: "web url override",
			Ref:  sourcestest.EPLMatch("762548030"),
			Want: "https://1xbet.example/en/line/football/88637-england-premier-league/762548030-chelsea-manchester-united",
		},
	})
}
