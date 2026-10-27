package bayse

import (
	"bufio"
	"bytes"
	"context"
	"math"
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

var fixtureGames = []string{"bm-game-20261024-ars-eve", "bm-game-20261023-ips-nfo", "bm-game-20261024-avl-mci", "bm-game-20261012-cov-new"}

var multiSportFixtureGames = []string{"bm-game-20261027-bvb-svw", "new-orleans-vs-utah-october-24-2026", "portland-vs-sacramento-october-24-2026", "houston-vs-san-antonio-october-24-2026"}

var footballMarkets = []canon.MarketCode{
	canon.Code1X2, canon.CodeBTTS, canon.CodeAH, canon.CodeOU, canon.CodeCornOU,
	canon.PropAnytime, canon.PropGoals, canon.PropShots, canon.PropAssists, canon.PropTackles, canon.PropPasses,
	canon.PropSaves, canon.PropRating,
	canon.TeamGoals, canon.TeamShots, canon.TeamPasses, canon.TeamCards,
}

var basketballMarkets = []canon.MarketCode{canon.CodeMoneyline, canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees}

var gameFixtureByLeague = map[string]string{
	defaultGameLeagueNames["EPL"]:      "games_england_premier_league.json",
	defaultGameLeagueNames["DFBPOKAL"]: "games_germany_dfb_pokal.json",
}

const (
	testPublicKey    = "pk_test_123"
	arsenalH2HMarket = "05652f33-75ac-42dc-9a79-3626462ae66f"
	clobBTTSMarket   = "ef285b37-c8b1-4db1-b76c-c5af0445469d"
	overTwoAndAHalf  = "d9ff4e26-a904-494d-a233-0b82a7ea175d"
)

func route(seenKeys chan<- string) sourcestest.Route {
	return func(r *http.Request) (string, bool) {
		select {
		case seenKeys <- r.Header.Get(publicKeyHeader):
		default:
		}
		q := r.URL.Query()
		switch r.URL.Path {
		case "/v1/pm/sports/games":
			if fixture, listed := gameFixtureByLeague[q.Get("league")]; listed {
				return fixture, true
			}
			return "games_empty.json", true
		case "/v1/pm/events":
			if subcategory := q.Get(subcategoryParam); subcategory != "" {
				return "events_subcategory_" + subcategory + "_p" + q.Get("page") + ".json", true
			}
			return "events_" + q.Get(sportGameIDParam) + "_p" + q.Get("page") + ".json", q.Get(sportGameIDParam) != ""
		case "/v1/pm/books":
			return "books.json", len(q[outcomeIDsParam]) > 0
		}
		return "", false
	}
}

func newTestAdapter(t *testing.T, cfg sources.Config, seenKeys chan<- string) *Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, Name, route(seenKeys))
	adapter, ok := New(sourcestest.Deps(srv, cfg)).(*Adapter)
	require.True(t, ok)
	return adapter
}

func TestAdapter_Discover(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Discover(context.Background(), []string{"EPL", "LALIGA", "ERED"})
	require.NoError(t, err)
	require.Len(t, snap.Events, 10)
	sourcestest.AssertGolden(t, Name, "discover", snap)
}

func TestAdapter_Price(t *testing.T) {
	t.Parallel()
	seenKeys := make(chan string, 1)
	snap, err := newTestAdapter(t, sources.Config{APIKey: testPublicKey}, seenKeys).Price(context.Background(), fixtureGames)
	require.NoError(t, err)
	assert.Equal(t, testPublicKey, <-seenKeys)
	sourcestest.RequireMarkets(t, snap, footballMarkets...)
	sourcestest.AssertGolden(t, Name, "price", snap)
}

func TestAdapter_DiscoverMultiSport(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Discover(context.Background(), []string{"DFBPOKAL", "NBA", "NFL"})
	require.NoError(t, err)
	byLeague := map[string]int{}
	for _, ev := range snap.Events {
		byLeague[ev.LeagueHint]++
	}
	assert.Equal(t, map[string]int{"DFBPOKAL": 4, "NBA": 4}, byLeague)
	sourcestest.AssertGolden(t, Name, "discover_multisport", snap)
}

