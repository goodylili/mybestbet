package polymarket

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var (
	fixtureMatches         = []string{"epl-ars-eve-2026-10-24", "epl-che-tot-2026-10-24", "epl-liv-bri-2026-10-25"}
	multiSportMatches      = []string{"nba-mia-cha-2026-10-24", "nfl-la-lv-2026-10-25", "atp-forti-batalla-2026-10-23", "cs2-mgc-big5-2026-10-27"}
	recordedSeries         = map[string]bool{"10188": true, "10345": true, "12185": true, "10365": true, "10310": true}
	multiSportLeagues      = []string{"NBA", "NFL", "ATP_CH_ZADAR", "WTA_MIAMI", "CS2"}
	florenceAsZadarLeagues = map[string]string{"ATP_CH_ZADAR": "atp:Florence"}
	multiSportMarkets      = []canon.MarketCode{
		canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeSetHcp, canon.CodeTotalGames, canon.CodeMapHcp, canon.CodeTotalMaps,
		canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees,
		canon.PropPassYards, canon.PropRushYards, canon.PropRecYards, canon.PropReceptions, canon.PropPassTDs, canon.PropAnytimeTD,
	}
)

const arsenalYesToken = "114460869056021695126097471762228510096841940195417663873021964330022095891387"

func route(r *http.Request) (string, bool) {
	switch {
	case r.URL.Path == "/sports":
		return "sports.json", true
	case r.URL.Path == "/books" && r.Method == http.MethodPost:
		return "books.json", true
	case r.URL.Path == "/events" && recordedSeries[r.URL.Query().Get("series_id")]:
		return "events_series_" + r.URL.Query().Get("series_id") + ".json", true
	case r.URL.Path == "/events" && r.URL.Query().Has("series_id"):
		return "events_empty.json", true
	case r.URL.Path == "/events" && r.URL.Query().Has("slug"):
		return "match_" + r.URL.Query()["slug"][0] + ".json", true
	}
	return "", false
}

func newTestAdapter(t *testing.T, extra map[string]string) *Adapter {
	t.Helper()
	return newTestAdapterWithLeagues(t, extra, nil)
}

func newTestAdapterWithLeagues(t *testing.T, extra, leagueIDs map[string]string) *Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, Name, route)
	cfg := sources.Config{Extra: map[string]string{"clobUrl": srv.URL}, LeagueIDs: leagueIDs}
	for k, v := range extra {
		cfg.Extra[k] = v
	}
	adapter, ok := New(sourcestest.Deps(srv, cfg)).(*Adapter)
	require.True(t, ok)
	return adapter
}

func TestAdapter_Discover(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, nil).Discover(context.Background(), []string{"EPL", "LALIGA"})
	require.NoError(t, err)
	require.NotEmpty(t, snap.Events)
	for _, ev := range snap.Events {
		assert.Equal(t, "EPL", ev.LeagueHint)
		assert.NotContains(t, ev.SourceID, "halftime")
	}
	sourcestest.AssertGolden(t, Name, "discover", snap)
}

func TestAdapter_Price(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, nil).Price(context.Background(), fixtureMatches)
	require.NoError(t, err)
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.Code1H1X2, canon.CodeCS, canon.CodeOU, canon.CodeCornOU, canon.CodeBTTS, canon.CodeAH,
		canon.PropAnytime, canon.PropGA, canon.PropGoals, canon.PropShots, canon.PropSOT, canon.PropAssists, canon.PropSaves, canon.TeamGoals)
	sourcestest.AssertGolden(t, Name, "price", snap)
}

