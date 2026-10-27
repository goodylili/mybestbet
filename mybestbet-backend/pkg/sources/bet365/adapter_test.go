package bet365_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/bet365"
	"mybestbet/pkg/sources/sourcestest"
)

const source = "bet365"

type fixtureBrowser struct {
	t         testing.TB
	responses map[string][]string
	requests  []sources.BrowserRequest
}

func (f *fixtureBrowser) Fetch(_ context.Context, req sources.BrowserRequest) (sources.BrowserResult, error) {
	f.requests = append(f.requests, req)
	for urlSuffix, files := range f.responses {
		if !strings.HasSuffix(req.URL, urlSuffix) {
			continue
		}
		var result sources.BrowserResult
		for _, name := range files {
			result.Captured = append(result.Captured, sources.Captured{URL: req.URL, Status: 200, Body: sourcestest.ReadFixture(f.t, source, name)})
		}
		return result, nil
	}
	return sources.BrowserResult{}, nil
}

func newAdapter(t *testing.T, browser sources.Browser) sources.Adapter {
	t.Helper()
	return bet365.New(sources.Deps{
		Browser: browser,
		Config:  sources.Config{BaseURL: "https://www.bet365.com"},
		Now:     func() time.Time { return sourcestest.FixedNow },
	})
}

func recordedBrowser(t *testing.T) *fixtureBrowser {
	return &fixtureBrowser{t: t, responses: map[string][]string{
		"/#/AC/B1/C1/D1002/G40/J99/Q1/F^72/": {"coupon.txt"},
		"/#/AC/B1/C1/D8/E151234001/F3/":      {"event_151234001.txt"},
		"/#/AC/B1/C1/D8/E151234002/F3/":      {"event_151234002.txt", "event_151234002_ws.txt"},
		"/#/AC/B1/C1/D8/E151234003/F3/":      {"event_151234003.txt"},
	}}
}

func TestAdapter_DiscoverGolden(t *testing.T) {
	t.Parallel()
	browser := recordedBrowser(t)
	snap, err := newAdapter(t, browser).Discover(context.Background(), []string{"EPL", "LALIGA"})
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(snap.Events) != 4 {
		t.Fatalf("Discover() returned %d events, want 4 (EPL + LALIGA only)", len(snap.Events))
	}
	if len(browser.requests) != 1 || browser.requests[0].Profile != "bet365" {
		t.Fatalf("browser requests = %+v, want one coupon fetch on the bet365 profile", browser.requests)
	}
	sourcestest.AssertGolden(t, source, "discover", snap)
}

func TestAdapter_PriceGolden(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t, recordedBrowser(t)).Price(context.Background(), []string{"151234001", "151234002", "151234003"})
	if err != nil {
		t.Fatalf("Price() error: %v", err)
	}
	if len(snap.Errors) != 0 {
		t.Fatalf("Price() snapshot errors: %v", snap.Errors)
	}
	sourcestest.RequireMarkets(t, snap,
		canon.Code1X2, canon.CodeDC, canon.CodeOU, canon.CodeBTTS, canon.CodeDNB, canon.CodeAH, canon.CodeCS,
		canon.CodeHTFT, canon.Code1H1X2, canon.CodeCornOU,
		canon.PropAnytime, canon.PropFirst, canon.PropLast, canon.PropShots, canon.PropSOT, canon.PropAssists,
		canon.PropFouls, canon.PropTackles, canon.PropCard,
	)
	sourcestest.AssertGolden(t, source, "price", snap)
}

