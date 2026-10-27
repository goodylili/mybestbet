package sporty

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const fixtureDir = "sportybet"

func recordedFixtures(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(sourcestest.TestdataDir(t, dir))
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
		sportNumber := strings.TrimPrefix(q.Get("sportId"), "sr:sport:")
		switch r.URL.Path {
		case "/factsCenter/pcEvents":
			name := eventsFixtureName(r)
			if recorded[name] {
				return name, true
			}
			return "events_empty.json", true
		case "/factsCenter/popularAndSportList":
			name := "tree_" + sportNumber + ".json"
			return name, recorded[name] && q.Get("productId") == prematchProductID
		case "/factsCenter/event":
			name := "event_" + strings.TrimPrefix(q.Get("eventId"), "sr:match:") + ".json"
			return name, recorded[name] && q.Get("productId") == prematchProductID
		}
		return "", false
	}
}

func eventsFixtureName(r *http.Request) string {
	var requests []eventsRequest
	if err := json.NewDecoder(r.Body).Decode(&requests); err != nil || len(requests) != 1 {
		return ""
	}
	ids := make([]string, 0, len(requests[0].TournamentIDs))
	for _, group := range requests[0].TournamentIDs {
		for _, id := range group {
			ids = append(ids, strings.TrimPrefix(id, tournamentIDPrefix))
		}
	}
	return "events_" + strings.TrimPrefix(requests[0].SportID, "sr:sport:") + "_" + strings.Join(ids, "-") + ".json"
}

func newReplayAdapterFrom(t *testing.T, dir, brand string, cfg sources.Config) sources.Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, dir, replayRoute(recordedFixtures(t, dir)))
	return Factory(brand)(sourcestest.Deps(srv, cfg))
}

func newReplayAdapter(t *testing.T, brand string) sources.Adapter {
	t.Helper()
	return newReplayAdapterFrom(t, fixtureDir, brand, sources.Config{})
}

func TestAdapter_DiscoverGolden(t *testing.T) {
	for _, brand := range []string{SportyBet, FootballCom} {
		t.Run(brand, func(t *testing.T) {
			snap, err := newReplayAdapter(t, brand).Discover(t.Context(), []string{"EPL", "SERIEA", "ERED", "UNMAPPED"})
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("discover errors: %v", snap.Errors)
			}
			if got := len(snap.Events); got != 58 {
				t.Fatalf("discover events = %d, want 58", got)
			}
			sourcestest.AssertGolden(t, fixtureDir, brand+"_discover", snap)
		})
	}
}

