package betpawa_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betpawa"
	"mybestbet/pkg/sources/sourcestest"
)

func replayRoute(t *testing.T) sourcestest.Route {
	fixtures := sourcestest.TestdataDir(t, betpawa.Name)
	return func(r *http.Request) (string, bool) {
		name := fixtureName(r)
		if _, err := os.Stat(filepath.Join(fixtures, name)); name == "" || err != nil {
			return "", false
		}
		return name, true
	}
}

func fixtureName(r *http.Request) string {
	if r.URL.Path == "/events/lists/by-queries" {
		return listFixtureName(r.URL.Query().Get("q"))
	}
	if categoryID, ok := strings.CutPrefix(r.URL.Path, "/categories/list/"); ok {
		return "categories_" + categoryID + ".json"
	}
	if eventID, ok := strings.CutPrefix(r.URL.Path, "/events/"); ok && !strings.Contains(eventID, "/") {
		return "event_" + eventID + ".json"
	}
	return ""
}

func listFixtureName(rawQuery string) string {
	var q struct {
		Queries []struct {
			Query struct {
				Zones struct {
					Competitions []string `json:"competitions"`
				} `json:"zones"`
			} `json:"query"`
			Skip int `json:"skip"`
		} `json:"queries"`
	}
	if err := json.Unmarshal([]byte(rawQuery), &q); err != nil || len(q.Queries) != 1 || q.Queries[0].Skip != 0 {
		return ""
	}
	return "list_" + strings.Join(q.Queries[0].Query.Zones.Competitions, "_") + ".json"
}

func newAdapter(t *testing.T) sources.Adapter {
	t.Helper()
	return newAdapterWithConfig(t, sources.Config{})
}

func newAdapterWithConfig(t *testing.T, cfg sources.Config) sources.Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, betpawa.Name, replayRoute(t))
	return betpawa.New(sourcestest.Deps(srv, cfg))
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Discover(context.Background(), []string{"EPL", "LALIGA", "UNKNOWN"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Events) != 8 {
		t.Fatalf("want 8 events, got %d", len(snap.Events))
	}
	sourcestest.AssertGolden(t, betpawa.Name, "discover", snap)
}

func TestDiscoverBasketballAndTennis(t *testing.T) {
	t.Parallel()
	a := newAdapterWithConfig(t, sources.Config{LeagueIDs: map[string]string{"ATP_CH_ZADAR": "15093"}})
	snap, err := a.Discover(context.Background(), []string{"NBA", "ATP_CH_ZADAR", "NFL"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", snap.Errors)
	}
	if len(snap.Events) != 15 {
		t.Fatalf("want 8 NBA and 7 tennis events, got %d", len(snap.Events))
	}
	sourcestest.AssertGolden(t, betpawa.Name, "discover_sports", snap)
}

func TestPriceBasketballAndTennis(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"38617130", "36953845", "38913841"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeTotalGames)
	for _, m := range snap.Markets {
		if spec, ok := canon.SpecOf(m.Code); !ok || spec.IsFor(canon.SportFootball) {
			t.Errorf("market %s mapped to football code %s", m.SourceMarketID, m.Code)
		}
	}
	sourcestest.AssertGolden(t, betpawa.Name, "price_sports", snap)
}

func TestPrice(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"38638128", "38616982", "37079166"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	props := []canon.MarketCode{canon.PropAnytime, canon.PropFirst, canon.PropLast, canon.PropGA, canon.PropCard, canon.PropShots, canon.PropSOT, canon.PropSaves, canon.TeamGoals}
	sourcestest.RequireMarkets(t, snap, append(canon.MatchMarkets, props...)...)
	sourcestest.AssertGolden(t, betpawa.Name, "price", snap)
}

func TestPriceTeamGoalsByTeamAndHalf(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"38282373"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.TeamGoals, canon.PropSaves)
	seen := map[string]bool{}
	for _, m := range snap.Markets {
		if m.Code != canon.TeamGoals {
			continue
		}
		if m.Line == nil || len(m.Selections) == 0 {
			t.Errorf("team goals market %s has no line or selections", m.SourceMarketID)
		}
		for _, s := range m.Selections {
			if s.Side != canon.SideOver && s.Side != canon.SideUnder {
				t.Errorf("team goals market %s has side %q", m.SourceMarketID, s.Side)
			}
		}
		seen[m.Team+"/"+m.Period] = true
	}
	for _, team := range []string{"Sassuolo Calcio", "Lazio Rome"} {
		for _, period := range []string{canon.PeriodFullTime, canon.PeriodFirstHalf, canon.PeriodSecondHalf} {
			if !seen[team+"/"+period] {
				t.Errorf("missing team goals for %s period %q", team, period)
			}
		}
	}
	sourcestest.AssertGolden(t, betpawa.Name, "price_team_goals", snap)
}

func TestPriceSkipsNoScorerOutcomes(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"38638128"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	for _, m := range snap.Markets {
		if m.Player != nil && strings.HasPrefix(m.Player.Name, "No ") {
			t.Errorf("market %s priced a no-scorer outcome as a player", m.SourceMarketID)
		}
	}
}

func TestPriceReportsMissingEvent(t *testing.T) {
	t.Parallel()
	_, err := newAdapter(t).Price(context.Background(), []string{"404"})
	if !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPriceFootballHalves(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"38781890"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	seen := map[string]bool{}
	for _, m := range snap.Markets {
		seen[string(m.Code)+"/"+m.Period] = len(m.Selections) > 0
	}
	for _, code := range []canon.MarketCode{canon.CodeOU, canon.CodeAH, canon.CodeDC, canon.CodeBTTS, canon.CodeDNB, canon.CodeCS} {
		for _, period := range []string{canon.PeriodFirstHalf, canon.PeriodSecondHalf} {
			if !seen[string(code)+"/"+period] {
				t.Errorf("missing priced %s %s market", code, period)
			}
		}
	}
}
