package sporty_test

import (
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
	"mybestbet/pkg/sources/sporty"
)

func TestEventURL(t *testing.T) {
	t.Parallel()
	laliga := sources.EventRef{
		SourceEventID: "sr:match:61301163",
		Sport:         canon.SportFootball,
		League:        "LALIGA",
		Country:       "Spain",
		LeagueName:    "LaLiga",
		Home:          "Atlético Madrid",
		Away:          "Real Betis",
	}
	sourcestest.AssertEventURLs(t, sporty.SportyBet, sources.Config{BaseURL: "https://www.sportybet.com/api/ng"}, []sourcestest.LinkCase{
		{
			Name: "premier league match",
			Ref:  sourcestest.EPLMatch("sr:match:72221358"),
			Want: "https://www.sportybet.com/ng/sport/football/England/Premier_League/Chelsea_vs_Manchester_United/sr:match:72221358",
		},
		{
			Name: "accents are folded",
			Ref:  laliga,
			Want: "https://www.sportybet.com/ng/sport/football/Spain/LaLiga/Atletico_Madrid_vs_Real_Betis/sr:match:61301163",
		},
		{Name: "league the brand does not list", Ref: sources.EventRef{SourceEventID: "sr:match:1", League: "NOPE"}, Want: ""},
	})
	sourcestest.AssertEventURLs(t, sporty.FootballCom, sources.Config{BaseURL: "https://www.football.com/api/ng"}, []sourcestest.LinkCase{
		{
			Name: "football.com mobile route",
			Ref:  sourcestest.EPLMatch("sr:match:72221358"),
			Want: "https://www.football.com/ng/m/sport/football/England/Premier_League/Chelsea_vs_Manchester_United/sr:match:72221358",
		},
	})
}
