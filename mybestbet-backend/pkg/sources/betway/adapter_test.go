package betway_test

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betway"
	"mybestbet/pkg/sources/sourcestest"
)

func replayRoute(t *testing.T) sourcestest.Route {
	fixtures := sourcestest.TestdataDir(t, betway.Name)
	return func(r *http.Request) (string, bool) {
		name := fixtureName(r)
		if _, err := os.Stat(filepath.Join(fixtures, name)); name == "" || err != nil {
			return "", false
		}
		return name, true
	}
}

func fixtureName(r *http.Request) string {
	q := r.URL.Query()
	if sport, ok := strings.CutPrefix(r.URL.Path, "/Feeds/RegionsAndLeagues/"); ok {
		return "regions_" + sport + ".json"
	}
	switch r.URL.Path {
	case "/BetBook/Filtered/":
		return catalogFixtureName(q)
	case "/MarketGroupings/MarketGroupNamesAndMarketsForEvent":
		return "event_" + q.Get("eventId") + ".json"
	}
	return ""
}

func catalogFixtureName(q url.Values) string {
	if q.Get("Skip") != "0" || len(q["marketTypes"]) == 0 {
		return ""
	}
	sport := q.Get("sportId")
	if sport != "soccer" {
		return "catalog_" + sport + ".json"
	}
	if q.Get("RegionAndLeagueIds[0].leagueId") == "premier-league" && q.Get("RegionAndLeagueIds[1].leagueId") == "laliga" && !q.Has("RegionAndLeagueIds[2].leagueId") {
		return "catalog.json"
	}
	return ""
}

func newAdapter(t *testing.T) sources.Adapter {
	t.Helper()
	return newAdapterWithConfig(t, sources.Config{})
}

func newAdapterWithConfig(t *testing.T, cfg sources.Config) sources.Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, betway.Name, replayRoute(t))
	return betway.New(sourcestest.Deps(srv, cfg))
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Discover(context.Background(), []string{"EPL", "LALIGA", "SPL"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Events) != 8 {
		t.Fatalf("want 8 events, got %d", len(snap.Events))
	}
	sourcestest.AssertGolden(t, betway.Name, "discover", snap)
}

func TestDiscoverOtherSports(t *testing.T) {
	t.Parallel()
	a := newAdapterWithConfig(t, sources.Config{LeagueIDs: map[string]string{"ATP_CH_ZADAR": "atp/atp-basel-switzerland-men-singles"}})
	snap, err := a.Discover(context.Background(), []string{"NBA", "NFL", "ATP_CH_ZADAR", "ATP_MIAMI", "CS2", "LOL", "DOTA2", "VALORANT"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", snap.Errors)
	}
	if len(snap.Events) != 21 {
		t.Fatalf("want 5 NBA, 5 NFL, 5 tennis, 1 CS2 and 5 LoL pre-match events, got %d", len(snap.Events))
	}
	sourcestest.AssertGolden(t, betway.Name, "discover_sports", snap)
}

func TestPriceOtherSports(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"73719372", "71515928", "75620244", "75881892", "75957214"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeSetHcp, canon.CodeTotalGames, canon.CodeMapHcp, canon.CodeTotalMaps)
	for _, m := range snap.Markets {
		if spec, ok := canon.SpecOf(m.Code); !ok || spec.IsFor(canon.SportFootball) {
			t.Errorf("market %s mapped to football code %s", m.SourceMarketID, m.Code)
		}
	}
	sourcestest.AssertGolden(t, betway.Name, "price_sports", snap)
}

func TestPrice(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"72221344", "72221332", "72478646"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	props := []canon.MarketCode{canon.PropAnytime, canon.PropFirst, canon.PropLast, canon.PropShots, canon.PropSOT, canon.PropAssists}
	sourcestest.RequireMarkets(t, snap, append(canon.MatchMarkets, props...)...)
	sourcestest.RequireMarkets(t, snap, canon.TeamGoals)
	sourcestest.AssertGolden(t, betway.Name, "price", snap)
}

func TestPriceTeamTotals(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"71945360"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	type teamPeriod struct{ team, period string }
	seen := map[teamPeriod]int{}
	for _, m := range snap.Markets {
		if m.Code != canon.TeamGoals {
			continue
		}
		if m.Line == nil || len(m.Selections) != 2 || m.Selections[0].Side != canon.SideOver || m.Selections[1].Side != canon.SideUnder {
			t.Fatalf("team total %s = %+v, want an over/under line", m.SourceMarketID, m)
		}
		seen[teamPeriod{m.Team, m.Period}]++
	}
	want := []teamPeriod{
		{"Sassuolo Calcio", canon.PeriodFullTime},
		{"Lazio Rome", canon.PeriodFullTime},
		{"Sassuolo Calcio", canon.PeriodSecondHalf},
		{"Lazio Rome", canon.PeriodSecondHalf},
	}
	for _, w := range want {
		if seen[w] == 0 {
			t.Errorf("no team total for %+v in %v", w, seen)
		}
	}
	if len(seen) != len(want) {
		t.Errorf("team totals = %v, want only %v", seen, want)
	}
}

