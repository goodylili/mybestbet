package prizepicks_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/prizepicks"
	"mybestbet/pkg/sources/sourcestest"
)

const source = "prizepicks"

var boardFilesByLeagueID = map[string]string{
	"82":  "projections.json",
	"7":   "projections_nba.json",
	"9":   "projections_nfl.json",
	"5":   "projections_tennis.json",
	"265": "projections_cs2.json",
}

func boardRoute(r *http.Request) (string, bool) {
	file, ok := boardFilesByLeagueID[r.URL.Query().Get("league_id")]
	return file, ok && r.URL.Path == "/projections"
}

func TestAdapter_DiscoverGolden(t *testing.T) {
	t.Parallel()
	srv := sourcestest.ReplayServer(t, source, boardRoute)
	snap, err := prizepicks.New(sourcestest.Deps(srv, sources.Config{})).Discover(context.Background(), []string{"EPL", "LALIGA", "MLS"})
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(snap.Events) != 3 {
		t.Fatalf("Discover() = %d events, want 3 (Liga MX filtered out)", len(snap.Events))
	}
	sourcestest.AssertGolden(t, source, "discover", snap)
}

func TestAdapter_PriceGolden(t *testing.T) {
	t.Parallel()
	srv := sourcestest.ReplayServer(t, source, boardRoute)
	snap, err := prizepicks.New(sourcestest.Deps(srv, sources.Config{})).Price(context.Background(), []string{"5501", "5502", "5503"})
	if err != nil {
		t.Fatalf("Price() error: %v", err)
	}
	sourcestest.RequireMarkets(t, snap, canon.PropGoals, canon.PropShots, canon.PropSOT, canon.PropAssists, canon.PropFouls, canon.PropTackles,
		canon.PropPasses, canon.PropSaves)
	requireLineOnlyOvers(t, snap)
	sourcestest.AssertGolden(t, source, "price", snap)
}

type sessionBrowser struct {
	warms atomic.Int32
	err   error
}

func (b *sessionBrowser) Fetch(_ context.Context, req sources.BrowserRequest) (sources.BrowserResult, error) {
	b.warms.Add(1)
	if b.err != nil {
		return sources.BrowserResult{}, b.err
	}
	if req.Profile != "prizepicks" || req.URL != "https://app.prizepicks.com/" {
		return sources.BrowserResult{}, errors.New("unexpected warm request")
	}
	return sources.BrowserResult{
		Cookies: map[string]string{"_px3": "token", "__cf_bm": "bm"},
		Headers: map[string]string{"User-Agent": "WarmChrome", "X-Device-Id": "device-1"},
	}, nil
}

func TestAdapter_ReusesWarmSessionUntilForbidden(t *testing.T) {
	t.Parallel()
	board := sourcestest.ReadFixture(t, source, "projections.json")
	var forbidNext atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "__cf_bm=bm; _px3=token" || r.Header.Get("User-Agent") != "WarmChrome" || r.Header.Get("X-Device-Id") != "device-1" {
			http.Error(w, "missing session", http.StatusUnauthorized)
			return
		}
		if forbidNext.CompareAndSwap(true, false) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write(board)
	}))
	t.Cleanup(srv.Close)
	browser := &sessionBrowser{}
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Browser = browser
	adapter := prizepicks.New(deps)

	for range 2 {
		if _, err := adapter.Price(context.Background(), []string{"5501"}); err != nil {
			t.Fatalf("Price() error: %v", err)
		}
	}
	if got := browser.warms.Load(); got != 1 {
		t.Fatalf("session warmed %d times for two fetches, want 1", got)
	}
	forbidNext.Store(true)
	if _, err := adapter.Price(context.Background(), []string{"5501"}); !errors.Is(err, sources.ErrForbidden) {
		t.Fatalf("Price() error = %v, want ErrForbidden", err)
	}
	if _, err := adapter.Price(context.Background(), []string{"5501"}); err != nil {
		t.Fatalf("Price() after re-warm error: %v", err)
	}
	if got := browser.warms.Load(); got != 2 {
		t.Fatalf("session warmed %d times, want a re-warm after 403", got)
	}
}

func TestAdapter_FailsCleanlyWhenSessionWarmFails(t *testing.T) {
	t.Parallel()
	srv := sourcestest.ReplayServer(t, source, boardRoute)
	deps := sourcestest.Deps(srv, sources.Config{})
	browserdDown := errors.New("connection refused")
	deps.Browser = &sessionBrowser{err: browserdDown}
	if _, err := prizepicks.New(deps).Discover(context.Background(), []string{"EPL"}); !errors.Is(err, browserdDown) {
		t.Fatalf("Discover() error = %v, want the browser error", err)
	}
}