func TestAdapter_PriceNamesTeamOfTeamTotals(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, nil).Price(context.Background(), fixtureMatches)
	require.NoError(t, err)
	tests := []struct {
		name           string
		sourceMarketID string
		period         string
		expectedTeam   string
		expectedLine   *float64
	}{
		{name: "home full time", sourceMarketID: "epl-ars-eve-2026-10-24-team-total-home-1pt5", expectedTeam: "Arsenal FC", expectedLine: canon.Line(1.5)},
		{name: "away full time", sourceMarketID: "epl-ars-eve-2026-10-24-team-total-away-0pt5", expectedTeam: "Everton FC", expectedLine: canon.Line(0.5)},
		{name: "home first half", sourceMarketID: "epl-ars-eve-2026-10-24-first-half-team-total-home-0pt5", period: canon.PeriodFirstHalf, expectedTeam: "Arsenal FC", expectedLine: canon.Line(0.5)},
		{name: "away second half", sourceMarketID: "epl-ars-eve-2026-10-24-second-half-team-total-away-2pt5", period: canon.PeriodSecondHalf, expectedTeam: "Everton FC", expectedLine: canon.Line(2.5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := findMarket(t, snap.Markets, tt.sourceMarketID)
			assert.Equal(t, canon.TeamGoals, m.Code)
			assert.Equal(t, tt.period, m.Period)
			assert.Equal(t, tt.expectedTeam, m.Team)
			assert.Equal(t, tt.expectedLine, m.Line)
			assert.Equal(t, []canon.Side{canon.SideOver, canon.SideUnder}, []canon.Side{m.Selections[0].Side, m.Selections[1].Side})
		})
	}
	for _, m := range snap.Markets {
		assert.NotContains(t, m.SourceMarketID, "-corners-team-", "team corners have no canonical code")
	}
}

func TestAdapter_DiscoverMultiSport(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapterWithLeagues(t, nil, florenceAsZadarLeagues).Discover(context.Background(), multiSportLeagues)
	require.NoError(t, err)
	leagues := map[string]bool{}
	for _, ev := range snap.Events {
		leagues[ev.LeagueHint] = true
		if ev.LeagueHint == "ATP_CH_ZADAR" {
			assert.Equal(t, "atp-forti-batalla-2026-10-23", ev.SourceID)
		}
	}
	assert.Equal(t, map[string]bool{"NBA": true, "NFL": true, "ATP_CH_ZADAR": true, "CS2": true}, leagues)
	sourcestest.AssertGolden(t, Name, "discover_multisport", snap)
}

func TestAdapter_PriceMultiSport(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, nil).Price(context.Background(), multiSportMatches)
	require.NoError(t, err)
	sourcestest.RequireMarkets(t, snap, multiSportMarkets...)
	sourcestest.AssertGolden(t, Name, "price_multisport", snap)
}

func TestAdapter_PriceKeepsHomeHandicapAndSides(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, nil).Price(context.Background(), multiSportMatches)
	require.NoError(t, err)
	tests := []struct {
		name           string
		sourceMarketID string
		code           canon.MarketCode
		period         string
		expectedLine   *float64
		expectedFirst  canon.Side
	}{
		{name: "nba spread owned by away favourite", sourceMarketID: "nba-mia-cha-2026-10-24-spread-away-3pt5", code: canon.CodeSpread, expectedLine: canon.Line(3.5), expectedFirst: canon.SideAway},
		{name: "nba first half moneyline", sourceMarketID: "nba-mia-cha-2026-10-24-1h-moneyline", code: canon.CodeMoneyline, period: canon.PeriodFirstHalf, expectedFirst: canon.SideAway},
		{name: "cs2 map handicap owned by away", sourceMarketID: "cs2-mgc-big5-2026-10-27-map-handicap-home-1pt5", code: canon.CodeMapHcp, expectedLine: canon.Line(1.5), expectedFirst: canon.SideAway},
		{name: "cs2 map one winner", sourceMarketID: "cs2-mgc-big5-2026-10-27-game1", code: canon.CodeMoneyline, period: canon.PeriodFirstMap, expectedFirst: canon.SideHome},
		{name: "tennis set handicap", sourceMarketID: "atp-forti-batalla-2026-10-23-set-handicap-home-1pt5", code: canon.CodeSetHcp, expectedLine: canon.Line(-1.5), expectedFirst: canon.SideHome},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := findMarket(t, snap.Markets, tt.sourceMarketID)
			assert.Equal(t, tt.code, m.Code)
			assert.Equal(t, tt.period, m.Period)
			assert.Equal(t, tt.expectedLine, m.Line)
			assert.Equal(t, tt.expectedFirst, m.Selections[0].Side)
			assert.False(t, m.Rules.RegulationOnly)
		})
	}
	for _, m := range snap.Markets {
		assert.NotContains(t, m.SourceMarketID, "-game2", "only map one winner is canonical")
	}
}