func TestAdapter_PriceGolden(t *testing.T) {
	eventIDs := []string{"sr:match:72221344", "sr:match:72221334", "sr:match:72221332"}
	for _, brand := range []string{SportyBet, FootballCom} {
		t.Run(brand, func(t *testing.T) {
			snap, err := newReplayAdapter(t, brand).Price(t.Context(), eventIDs)
			if err != nil {
				t.Fatalf("price: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("price errors: %v", snap.Errors)
			}
			sourcestest.RequireMarkets(t, snap, canon.MatchMarkets...)
			sourcestest.RequireMarkets(t, snap, canon.PropAnytime, canon.PropFirst, canon.PropLast, canon.PropGoals,
				canon.PropShots, canon.PropSOT, canon.PropAssists, canon.PropGA, canon.PropCard, canon.PropFouls, canon.PropTackles)
			sourcestest.AssertGolden(t, fixtureDir, brand+"_price", snap)
		})
	}
}

func TestAdapter_PriceTeamGoalsSavesAndPasses(t *testing.T) {
	snap, err := newReplayAdapter(t, SportyBet).Price(t.Context(), []string{"sr:match:71945360"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.TeamGoals, canon.PropSaves, canon.PropPasses)
	seen := map[string]bool{}
	for _, m := range snap.Markets {
		if m.Code != canon.TeamGoals {
			continue
		}
		if m.Line == nil || len(m.Selections) != 2 {
			t.Errorf("team goals market %s wants a line and two selections", m.SourceMarketID)
		}
		seen[m.Team+"/"+m.Period] = true
	}
	for _, team := range []string{"Sassuolo", "Lazio"} {
		for _, period := range []string{canon.PeriodFullTime, canon.PeriodFirstHalf, canon.PeriodSecondHalf} {
			if !seen[team+"/"+period] {
				t.Errorf("missing team goals for %s period %q", team, period)
			}
		}
	}
	sourcestest.AssertGolden(t, fixtureDir, "sportybet_price_team_goals", snap)
}

func TestAdapter_PriceFailsWhenEveryEventFails(t *testing.T) {
	snap, err := newReplayAdapter(t, SportyBet).Price(t.Context(), []string{"sr:match:1", "sr:match:2"})
	if !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("price error = %v, want ErrNotFound", err)
	}
	if len(snap.Errors) != 2 {
		t.Fatalf("snapshot errors = %d, want 2", len(snap.Errors))
	}
}

func TestAdapter_PriceKeepsPartialResults(t *testing.T) {
	snap, err := newReplayAdapter(t, SportyBet).Price(t.Context(), []string{"sr:match:72221334", "sr:match:1"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if len(snap.Errors) != 1 || len(snap.Markets) == 0 {
		t.Fatalf("errors = %d markets = %d, want 1 error and some markets", len(snap.Errors), len(snap.Markets))
	}
}

func TestPlayerFromDesc(t *testing.T) {
	tests := []struct {
		name         string
		desc         string
		expectedName string
		expectedTeam string
	}{
		{name: "surname first with team", desc: "Foden, Phil (Man City)", expectedName: "Phil Foden", expectedTeam: "Man City"},
		{name: "surname first with threshold", desc: "Barkley, Ross (Aston Villa) 2+", expectedName: "Ross Barkley", expectedTeam: "Aston Villa"},
		{name: "given name first", desc: "Phil Foden (Manchester City) 1+", expectedName: "Phil Foden", expectedTeam: "Manchester City"},
		{name: "no team", desc: "Phil Foden", expectedName: "Phil Foden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := playerFromDesc("sr:player:1", tt.desc)
			if got.Name != tt.expectedName || got.Team != tt.expectedTeam {
				t.Fatalf("playerFromDesc(%q) = %q/%q, want %q/%q", tt.desc, got.Name, got.Team, tt.expectedName, tt.expectedTeam)
			}
		})
	}
}

func TestParsePlayerOutcomeID(t *testing.T) {
	tests := []struct {
		outcomeID       string
		expectedPlayer  string
		expectedAtLeast int
		expectedOK      bool
	}{
		{outcomeID: "sr:player:1047129", expectedPlayer: "sr:player:1047129", expectedOK: true},
		{outcomeID: "pre:playerprops:72221334:98435:3", expectedPlayer: "sr:player:98435", expectedAtLeast: 3, expectedOK: true},
		{outcomeID: "pre:playerprops:72221334:98435:2:1", expectedPlayer: "sr:player:98435", expectedAtLeast: 2, expectedOK: true},
		{outcomeID: "pre:playerprops:72221334:1047129:2", expectedPlayer: "sr:player:1047129", expectedAtLeast: 2, expectedOK: true},
		{outcomeID: "1716"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("outcome %s", tt.outcomeID), func(t *testing.T) {
			got, ok := parsePlayerOutcomeID(tt.outcomeID)
			if ok != tt.expectedOK || got.playerID != tt.expectedPlayer || got.atLeast != tt.expectedAtLeast {
				t.Fatalf("parsePlayerOutcomeID(%q) = %+v %v", tt.outcomeID, got, ok)
			}
		})
	}
}

func emptyBodyServer(t *testing.T, emptyReplies int) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if int(calls.Add(1)) <= emptyReplies {
			return
		}
		_, _ = w.Write(sourcestest.ReadFixture(t, fixtureDir, "event_73719408.json"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAdapter_PriceRetriesEmptyBody(t *testing.T) {
	srv := emptyBodyServer(t, emptyBodyRetries)
	snap, err := Factory(SportyBet)(sourcestest.Deps(srv, sources.Config{})).Price(t.Context(), []string{"sr:match:73719408"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if len(snap.Markets) == 0 {
		t.Fatalf("no markets after empty replies")
	}
}

func TestAdapter_PriceTreatsPersistentEmptyBodyAsThrottled(t *testing.T) {
	srv := emptyBodyServer(t, emptyBodyRetries+1)
	_, err := Factory(SportyBet)(sourcestest.Deps(srv, sources.Config{})).Price(t.Context(), []string{"sr:match:73719408"})
	if !errors.Is(err, sources.ErrThrottled) {
		t.Fatalf("price error = %v, want ErrThrottled", err)
	}
}