func TestAdapter_PriceMultiSport(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Price(context.Background(), multiSportFixtureGames)
	require.NoError(t, err)
	sourcestest.RequireMarkets(t, snap, basketballMarkets...)
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.CodeBTTS, canon.CodeOU, canon.CodeAH)
	sourcestest.AssertGolden(t, Name, "price_multisport", snap)
}

func TestAdapter_PriceMapsNBAFromKalshiMirror(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Price(context.Background(), multiSportFixtureGames[1:2])
	require.NoError(t, err)
	byCode := map[canon.MarketCode][]canon.RawMarket{}
	for _, m := range snap.Markets {
		assert.Equal(t, "new-orleans-vs-utah-october-24-2026", m.EventSourceID)
		byCode[m.Code] = append(byCode[m.Code], m)
	}
	t.Run("moneyline sides follow the away vs home title", func(t *testing.T) {
		require.Len(t, byCode[canon.CodeMoneyline], 1)
		ml := byCode[canon.CodeMoneyline][0]
		assert.False(t, ml.Rules.RegulationOnly)
		utah := findSelection(t, snap.Markets, ml.SourceMarketID, canon.SideHome)
		assert.InDelta(t, 73.41, utah.Price.Raw, 1e-9)
	})
	t.Run("ladder props become over half a point below the threshold", func(t *testing.T) {
		require.NotEmpty(t, byCode[canon.PropPoints])
		for _, m := range byCode[canon.PropPoints] {
			require.NotNil(t, m.Player)
			require.NotNil(t, m.Line)
			assert.InDelta(t, 0.5, math.Mod(*m.Line, 1), 1e-9)
			assert.True(t, m.Rules.VoidIfNoStart)
			assert.Equal(t, []canon.Side{canon.SideOver, canon.SideUnder}, []canon.Side{m.Selections[0].Side, m.Selections[1].Side})
		}
	})
}

func TestAdapter_DiscoverListedSkipsStartedGames(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, sources.Config{}, nil)
	a.deps.Now = func() time.Time { return time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC) }
	snap, err := a.Discover(context.Background(), []string{"NBA"})
	require.NoError(t, err)
	require.Len(t, snap.Events, 1)
	assert.Equal(t, "brooklyn-vs-boston-october-28-2026", snap.Events[0].SourceID)
}

func TestAdapter_PriceMapsBayseSpecifics(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Price(context.Background(), fixtureGames)
	require.NoError(t, err)
	byCode := map[canon.MarketCode][]canon.RawMarket{}
	for _, m := range snap.Markets {
		byCode[m.Code] = append(byCode[m.Code], m)
	}
	t.Run("team stat buckets carry range labels", func(t *testing.T) {
		require.NotEmpty(t, byCode[canon.TeamShots])
		var labels []string
		for _, sel := range byCode[canon.TeamShots][0].Selections {
			labels = append(labels, sel.Label)
		}
		assert.ElementsMatch(t, []string{"<21", "21-28", "29+"}, labels)
	})
	t.Run("team goals name the team from the title", func(t *testing.T) {
		require.NotEmpty(t, byCode[canon.TeamGoals])
		teams := map[string]bool{}
		for _, m := range byCode[canon.TeamGoals] {
			require.NotNil(t, m.Line)
			assert.NotEmpty(t, m.Team)
			assert.Equal(t, []canon.Side{canon.SideOver, canon.SideUnder}, []canon.Side{m.Selections[0].Side, m.Selections[1].Side})
			teams[m.Team] = true
		}
		assert.True(t, teams["Aston Villa FC"])
		assert.True(t, teams["Manchester City FC"])
	})
	t.Run("saves and passes are player over unders", func(t *testing.T) {
		for _, code := range []canon.MarketCode{canon.PropSaves, canon.PropPasses} {
			require.NotEmpty(t, byCode[code], code)
			for _, m := range byCode[code] {
				require.NotNil(t, m.Player)
				require.NotNil(t, m.Line)
				assert.Equal(t, "opta", m.Rules.Resolver)
			}
		}
	})
	t.Run("sofascore rating voids without a start", func(t *testing.T) {
		require.NotEmpty(t, byCode[canon.PropRating])
		for _, m := range byCode[canon.PropRating] {
			assert.True(t, m.Rules.VoidIfNoStart)
			assert.Equal(t, "sofascore", m.Rules.Resolver)
			require.NotNil(t, m.Player)
			require.NotNil(t, m.Line)
		}
	})
	t.Run("amm prices are naira", func(t *testing.T) {
		home := findSelection(t, snap.Markets, arsenalH2HMarket, canon.SideHome)
		assert.Equal(t, canon.FormNaira, home.Price.Form)
		assert.InDelta(t, 69.52, home.Price.Raw, 1e-9)
		assert.InDelta(t, 0.6952, home.Price.Prob, 1e-9)
	})
	t.Run("every selection links to its own bayse market", func(t *testing.T) {
		for _, m := range snap.Markets {
			for _, sel := range m.Selections {
				assert.True(t, strings.HasPrefix(sel.DeepLink, "https://bayse.markets/market/"), "%s %s", m.SourceMarketID, sel.DeepLink)
				assert.Greater(t, len(sel.DeepLink), len("https://bayse.markets/market/"))
			}
		}
	})
	t.Run("clob markets use the book touch", func(t *testing.T) {
		yes := findSelection(t, snap.Markets, clobBTTSMarket, canon.SideYes)
		require.NotNil(t, yes.Price.Ask)
		assert.InDelta(t, 49, *yes.Price.Ask, 1e-9)
		assert.InDelta(t, 0.49*200, *yes.Price.Liquidity, 1e-9)
	})
}

