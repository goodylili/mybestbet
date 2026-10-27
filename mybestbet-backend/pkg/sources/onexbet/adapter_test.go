package onexbet

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const fixtureDir = "onexbet"

func recordedFixtures(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(sourcestest.TestdataDir(t, fixtureDir))
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

func replayRoute(recorded map[string]bool, partner string) sourcestest.Route {
	return func(r *http.Request) (string, bool) {
		var name string
		switch r.URL.Path {
		case "/service-api/LineFeed/GetChampZip":
			if !strings.HasPrefix(r.URL.RawQuery, "champ=") || !strings.HasSuffix(r.URL.RawQuery, "&lng=en&country=132&partner="+partner) {
				return "", false
			}
			name = "champ_" + r.URL.Query().Get("champ") + ".json"
		case "/service-api/LineFeed/GetChampsZip":
			if !strings.HasPrefix(r.URL.RawQuery, "sport=") || !strings.HasSuffix(r.URL.RawQuery, "&lng=en&country=132&partner="+partner) {
				return "", false
			}
			name = "champs_" + r.URL.Query().Get("sport") + ".json"
		case "/service-api/LineFeed/GetGameZip":
			if !strings.HasPrefix(r.URL.RawQuery, "id=") || !strings.HasSuffix(r.URL.RawQuery, "&lng=en&isSubGames=true&GroupEvents=true&countevents=2000&country=132&partner="+partner) {
				return "", false
			}
			name = "game_" + r.URL.Query().Get("id") + ".json"
		}
		return name, recorded[name]
	}
}

func newReplayAdapter(t *testing.T, b brand) sources.Adapter {
	t.Helper()
	return newConfiguredReplayAdapter(t, b, sources.Config{})
}

func newConfiguredReplayAdapter(t *testing.T, b brand, cfg sources.Config) sources.Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, fixtureDir, replayRoute(recordedFixtures(t), b.defaultPartner))
	return b.factory()(sourcestest.Deps(srv, cfg))
}

func TestAdapter_DiscoverGolden(t *testing.T) {
	for _, b := range brands {
		t.Run(b.name, func(t *testing.T) {
			snap, err := newReplayAdapter(t, b).Discover(t.Context(), []string{"EPL", "SERIEA", "UNMAPPED"})
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("discover errors: %v", snap.Errors)
			}
			for _, ev := range snap.Events {
				if ev.Home == "Home" || ev.Away == "Away" {
					t.Fatalf("special market leaked into fixtures: %+v", ev)
				}
			}
			sourcestest.AssertGolden(t, fixtureDir, b.name+"_discover", snap)
		})
	}
}

func TestAdapter_PriceGolden(t *testing.T) {
	eventIDs := []string{"760398125", "760404432", "760404429"}
	for _, b := range brands {
		t.Run(b.name, func(t *testing.T) {
			snap, err := newReplayAdapter(t, b).Price(t.Context(), eventIDs)
			if err != nil {
				t.Fatalf("price: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("price errors: %v", snap.Errors)
			}
			sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.CodeDC, canon.CodeOU, canon.CodeBTTS, canon.CodeDNB,
				canon.CodeAH, canon.CodeCS, canon.Code1H1X2, canon.CodeCornOU)
			sourcestest.RequireMarkets(t, snap, canon.PropAnytime, canon.PropFirst, canon.PropLast, canon.PropShots, canon.PropSOT, canon.PropAssists, canon.PropTackles)
			sourcestest.AssertGolden(t, fixtureDir, b.name+"_price", snap)
		})
	}
}

func TestAdapter_PriceFailsWhenEveryEventFails(t *testing.T) {
	_, err := newReplayAdapter(t, brands[0]).Price(t.Context(), []string{"1", "2"})
	if !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("price error = %v, want ErrNotFound", err)
	}
}

func TestAdapter_PartnerFromConfig(t *testing.T) {
	cfg := sources.Config{Extra: map[string]string{partnerExtraKey: "999"}}
	srv := sourcestest.ReplayServer(t, fixtureDir, replayRoute(recordedFixtures(t), "999"))
	snap, err := brands[1].factory()(sourcestest.Deps(srv, cfg)).Discover(t.Context(), []string{"EPL"})
	if err != nil || len(snap.Events) == 0 {
		t.Fatalf("discover with configured partner: events=%d err=%v", len(snap.Events), err)
	}
}

func TestCorrectScoreLabel(t *testing.T) {
	tests := []struct {
		param    *float64
		betType  int
		expected string
	}{
		{param: nil, betType: correctScoreType, expected: "0-0"},
		{param: canon.Line(2.001), betType: correctScoreType, expected: "2-1"},
		{param: canon.Line(0.003), betType: correctScoreType, expected: "0-3"},
		{param: canon.Line(10.009), betType: correctScoreType, expected: "10-9"},
		{param: nil, betType: otherCorrectScoreType, expected: otherScoreLabel},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("type %d expected %s", tt.betType, tt.expected), func(t *testing.T) {
			got, ok := correctScoreLabel(bet{Type: tt.betType, Param: tt.param})
			if !ok || got != tt.expected {
				t.Fatalf("correctScoreLabel = %q %v, want %q", got, ok, tt.expected)
			}
		})
	}
}

func TestHomeHandicap(t *testing.T) {
	tests := []struct {
		name     string
		side     canon.Side
		param    *float64
		expected float64
	}{
		{name: "home keeps sign", side: canon.SideHome, param: canon.Line(-1.25), expected: -1.25},
		{name: "away flips sign", side: canon.SideAway, param: canon.Line(-1.25), expected: 1.25},
		{name: "missing is level", side: canon.SideAway, param: nil, expected: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := homeHandicap(tt.side, tt.param); got != tt.expected {
				t.Fatalf("homeHandicap = %v, want %v", got, tt.expected)
			}
		})
	}
}
