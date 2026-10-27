package bet9ja

import (
	"errors"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func recordedFixtures(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(sourcestest.TestdataDir(t, Name))
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

func replayRoute(recorded map[string]bool) sourcestest.Route {
	return func(r *http.Request) (string, bool) {
		q := r.URL.Query()
		var name string
		switch r.URL.Path {
		case "/PalimpsestAjax/GetSports":
			name = "sports.json"
		case "/PalimpsestAjax/GetEventsInGroup":
			name = "group_" + q.Get("GROUPID") + ".json"
		case "/PalimpsestAjax/GetEventsInGroupV2":
			name = "groupv2_" + q.Get("GROUPID") + "_" + q.Get("GROUPMARKETID") + ".json"
		case "/PalimpsestAjax/GetEvent":
			name = "event_" + q.Get("EVENTID") + ".json"
		}
		return name, recorded[name]
	}
}

func newReplayAdapter(t *testing.T) sources.Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, Name, replayRoute(recordedFixtures(t)))
	return New(sourcestest.Deps(srv, sources.Config{}))
}

func TestAdapter_DiscoverGolden(t *testing.T) {
	snap, err := newReplayAdapter(t).Discover(t.Context(), []string{"EPL", "SERIEA", "UNMAPPED"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("discover errors: %v", snap.Errors)
	}
	if got := len(snap.Events); got != 6 {
		t.Fatalf("discover events = %d, want 6", got)
	}
	sourcestest.AssertGolden(t, Name, "discover", snap)
}

func TestAdapter_PriceGolden(t *testing.T) {
	adapter := newReplayAdapter(t)
	if _, err := adapter.Discover(t.Context(), []string{"CHAMP"}); err != nil {
		t.Fatalf("discover: %v", err)
	}
	snap, err := adapter.Price(t.Context(), []string{"852457626", "832843044", "855985258"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("price errors: %v", snap.Errors)
	}
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.CodeDC, canon.CodeOU, canon.CodeBTTS, canon.CodeDNB,
		canon.CodeAH, canon.CodeCS, canon.CodeHTFT, canon.Code1H1X2, canon.CodeCornOU, canon.CodeQualify,
		canon.PropAnytime, canon.PropFirst, canon.PropLast)
	sourcestest.AssertGolden(t, Name, "price", snap)
}

func TestAdapter_PriceFailsWhenEveryEventFails(t *testing.T) {
	_, err := newReplayAdapter(t).Price(t.Context(), []string{"1"})
	if !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("price error = %v, want ErrNotFound", err)
	}
}

func TestAdapter_DiscoverMultiSportGolden(t *testing.T) {
	snap, err := newReplayAdapter(t).Discover(t.Context(), []string{"NBA", "NFL", "DFBPOKAL"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("discover errors: %v", snap.Errors)
	}
	for _, league := range []string{"NBA", "NFL", "DFBPOKAL"} {
		if !slices.ContainsFunc(snap.Events, func(ev canon.SourceEvent) bool { return ev.LeagueHint == league }) {
			t.Fatalf("no event discovered for %s", league)
		}
	}
	sourcestest.AssertGolden(t, Name, "discover_multisport", snap)
}

func TestAdapter_PriceMultiSportGolden(t *testing.T) {
	snap, err := newReplayAdapter(t).Price(t.Context(), []string{"856594391", "850368109", "855970462"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("price errors: %v", snap.Errors)
	}
	sourcestest.RequireMarkets(t, snap, canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeSetHcp, canon.CodeTotalGames)
	sourcestest.AssertGolden(t, Name, "price_multisport", snap)
}

func TestSportsTree_GroupIDsNamed(t *testing.T) {
	tree := sportsTree{Sports: map[string]treeSport{
		"1": {Name: soccer, Categories: map[string]treeCategory{
			"9": {Name: "Germany", Groups: map[string]treeGroup{"2": {Name: "DFB Pokal"}, "1": {Name: "Bundesliga"}}},
			"7": {Name: "Mexico", Groups: map[string]treeGroup{
				"3": {Name: "Liga MX, Apertura"},
				"4": {Name: "Liga MX, Women, Apertura"},
				"5": {Name: "Liga MX Expansion"},
			}},
			"6": {Name: "International Clubs", Groups: map[string]treeGroup{
				"8": {Name: "UEFA Champions League"},
				"9": {Name: "UEFA Champions League, Women"},
			}},
		}},
		"5": {Name: tennis, Categories: map[string]treeCategory{"4": {Name: "ATP", Groups: map[string]treeGroup{
			"10": {Name: "ATP Basel, Switzerland Men Singles"},
			"11": {Name: "ATP Basel, Switzerland Men Doubles"},
		}}}},
	}}
	tests := []struct {
		name     string
		match    groupName
		expected []string
	}{
		{name: "exact group", match: groupName{sport: soccer, category: "Germany", group: "DFB Pokal"}, expected: []string{"2"}},
		{name: "season stage suffix", match: groupName{sport: soccer, category: "Mexico", group: "Liga MX"}, expected: []string{"3"}},
		{name: "men without women", match: groupName{sport: soccer, category: "International Clubs", group: "UEFA Champions League"}, expected: []string{"8"}},
		{name: "women exact", match: groupName{sport: soccer, category: "International Clubs", group: "UEFA Champions League, Women"}, expected: []string{"9"}},
		{name: "singles only", match: groupName{sport: tennis, category: "ATP", group: "ATP Basel"}, expected: []string{"10"}},
		{name: "wrong sport", match: groupName{sport: basketball, category: "Germany", group: "DFB Pokal"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tree.groupIDsNamed(tt.match); !slices.Equal(got, tt.expected) {
				t.Fatalf("groupIDsNamed = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSourcePlayer(t *testing.T) {
	got, ok := sourcePlayer(event{ID: 7, Description: "(Bristol City) Yah, Gibson Osahumen "})
	if !ok {
		t.Fatal("sourcePlayer not ok")
	}
	if got.Name != "Gibson Osahumen Yah" || got.Team != "Bristol City" {
		t.Fatalf("sourcePlayer = %+v", got)
	}
}

func TestParseOddsKey(t *testing.T) {
	tests := []struct {
		key      string
		expected oddsKey
	}{
		{key: "S_1X2_1", expected: oddsKey{market: "S_1X2", outcome: "1"}},
		{key: "S_OU@2.5_O", expected: oddsKey{market: "S_OU", specifier: "2.5", outcome: "O"}},
		{key: "S_AH@-0.25_2", expected: oddsKey{market: "S_AH", specifier: "-0.25", outcome: "2"}},
		{key: "S_CSFT_2:1", expected: oddsKey{market: "S_CSFT", outcome: "2:1"}},
		{key: "S_HTFT_1/X", expected: oddsKey{market: "S_HTFT", outcome: "1/X"}},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := parseOddsKey(tt.key)
			if !ok || got != tt.expected {
				t.Fatalf("parseOddsKey(%q) = %+v %v, want %+v", tt.key, got, ok, tt.expected)
			}
		})
	}
}

func TestEvent_KickoffIsUTC(t *testing.T) {
	want := time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		ev   event
	}{
		{name: "utc field", ev: event{StartUTC: "2026-10-24T16:00:00Z", StartDate: "2026-10-24 16:00:00"}},
		{name: "start date only", ev: event{StartDate: "2026-10-24 16:00:00"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ev.kickoff()
			if err != nil {
				t.Fatalf("kickoff: %v", err)
			}
			if !got.Equal(want) {
				t.Fatalf("kickoff = %v, want %v", got, want)
			}
		})
	}
}