func TestAdapter_ConfiguredCouponOverridesCompetitionLookup(t *testing.T) {
	t.Parallel()
	browser := &fixtureBrowser{t: t, responses: map[string][]string{"/#/AC/B1/C1/D1002/E999/G40/": {"coupon.txt"}}}
	adapter := bet365.New(sources.Deps{
		Browser: browser,
		Config:  sources.Config{BaseURL: "https://www.bet365.com/", LeagueIDs: map[string]string{"SPL": "#/AC/B1/C1/D1002/E999/G40/"}},
		Now:     func() time.Time { return sourcestest.FixedNow },
	})
	snap, err := adapter.Discover(context.Background(), []string{"SPL"})
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	for _, e := range snap.Events {
		if e.LeagueHint != "SPL" {
			t.Fatalf("event %s league = %q, want SPL from the configured coupon", e.SourceID, e.LeagueHint)
		}
	}
	if browser.requests[0].URL != "https://www.bet365.com/#/AC/B1/C1/D1002/E999/G40/" {
		t.Fatalf("coupon url = %q", browser.requests[0].URL)
	}
}

type failingBrowser struct{ err error }

func (f failingBrowser) Fetch(context.Context, sources.BrowserRequest) (sources.BrowserResult, error) {
	return sources.BrowserResult{}, f.err
}

func TestAdapter_DegradesWhenBrowserOrSourceIsDown(t *testing.T) {
	t.Parallel()
	browserdDown := errors.New("connection refused")
	tests := []struct {
		name    string
		browser sources.Browser
		wantErr error
	}{
		{name: "no browser configured", browser: nil, wantErr: bet365.ErrBrowserUnavailable},
		{name: "browserd unreachable", browser: failingBrowser{err: browserdDown}, wantErr: browserdDown},
		{name: "page blocked with no payload", browser: &fixtureBrowser{t: t}, wantErr: bet365.ErrNoOddsPayload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adapter := newAdapter(t, tt.browser)
			if _, err := adapter.Discover(context.Background(), []string{"EPL"}); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Discover() error = %v, want %v", err, tt.wantErr)
			}
			snap, err := adapter.Price(context.Background(), []string{"151234001", "151234002"})
			if !errors.Is(err, tt.wantErr) || len(snap.Errors) != 2 {
				t.Fatalf("Price() error = %v with %d snapshot errors, want %v for both events", err, len(snap.Errors), tt.wantErr)
			}
		})
	}
}

func TestAdapter_PriceKeepsHealthyEventsWhenOneFails(t *testing.T) {
	t.Parallel()
	snap, err := newAdapter(t, recordedBrowser(t)).Price(context.Background(), []string{"151234001", "404"})
	if err != nil {
		t.Fatalf("Price() error = %v, want partial success", err)
	}
	if len(snap.Errors) != 1 || len(snap.Markets) == 0 {
		t.Fatalf("Price() = %d markets, %d errors; want markets plus one error", len(snap.Markets), len(snap.Errors))
	}
}

