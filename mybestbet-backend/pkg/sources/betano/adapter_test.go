package betano_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betano"
	"mybestbet/pkg/sources/sourcestest"
)

func replayRoute(t *testing.T) sourcestest.Route {
	fixtures := sourcestest.TestdataDir(t, betano.Name)
	return func(r *http.Request) (string, bool) {
		name := fixtureName(r)
		if _, err := os.Stat(filepath.Join(fixtures, name)); name == "" || err != nil {
			return "", false
		}
		return name, true
	}
}

func fixtureName(r *http.Request) string {
	if r.URL.Query().Get("req") != "s,stnf,c,mb,mbl" {
		return ""
	}
	p := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case strings.HasPrefix(p, "/api/sport/") && strings.Count(p, "/") == 3:
		return "sport_" + path.Base(p) + ".json"
	case strings.HasPrefix(p, "/api/sport/"):
		return "league_" + path.Base(p) + ".json"
	case strings.HasPrefix(p, "/api/match-odds/") && strings.Count(p, "/") == 4:
		return "match_" + path.Base(p) + ".json"
	}
	return ""
}

func newAdapter(t *testing.T) sources.Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, betano.Name, replayRoute(t))
	return betano.New(sourcestest.Deps(srv, sources.Config{}))
}

func discover(t *testing.T, a sources.Adapter) canon.Snapshot {
	t.Helper()
	snap, err := a.Discover(context.Background(), []string{"EPL", "LALIGA", "SERIEA"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return snap
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	snap := discover(t, newAdapter(t))
	if len(snap.Events) != 5 || len(snap.Errors) != 0 {
		t.Fatalf("want 5 events and no errors, got %d events, errors %v", len(snap.Events), snap.Errors)
	}
	sourcestest.AssertGolden(t, betano.Name, "discover", snap)
}

func TestDiscoverHonoursLeagueIDOverride(t *testing.T) {
	t.Parallel()
	srv := sourcestest.ReplayServer(t, betano.Name, replayRoute(t))
	a := betano.New(sourcestest.Deps(srv, sources.Config{LeagueIDs: map[string]string{"CHAMP": "5"}}))
	snap, err := a.Discover(context.Background(), []string{"CHAMP"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, ev := range snap.Events {
		if ev.LeagueHint != "CHAMP" {
			t.Fatalf("event %s hinted %s, want CHAMP", ev.SourceID, ev.LeagueHint)
		}
	}
	if len(snap.Events) != 2 {
		t.Fatalf("want the 2 events of overridden league 5, got %d", len(snap.Events))
	}
}

func TestPrice(t *testing.T) {
	t.Parallel()
	a := newAdapter(t)
	discover(t, a)
	snap, err := a.Price(context.Background(), []string{"74510233", "74510241", "74498810"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", snap.Errors)
	}
	props := []canon.MarketCode{canon.PropAnytime, canon.PropFirst, canon.PropLast, canon.PropCard}
	sourcestest.RequireMarkets(t, snap, append(canon.MatchMarkets, props...)...)
	sourcestest.AssertGolden(t, betano.Name, "price", snap)
}

var otherSportLeagues = []string{"NBA", "NFL", "ATP_MIAMI", "CS2", "LOL", "DOTA2"}

func TestDiscoverOtherSports(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Discover(context.Background(), otherSportLeagues)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Events) != 7 || len(snap.Errors) != 0 {
		t.Fatalf("want 7 events and no errors, got %d events, errors %v", len(snap.Events), snap.Errors)
	}
	sourcestest.AssertGolden(t, betano.Name, "discover_sports", snap)
}

func TestPriceOtherSports(t *testing.T) {
	t.Parallel()
	a := newAdapter(t)
	if _, err := a.Discover(context.Background(), otherSportLeagues); err != nil {
		t.Fatalf("discover: %v", err)
	}
	snap, err := a.Price(context.Background(), []string{"75100001", "75200001", "75300001", "75400001"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeSetHcp, canon.CodeTotalGames, canon.CodeMapHcp, canon.CodeTotalMaps)
	for _, m := range snap.Markets {
		if m.Code == canon.Code1X2 {
			t.Errorf("basketball 3-way market %s must be dropped, not mapped to 1X2", m.SourceMarketID)
		}
	}
	sourcestest.AssertGolden(t, betano.Name, "price_sports", snap)
}

func TestPriceReadsLegacyMarketsShape(t *testing.T) {
	t.Parallel()
	a := newAdapter(t)
	discover(t, a)
	snap, err := a.Price(context.Background(), []string{"74498810"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.CodeDNB, canon.CodeOU, canon.CodeCS, canon.Code1H1X2)
}

func TestPriceNeedsDiscoveredSlug(t *testing.T) {
	t.Parallel()
	_, err := newAdapter(t).Price(context.Background(), []string{"74510233"})
	if err == nil || errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("want slug error before discovery, got %v", err)
	}
}
