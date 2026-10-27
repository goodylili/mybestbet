package betking_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betking"
	"mybestbet/pkg/sources/sourcestest"
)

func replayRoute(t *testing.T) sourcestest.Route {
	fixtures := sourcestest.TestdataDir(t, betking.Name)
	return func(r *http.Request) (string, bool) {
		name := fixtureName(r.URL.Path)
		if _, err := os.Stat(filepath.Join(fixtures, name)); name == "" || err != nil {
			return "", false
		}
		return name, true
	}
}

func fixtureName(urlPath string) string {
	if sportID, ok := strings.CutPrefix(urlPath, "/api/feeds/prematch/matches/custommenu/en/4/"); ok {
		return "menu_" + sportID + ".json"
	}
	if eventID, ok := strings.CutPrefix(urlPath, "/api/feeds/prematch/event/en/4/"); ok {
		return "event_" + eventID + ".json"
	}
	if rest, ok := strings.CutPrefix(urlPath, "/api/feeds/prematch/en/4/"); ok {
		tournamentID, _, _ := strings.Cut(rest, "/")
		return "tournament_" + tournamentID + ".json"
	}
	return ""
}

func newAdapter(t *testing.T) sources.Adapter {
	t.Helper()
	return newAdapterWithConfig(t, sources.Config{})
}

func newAdapterWithConfig(t *testing.T, cfg sources.Config) sources.Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, betking.Name, replayRoute(t))
	return betking.New(sourcestest.Deps(srv, cfg))
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Discover(context.Background(), []string{"EPL", "LALIGA", "SERIEA", "MLS"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", snap.Errors)
	}
	if len(snap.Events) != 9 {
		t.Fatalf("want 9 events from the EPL, LALIGA and SERIEA fixtures, got %d", len(snap.Events))
	}
	sourcestest.AssertGolden(t, betking.Name, "discover", snap)
}

func TestDiscoverCupsAndOtherSports(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Discover(context.Background(), []string{"EFLCUP", "NBA", "NBL", "NFL", "CS2"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", snap.Errors)
	}
	if len(snap.Events) != 13 {
		t.Fatalf("want 13 events from the EFL Cup, NBA, NBL and NFL fixtures, got %d", len(snap.Events))
	}
	sourcestest.AssertGolden(t, betking.Name, "discover_sports", snap)
}

func TestDiscoverTennisByTournamentOverride(t *testing.T) {
	t.Parallel()
	a := newAdapterWithConfig(t, sources.Config{LeagueIDs: map[string]string{"ATP_CH_ZADAR": "24357972"}})
	snap, err := a.Discover(context.Background(), []string{"ATP_CH_ZADAR", "ATP_MIAMI"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Events) != 2 {
		t.Fatalf("want the 2 events of the overridden tournament, got %d", len(snap.Events))
	}
	for _, ev := range snap.Events {
		if ev.LeagueHint != "ATP_CH_ZADAR" || ev.Home == "" || ev.Away == "" {
			t.Errorf("event %+v: want ATP_CH_ZADAR with both players", ev)
		}
	}
}

func TestPrice(t *testing.T) {
	t.Parallel()
	ids := []string{"1005710306", "1005710393", "1005539315"}
	snap, err := newAdapter(t).Price(context.Background(), ids)
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, append(canon.MatchMarkets, canon.PropAnytime, canon.PropFirst, canon.PropLast)...)
	sourcestest.AssertGolden(t, betking.Name, "price", snap)
}

func TestPriceOtherSports(t *testing.T) {
	t.Parallel()
	ids := []string{"1005499155", "1005738805", "1005543054", "1005557634", "1005828847"}
	snap, err := newAdapter(t).Price(context.Background(), ids)
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.CodeQualify, canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeTotalGames, canon.PropPoints)
	sourcestest.AssertGolden(t, betking.Name, "price_sports", snap)
}

func TestPriceDropsRegulationThreeWayOutsideFootball(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"1005543054"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	for _, m := range snap.Markets {
		if m.Code == canon.Code1X2 || m.Code == canon.CodeHTFT {
			t.Errorf("basketball market %s mapped to football code %s", m.SourceMarketID, m.Code)
		}
		if m.Code == canon.CodeMoneyline && m.Rules.RegulationOnly {
			t.Errorf("moneyline %s must settle including overtime", m.SourceMarketID)
		}
	}
}

func TestPriceMergesDuplicateFragmentsOnce(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"1005710306"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	seen := map[string]int{}
	for _, m := range snap.Markets {
		seen[m.SourceMarketID]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("market %s emitted %d times", id, n)
		}
	}
}

func TestPriceReportsMissingEvent(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"999"})
	if !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(snap.Markets) != 0 {
		t.Fatalf("want no markets, got %d", len(snap.Markets))
	}
}

func TestPriceFootballHalves(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"1005767843"})
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

func TestPriceTeamGoalsByTeamAndHalf(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"1005574367"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.TeamGoals)
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
	sourcestest.AssertGolden(t, betking.Name, "price_team_goals", snap)
}