func TestTeamOfTotal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		displayName string
		expected    string
		ok          bool
	}{
		{name: "full time", displayName: "Arsenal Total (0.5)", expected: "Arsenal", ok: true},
		{name: "second half", displayName: "2nd Half - Lazio Rome Total (1.5)", expected: "Lazio Rome", ok: true},
		{name: "first half", displayName: "1st Half - Charlotte Hornets Total (57.5)", expected: "Charlotte Hornets", ok: true},
		{name: "header without line", displayName: "Lazio Rome - Total Goals", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			team, ok := betway.TeamOfTotal(tt.displayName)
			if ok != tt.ok || team != tt.expected {
				t.Fatalf("teamOfTotal(%q) = %q, %v; want %q, %v", tt.displayName, team, ok, tt.expected, tt.ok)
			}
		})
	}
}

func TestPriceReportsMissingEvent(t *testing.T) {
	t.Parallel()
	_, err := newAdapter(t).Price(context.Background(), []string{"1"})
	if !errors.Is(err, sources.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestPriceHalvesAndQualify(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t).Price(context.Background(), []string{"74374504"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.CodeQualify, canon.CodeDC, canon.CodeDNB, canon.CodeBTTS, canon.CodeAH)
	periods := map[canon.MarketCode]map[string]bool{}
	for _, m := range snap.Markets {
		if periods[m.Code] == nil {
			periods[m.Code] = map[string]bool{}
		}
		periods[m.Code][m.Period] = true
	}
	for _, code := range []canon.MarketCode{canon.CodeDC, canon.CodeDNB, canon.CodeBTTS, canon.CodeAH} {
		if !periods[code][canon.PeriodFirstHalf] || !periods[code][canon.PeriodSecondHalf] {
			t.Fatalf("%s periods = %v, want 1H and 2H", code, periods[code])
		}
	}
	sourcestest.AssertGolden(t, betway.Name, "price_halves", snap)
}

func TestPriceMapHandicapKeepsQuotedLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		eventID string
		line    float64
		home    float64
		away    float64
	}{
		{name: "home giving maps", eventID: "76002882", line: -1.5, home: 1.97, away: 1.72},
		{name: "home receiving maps", eventID: "76002810", line: 1.5, home: 1.55, away: 2.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snap, err := newAdapter(t).Price(context.Background(), []string{tt.eventID})
			if err != nil {
				t.Fatalf("price: %v", err)
			}
			var handicaps []canon.RawMarket
			for _, m := range snap.Markets {
				if m.Code == canon.CodeMapHcp {
					handicaps = append(handicaps, m)
				}
			}
			if len(handicaps) != 1 {
				t.Fatalf("got %d map handicap markets, want 1", len(handicaps))
			}
			m := handicaps[0]
			if m.Line == nil || *m.Line != tt.line {
				t.Fatalf("line = %v, want %v", m.Line, tt.line)
			}
			prices := map[canon.Side]float64{}
			for _, s := range m.Selections {
				prices[s.Side] = s.Price.Raw
			}
			if len(m.Selections) != 2 || prices[canon.SideHome] != tt.home || prices[canon.SideAway] != tt.away {
				t.Fatalf("selections = %+v, want home %v away %v", m.Selections, tt.home, tt.away)
			}
		})
	}
}

type ladderRung struct{ rising, falling float64 }

func pricedLadder(t *testing.T, eventID string, code canon.MarketCode, period string) map[float64]ladderRung {
	t.Helper()
	snap, err := newAdapter(t).Price(context.Background(), []string{eventID})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	ladder := map[float64]ladderRung{}
	for _, m := range snap.Markets {
		if m.Code != code || m.Period != period || m.Line == nil {
			continue
		}
		var r ladderRung
		for _, s := range m.Selections {
			switch s.Side {
			case canon.SideOver, canon.SideAway:
				r.rising = s.Price.Raw
			case canon.SideUnder, canon.SideHome:
				r.falling = s.Price.Raw
			default:
			}
		}
		ladder[*m.Line] = r
	}
	return ladder
}

func TestPriceDropsIncoherentLadderRungs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		eventID string
		code    canon.MarketCode
		period  string
		absent  []float64
		present []float64
	}{
		{name: "full game total clamped at 1.42/2.65 below the main line", eventID: "73719408", code: canon.CodeTotal, absent: []float64{226.5, 229.5, 233.5}, present: []float64{234.5, 240.5, 247.5}},
		{name: "first half total running backwards below 114.5", eventID: "73719414", code: canon.CodeTotal, period: canon.PeriodFirstHalf, absent: []float64{109.5, 111.5, 113.5}, present: []float64{114.5, 116.5, 118.5}},
		{name: "nfl spread near even money far from the 4.5 main line", eventID: "71515950", code: canon.CodeSpread, absent: []float64{-2.5, -0.5, 0.5, 1.5}, present: []float64{2.5, 4.5, 6.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ladder := pricedLadder(t, tt.eventID, tt.code, tt.period)
			for _, line := range tt.absent {
				if _, ok := ladder[line]; ok {
					t.Fatalf("incoherent line %v kept: %+v", line, ladder[line])
				}
			}
			for _, line := range tt.present {
				if _, ok := ladder[line]; !ok {
					t.Fatalf("line %v dropped", line)
				}
			}
			lines := slices.Sorted(maps.Keys(ladder))
			for i := 1; i < len(lines); i++ {
				lower, higher := ladder[lines[i-1]], ladder[lines[i]]
				if higher.rising <= lower.rising || higher.falling >= lower.falling {
					t.Fatalf("ladder not monotone between %v %+v and %v %+v", lines[i-1], lower, lines[i], higher)
				}
			}
		})
	}
}