func TestAdapter_PriceUsesYesAskForOneXTwo(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, nil).Price(context.Background(), fixtureMatches[:1])
	require.NoError(t, err)
	home := findSelection(t, snap.Markets, "epl-ars-eve-2026-10-24", canon.SideHome)
	require.NotNil(t, home.Price.Ask)
	assert.InDelta(t, 0.67, *home.Price.Ask, 1e-9)
	assert.InDelta(t, 0.67, home.Price.Prob, 1e-9)
	assert.Equal(t, canon.FormShare, home.Price.Form)
}

func TestBoard_ApplyRecordedFrames(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, nil)
	b, err := a.loadBoard(context.Background(), fixtureMatches[:1])
	require.NoError(t, err)
	var lastChanged []canon.RawMarket
	for _, frame := range readFrames(t) {
		changed, err := b.apply(frame, sourcestest.FixedNow)
		require.NoError(t, err)
		if len(changed) > 0 {
			lastChanged = changed
		}
	}
	home := findSelection(t, lastChanged, "epl-ars-eve-2026-10-24", canon.SideHome)
	require.NotNil(t, home.Price.Ask)
	assert.InDelta(t, 0.68, *home.Price.Ask, 1e-9)
	assert.InDelta(t, 0.68*102672.3, *home.Price.Liquidity, 1e-6)
}

func TestBoard_ApplyIgnoresUntrackedAssetsAndPong(t *testing.T) {
	t.Parallel()
	b := newBoard(nil, sourcestest.FixedNow)
	tests := []struct {
		name  string
		frame string
	}{
		{name: "pong", frame: "PONG"},
		{name: "untracked book", frame: `{"event_type":"book","asset_id":"1","bids":[],"asks":[]}`},
		{name: "untracked change", frame: `{"event_type":"price_change","price_changes":[{"asset_id":"1","price":"0.5","size":"1","side":"BUY"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := b.apply([]byte(tt.frame), sourcestest.FixedNow)
			require.NoError(t, err)
			assert.Empty(t, changed)
		})
	}
}

func TestAdapter_StreamSubscribesAndRelaysFrames(t *testing.T) {
	t.Parallel()
	frames := readFrames(t)
	subscriptions := make(chan wsSubscription, 1)
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_, msg, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var sub wsSubscription
		if err := json.Unmarshal(msg, &sub); err == nil {
			subscriptions <- sub
		}
		for _, frame := range frames {
			if err := conn.Write(r.Context(), websocket.MessageText, frame); err != nil {
				return
			}
		}
		<-r.Context().Done()
	}))
	t.Cleanup(wsSrv.Close)
	a := newTestAdapter(t, map[string]string{"wsUrl": "ws" + strings.TrimPrefix(wsSrv.URL, "http")})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := make(chan canon.Snapshot, 16)
	done := make(chan error, 1)
	go func() { done <- a.Stream(ctx, fixtureMatches[:1], out) }()
	resync := <-out
	assert.NotEmpty(t, resync.Markets)
	sub := <-subscriptions
	assert.Equal(t, marketChannel, sub.Type)
	assert.Contains(t, sub.AssetIDs, arsenalYesToken)
	update := <-out
	assert.NotEmpty(t, update.Markets)
	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}

func readFrames(t *testing.T) [][]byte {
	t.Helper()
	var frames [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(sourcestest.ReadFixture(t, Name, "ws_frames.jsonl")))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		frames = append(frames, bytes.Clone(scanner.Bytes()))
	}
	require.NoError(t, scanner.Err())
	return frames
}

func findMarket(t *testing.T, markets []canon.RawMarket, sourceMarketID string) canon.RawMarket {
	t.Helper()
	for _, m := range markets {
		if m.SourceMarketID == sourceMarketID {
			return m
		}
	}
	t.Fatalf("market %s not found", sourceMarketID)
	return canon.RawMarket{}
}

func findSelection(t *testing.T, markets []canon.RawMarket, sourceMarketID string, side canon.Side) canon.Selection {
	t.Helper()
	for _, m := range markets {
		if m.SourceMarketID != sourceMarketID {
			continue
		}
		for _, sel := range m.Selections {
			if sel.Side == side {
				return sel
			}
		}
	}
	t.Fatalf("selection %s/%s not found", sourceMarketID, side)
	return canon.Selection{}
}