func TestBoard_ApplyRecordedFrames(t *testing.T) {
	t.Parallel()
	b, err := newTestAdapter(t, sources.Config{}, nil).loadBoard(context.Background(), fixtureGames[:1])
	require.NoError(t, err)
	var updates [][]canon.RawMarket
	for _, frame := range readFrames(t) {
		changed, err := b.apply(frame, sourcestest.FixedNow)
		require.NoError(t, err)
		if len(changed) > 0 {
			updates = append(updates, changed)
		}
	}
	require.Len(t, updates, 3)
	home := findSelection(t, updates[0], arsenalH2HMarket, canon.SideHome)
	assert.InDelta(t, 71, home.Price.Raw, 1e-9)
	yes := findSelection(t, updates[1], clobBTTSMarket, canon.SideYes)
	assert.InDelta(t, 50, *yes.Price.Ask, 1e-9)
	assert.InDelta(t, 48, *yes.Price.Bid, 1e-9)
	under := findSelection(t, updates[2], overTwoAndAHalf, canon.SideUnder)
	assert.InDelta(t, 49, under.Price.Raw, 1e-9)
}

func TestAdapter_StreamSubscribesToPricesAndOrderbooks(t *testing.T) {
	t.Parallel()
	frames := readFrames(t)
	subs := make(chan wsSubscribe, 64)
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		go func() {
			for {
				_, msg, err := conn.Read(r.Context())
				if err != nil {
					return
				}
				var sub wsSubscribe
				if json.Unmarshal(msg, &sub) == nil {
					subs <- sub
				}
			}
		}()
		for _, frame := range frames {
			if conn.Write(r.Context(), websocket.MessageText, frame) != nil {
				return
			}
		}
		<-r.Context().Done()
	}))
	t.Cleanup(wsSrv.Close)
	a := newTestAdapter(t, sources.Config{Extra: map[string]string{"wsUrl": "ws" + strings.TrimPrefix(wsSrv.URL, "http")}}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out := make(chan canon.Snapshot, 8)
	done := make(chan error, 1)
	go func() { done <- a.Stream(ctx, fixtureGames[:1], out) }()
	assert.NotEmpty(t, (<-out).Markets)
	assert.NotEmpty(t, (<-out).Markets)
	first := <-subs
	assert.Equal(t, pricesChannel, first.Channel)
	assert.NotEmpty(t, first.EventID)
	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}

func TestBucketLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		title    string
		expected string
		ok       bool
	}{
		{name: "lower bucket", title: "Less than 524", expected: "<524", ok: true},
		{name: "middle bucket", title: "524 - 635", expected: "524-635", ok: true},
		{name: "upper bucket with words", title: " 636 or more", expected: "636+", ok: true},
		{name: "upper bucket with plus", title: "32+", expected: "32+", ok: true},
		{name: "unknown shape", title: "About 30", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := bucketLabel(tt.title)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestTeamOfTeamStat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		code     canon.MarketCode
		title    string
		expected string
	}{
		{name: "team goals", code: canon.TeamGoals, title: "Manchester City FC: Total Goals", expected: "Manchester City FC"},
		{name: "team goals any case", code: canon.TeamGoals, title: " Arsenal FC: total goals ", expected: "Arsenal FC"},
		{name: "team goals without team", code: canon.TeamGoals, title: "Total Goals", expected: ""},
		{name: "team passes", code: canon.TeamPasses, title: "How many passes will Arsenal FC get vs Everton FC?", expected: "Arsenal FC"},
		{name: "match wide code has no team", code: canon.TeamCards, title: "Arsenal FC: Total Goals", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, teamOfTeamStat(tt.code, tt.title))
		})
	}
}

func readFrames(t *testing.T) [][]byte {
	t.Helper()
	var frames [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(sourcestest.ReadFixture(t, Name, "ws_frames.jsonl")))
	for scanner.Scan() {
		line := bytes.Clone(scanner.Bytes())
		if bytes.HasPrefix(line, []byte(`"`)) {
			var batched string
			require.NoError(t, json.Unmarshal(line, &batched))
			line = []byte(batched)
		}
		frames = append(frames, line)
	}
	require.NoError(t, scanner.Err())
	return frames
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

func TestHandicapSideComesFromRulesNotTitle(t *testing.T) {
	const frosinoneRules = "If Frosinone wins by more than 1.5 goals in the Atalanta vs Frosinone professional Serie A soccer game"
	tests := []struct {
		name         string
		title        string
		rules        string
		propLine     float64
		expectedLine float64
		firstSide    canon.Side
		isMapped     bool
	}{
		{name: "away winner from rules despite home code in title", title: "ATA by 2+ Goals", rules: frosinoneRules, propLine: 1.5, expectedLine: 1.5, firstSide: canon.SideAway, isMapped: true},
		{name: "home winner", title: "ATA by 3+ Goals", rules: "If Atalanta wins by more than 2.5 goals in the Atalanta vs Frosinone professional Serie A soccer game", propLine: 2.5, expectedLine: -2.5, firstSide: canon.SideHome, isMapped: true},
		{name: "rules margin disagrees with prop line", title: "FRO by 3+ Goals", rules: frosinoneRules, propLine: 2.5, isMapped: false},
		{name: "unparseable rules", title: "FRO by 2+ Goals", rules: "Resolves yes if Frosinone wins by two", propLine: 1.5, isMapped: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newMatchBuilder("bm-game-20261025-ata-fro", matchCodes{home: "ata", away: "fro"}, func(string) {})
			m := market{ID: "m", Title: tt.title, Rules: tt.rules, PropLine: &tt.propLine, Outcome1ID: "yes", Outcome2ID: "no"}
			require.Equal(t, tt.isMapped, b.addHandicap(m, sportMarketSpecs["GOAL_SPREAD"]))
			if !tt.isMapped {
				return
			}
			raw := b.markets[0].raw
			assert.Equal(t, tt.expectedLine, *raw.Line)
			assert.Equal(t, tt.firstSide, raw.Selections[0].Side)
		})
	}
}

func TestDuplicateMarketKeepsMostLiquidEvent(t *testing.T) {
	line := 9.5
	single := event{ID: "single", Status: openStatus, SportMarketType: "TOTAL_CORNERS", Liquidity: 100, Markets: []market{{ID: "single-m", Status: openStatus, Title: "10+ corners", PropLine: &line}}}
	grouped := event{ID: "grouped", Status: openStatus, SportMarketType: "TOTAL_CORNERS", Liquidity: 1000, Markets: []market{{ID: "grouped-m", Status: openStatus, Title: "10+ Total Corners", PropLine: &line}}}
	b := newMatchBuilder("bm-game-20261024-ars-eve", matchCodes{home: "ars", away: "eve"}, func(string) {})
	for _, ev := range mostLiquidFirst([]event{single, grouped}) {
		b.addEvent(ev)
	}
	require.Len(t, b.markets, 1)
	assert.Equal(t, "grouped-m", b.markets[0].raw.SourceMarketID)
}