func TestAdapter_MultiSportGoldens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		couponPath string
		couponFile string
		leagues    []string
		wantEvents int
		eventID    string
		eventPath  string
		eventFile  string
		wantCodes  []canon.MarketCode
	}{
		{
			name:       "basketball",
			couponPath: "/#/AC/B18/C1/D1002/G40/J99/Q1/F^72/",
			couponFile: "coupon_basketball.txt",
			leagues:    []string{"NBA", "EUROLEAGUE", "WNBA"},
			wantEvents: 3,
			eventID:    "B18-161000001",
			eventPath:  "/#/AC/B18/C1/D8/E161000001/F3/",
			eventFile:  "event_161000001.txt",
			wantCodes: []canon.MarketCode{
				canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal,
				canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees, canon.PropPtsRebAst,
			},
		},
		{
			name:       "american football",
			couponPath: "/#/AC/B12/C1/D1002/G40/J99/Q1/F^72/",
			couponFile: "coupon_americanfootball.txt",
			leagues:    []string{"NFL", "NCAAF"},
			wantEvents: 2,
			eventID:    "B12-171000001",
			eventPath:  "/#/AC/B12/C1/D8/E171000001/F3/",
			eventFile:  "event_171000001.txt",
			wantCodes: []canon.MarketCode{
				canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal,
				canon.PropPassYards, canon.PropRushYards, canon.PropRecYards, canon.PropReceptions, canon.PropPassTDs, canon.PropAnytimeTD,
			},
		},
		{
			name:       "tennis",
			couponPath: "/#/AC/B13/C1/D1002/G40/J99/Q1/F^72/",
			couponFile: "coupon_tennis.txt",
			leagues:    []string{"ATP_MIAMI", "WTA_MIAMI", "ATP_CH_ZADAR"},
			wantEvents: 3,
			eventID:    "B13-181000001",
			eventPath:  "/#/AC/B13/C1/D8/E181000001/F3/",
			eventFile:  "event_181000001.txt",
			wantCodes:  []canon.MarketCode{canon.CodeMoneyline, canon.CodeSetHcp, canon.CodeTotalGames, canon.PropAces, canon.PropDoubleFlt},
		},
		{
			name:       "esports",
			couponPath: "/#/AC/B151/C1/D1002/G40/J99/Q1/F^72/",
			couponFile: "coupon_esports.txt",
			leagues:    []string{"CS2", "DOTA2", "VALORANT", "LOL"},
			wantEvents: 4,
			eventID:    "B151-191000001",
			eventPath:  "/#/AC/B151/C1/D8/E191000001/F3/",
			eventFile:  "event_191000001.txt",
			wantCodes:  []canon.MarketCode{canon.CodeMoneyline, canon.CodeMapHcp, canon.CodeTotalMaps, canon.PropMapKills},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			browser := &fixtureBrowser{t: t, responses: map[string][]string{tt.couponPath: {tt.couponFile}, tt.eventPath: {tt.eventFile}}}
			adapter := newAdapter(t, browser)
			golden := strings.ReplaceAll(tt.name, " ", "_")

			discovered, err := adapter.Discover(context.Background(), tt.leagues)
			if err != nil {
				t.Fatalf("Discover() error: %v", err)
			}
			if len(discovered.Events) != tt.wantEvents {
				t.Fatalf("Discover() = %d events, want %d", len(discovered.Events), tt.wantEvents)
			}
			sourcestest.AssertGolden(t, source, "discover_"+golden, discovered)

			priced, err := adapter.Price(context.Background(), []string{tt.eventID})
			if err != nil {
				t.Fatalf("Price() error: %v", err)
			}
			sourcestest.RequireMarkets(t, priced, tt.wantCodes...)
			requireTwoWayMoneylines(t, priced)
			sourcestest.AssertGolden(t, source, "price_"+golden, priced)
		})
	}
}

func requireTwoWayMoneylines(t *testing.T, snap canon.Snapshot) {
	t.Helper()
	for _, m := range snap.Markets {
		if m.Code != canon.CodeMoneyline {
			continue
		}
		if len(m.Selections) != 2 || m.Rules.RegulationOnly {
			t.Fatalf("moneyline %s = %+v, want two sides settled including overtime", m.SourceMarketID, m)
		}
	}
}

func TestAdapter_PriceCupQualifyDropsRegulationThreeWay(t *testing.T) {
	t.Parallel()
	browser := &fixtureBrowser{t: t, responses: map[string][]string{"/#/AC/B1/C1/D8/E151234010/F3/": {"event_151234010.txt"}}}
	snap, err := newAdapter(t, browser).Price(context.Background(), []string{"151234010"})
	if err != nil {
		t.Fatalf("Price() error: %v", err)
	}
	if len(snap.Markets) != 1 || snap.Markets[0].Code != canon.CodeQualify || snap.Markets[0].Rules.RegulationOnly {
		t.Fatalf("Price() markets = %+v, want only QUALIFY settled including extra time", snap.Markets)
	}
}

func TestAdapter_DiscoverSkipsLeaguesOfOtherSports(t *testing.T) {
	t.Parallel()
	browser := &fixtureBrowser{t: t, responses: map[string][]string{"/#/AC/B18/C1/D1002/G40/J99/Q1/F^72/": {"coupon.txt"}}}
	snap, err := newAdapter(t, browser).Discover(context.Background(), []string{"NBA", "UNKNOWN"})
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	if len(snap.Events) != 0 || len(browser.requests) != 1 {
		t.Fatalf("Discover() = %d events from %d requests, want soccer fixtures on a basketball coupon ignored", len(snap.Events), len(browser.requests))
	}
}