func TestAdapter_MultiSportGoldens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		leagues    []string
		wantEvents []string
		wantCodes  []canon.MarketCode
	}{
		{
			name:       "basketball",
			leagues:    []string{"NBA"},
			wantEvents: []string{"NBA-6601", "NBA-6602"},
			wantCodes:  []canon.MarketCode{canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees, canon.PropPtsRebAst},
		},
		{
			name:       "american football",
			leagues:    []string{"NFL"},
			wantEvents: []string{"NFL-6701"},
			wantCodes:  []canon.MarketCode{canon.PropPassYards, canon.PropRushYards, canon.PropRecYards, canon.PropReceptions, canon.PropPassTDs},
		},
		{
			name:       "tennis",
			leagues:    []string{"ATP_MIAMI", "WTA_MIAMI"},
			wantEvents: []string{"TENNIS-6801", "TENNIS-6802"},
			wantCodes:  []canon.MarketCode{canon.PropAces, canon.PropDoubleFlt},
		},
		{
			name:       "esports",
			leagues:    []string{"CS2"},
			wantEvents: []string{"CS2-6901"},
			wantCodes:  []canon.MarketCode{canon.PropMapKills},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adapter := prizepicks.New(sourcestest.Deps(sourcestest.ReplayServer(t, source, boardRoute), sources.Config{}))
			golden := strings.ReplaceAll(tt.name, " ", "_")

			discovered, err := adapter.Discover(context.Background(), tt.leagues)
			if err != nil {
				t.Fatalf("Discover() error: %v", err)
			}
			var gotEvents []string
			for _, e := range discovered.Events {
				gotEvents = append(gotEvents, e.SourceID)
			}
			if !slices.Equal(gotEvents, tt.wantEvents) {
				t.Fatalf("Discover() events = %v, want %v", gotEvents, tt.wantEvents)
			}
			sourcestest.AssertGolden(t, source, "discover_"+golden, discovered)

			priced, err := adapter.Price(context.Background(), tt.wantEvents)
			if err != nil {
				t.Fatalf("Price() error: %v", err)
			}
			sourcestest.RequireMarkets(t, priced, tt.wantCodes...)
			requireLineOnlyOvers(t, priced)
			sourcestest.AssertGolden(t, source, "price_"+golden, priced)
		})
	}
}

func requireLineOnlyOvers(t *testing.T, snap canon.Snapshot) {
	t.Helper()
	for _, m := range snap.Markets {
		sel := m.Selections[0]
		if sel.Price.Form != canon.FormLineOnly || sel.Price.Prob != 0 || sel.Side != canon.SideOver || m.Line == nil || sel.Price.Raw != *m.Line {
			t.Fatalf("market %s: selection %+v is not a line-only over at the market line", m.SourceMarketID, sel)
		}
	}
}

func TestAdapter_FetchesOneBoardPerRequestedLeagueFamily(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requested = append(requested, r.URL.Query().Get("league_id"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":[],"included":[]}`))
	}))
	t.Cleanup(srv.Close)
	adapter := prizepicks.New(sourcestest.Deps(srv, sources.Config{Extra: map[string]string{"cs2LeagueId": "999"}}))
	leagues := []string{"EPL", "LALIGA", "NBA", "WNBA", "NCAAB", "NFL", "NCAAF", "ATP_AO", "CS2", "DOTA2", "VALORANT", "LOL", "EUROLEAGUE"}
	if _, err := adapter.Discover(context.Background(), leagues); err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	want := []string{"82", "5", "7", "3", "20", "9", "15", "999", "174", "159", "121"}
	if !slices.Equal(requested, want) {
		t.Fatalf("boards fetched = %v, want %v", requested, want)
	}
}

func TestAdapter_PriceKeepsHealthyBoardsWhenOneFails(t *testing.T) {
	t.Parallel()
	srv := sourcestest.ReplayServer(t, source, boardRoute)
	adapter := prizepicks.New(sourcestest.Deps(srv, sources.Config{Extra: map[string]string{"nflLeagueId": "404"}}))
	snap, err := adapter.Price(context.Background(), []string{"NBA-6601", "NFL-6701"})
	if err != nil {
		t.Fatalf("Price() error = %v, want partial success", err)
	}
	if len(snap.Errors) != 1 || len(snap.Markets) == 0 {
		t.Fatalf("Price() = %d markets, %d errors; want NBA markets plus one NFL error", len(snap.Markets), len(snap.Errors))
	}
}
