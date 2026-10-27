package kalshi

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

var fixtureGames = []string{"KXEPLGAME-26OCT24ARSEVE", "KXEPLGAME-26OCT24CFCTOT", "KXEPLGAME-26OCT25LFCBRI"}

func routeFor(t *testing.T) sourcestest.Route {
	dir := sourcestest.TestdataDir(t, Name)
	return func(r *http.Request) (string, bool) {
		if r.URL.Path != "/events" {
			return "", false
		}
		kind := "markets_"
		if r.URL.Query().Get("with_milestones") == "true" {
			kind = "events_"
		}
		name := kind + r.URL.Query().Get("series_ticker") + ".json"
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return "events_empty.json", true
		}
		return name, true
	}
}

func newTestAdapter(t *testing.T, cfg sources.Config) *Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, Name, routeFor(t))
	adapter, ok := New(sourcestest.Deps(srv, cfg)).(*Adapter)
	require.True(t, ok)
	return adapter
}

func TestAdapter_Discover(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}).Discover(context.Background(), []string{"EPL", "LALIGA"})
	require.NoError(t, err)
	require.Len(t, snap.Events, 8)
	sourcestest.AssertGolden(t, Name, "discover", snap)
}

func TestAdapter_Price(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}).Price(context.Background(), fixtureGames)
	require.NoError(t, err)
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.Code1H1X2, canon.CodeCS, canon.CodeOU, canon.CodeCornOU, canon.CodeBTTS, canon.CodeAH)
	sourcestest.AssertGolden(t, Name, "price", snap)
}

func TestAdapter_PriceMapsTeamTotalsToTheirTeam(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}).Price(context.Background(), []string{"KXSERIEAGAME-26OCT28MONNAP"})
	require.NoError(t, err)
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.TeamGoals)
	tests := []struct {
		name         string
		ticker       string
		expectedTeam string
		expectedLine *float64
	}{
		{name: "home team over half a goal", ticker: "KXSERIEATEAMTOTAL-26OCT28MONNAP-MON1", expectedTeam: "Monza", expectedLine: canon.Line(0.5)},
		{name: "away team over two and a half", ticker: "KXSERIEATEAMTOTAL-26OCT28MONNAP-NAP3", expectedTeam: "Napoli", expectedLine: canon.Line(2.5)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var found *canon.RawMarket
			for i := range snap.Markets {
				if snap.Markets[i].SourceMarketID == tt.ticker {
					found = &snap.Markets[i]
				}
			}
			require.NotNil(t, found)
			assert.Equal(t, canon.TeamGoals, found.Code)
			assert.Equal(t, tt.expectedTeam, found.Team)
			assert.Equal(t, tt.expectedLine, found.Line)
			assert.Equal(t, canon.PeriodFullTime, found.Period)
			require.Len(t, found.Selections, 2)
			assert.Equal(t, canon.SideOver, found.Selections[0].Side)
			assert.Equal(t, canon.SideUnder, found.Selections[1].Side)
		})
	}
}

func TestAdapter_PriceConvertsDollarsToCents(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}).Price(context.Background(), fixtureGames[:1])
	require.NoError(t, err)
	over := findSelection(t, snap.Markets, "KXEPLTOTAL-26OCT24ARSEVE-3", canon.SideOver)
	under := findSelection(t, snap.Markets, "KXEPLTOTAL-26OCT24ARSEVE-3", canon.SideUnder)
	assert.Equal(t, canon.FormCents, over.Price.Form)
	require.NotNil(t, over.Price.Ask)
	require.NotNil(t, under.Price.Ask)
	assert.InDelta(t, 100, *over.Price.Ask+*under.Price.Bid, 1e-9)
	assert.InDelta(t, *over.Price.Ask/100, over.Price.Prob, 1e-9)
}

func TestAdapter_PriceRejectsNonGameTicker(t *testing.T) {
	t.Parallel()
	_, err := newTestAdapter(t, sources.Config{}).Price(context.Background(), []string{"KXEPLTOTAL-26OCT24ARSEVE"})
	assert.ErrorIs(t, err, errNotGameTicker)
}

func TestBoard_ApplyRecordedFrames(t *testing.T) {
	t.Parallel()
	b, err := newTestAdapter(t, sources.Config{}).loadBoard(context.Background(), fixtureGames[:1])
	require.NoError(t, err)
	var updates [][]canon.RawMarket
	var streamErrs int
	for _, frame := range readFrames(t) {
		changed, err := b.apply(frame, sourcestest.FixedNow)
		if err != nil {
			streamErrs++
			continue
		}
		if len(changed) > 0 {
			updates = append(updates, changed)
		}
	}
	require.Len(t, updates, 2)
	assert.Equal(t, 1, streamErrs)
	home := findSelection(t, updates[0], "KXEPLGAME-26OCT24ARSEVE", canon.SideHome)
	assert.InDelta(t, 69, *home.Price.Ask, 1e-9)
	assert.InDelta(t, 0.69*800, *home.Price.Liquidity, 1e-9)
	under := findSelection(t, updates[1], "KXEPLTOTAL-26OCT24ARSEVE-3", canon.SideUnder)
	assert.InDelta(t, 48, *under.Price.Ask, 1e-9)
	assert.InDelta(t, 46, *under.Price.Bid, 1e-9)
}

func TestQuote_NoSideWithoutYesBidIsSuspended(t *testing.T) {
	t.Parallel()
	p := quote{yesAsk: 0.02, yesAskSize: 10}.price(outcomeNo, sourcestest.FixedNow)
	assert.True(t, p.Suspended)
	assert.Nil(t, p.Ask)
	require.NotNil(t, p.Bid)
	assert.InDelta(t, 98, *p.Bid, 1e-9)
}

func TestAdapter_StreamWithoutKeyAsksRuntimeToPoll(t *testing.T) {
	t.Parallel()
	err := newTestAdapter(t, sources.Config{}).Stream(context.Background(), fixtureGames, make(chan canon.Snapshot))
	assert.ErrorIs(t, err, ErrStreamNeedsAPIKey)
}

func TestAdapter_StreamSignsHandshakeAndRelaysTicker(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	frames := readFrames(t)
	handshakeOK := make(chan bool, 1)
	commands := make(chan wsCommand, 1)
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handshakeOK <- verifyHandshake(&key.PublicKey, r)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		_, msg, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		var cmd wsCommand
		if json.Unmarshal(msg, &cmd) == nil {
			commands <- cmd
		}
		for _, frame := range frames {
			if conn.Write(r.Context(), websocket.MessageText, frame) != nil {
				return
			}
		}
		<-r.Context().Done()
	}))
	t.Cleanup(wsSrv.Close)
	a := newTestAdapter(t, sources.Config{
		APIKey:    "key-id",
		APISecret: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		Extra:     map[string]string{"wsUrl": "ws" + strings.TrimPrefix(wsSrv.URL, "http") + "/trade-api/ws/v2"},
	})
	a.deps.Now = time.Now
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := make(chan canon.Snapshot, 8)
	done := make(chan error, 1)
	go func() { done <- a.Stream(ctx, fixtureGames[:1], out) }()
	assert.NotEmpty(t, (<-out).Markets)
	assert.True(t, <-handshakeOK)
	cmd := <-commands
	assert.Equal(t, []string{tickerChannel}, cmd.Params.Channels)
	assert.Contains(t, cmd.Params.MarketTickers, "KXEPLGAME-26OCT24ARSEVE-ARS")
	assert.NotEmpty(t, (<-out).Markets)
	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}

func verifyHandshake(pub *rsa.PublicKey, r *http.Request) bool {
	signature, err := base64.StdEncoding.DecodeString(r.Header.Get(headerAccessSignature))
	if err != nil || r.Header.Get(headerAccessKey) != "key-id" {
		return false
	}
	digest := sha256.Sum256([]byte(r.Header.Get(headerAccessTimestamp) + http.MethodGet + r.URL.Path))
	return rsa.VerifyPSS(pub, crypto.SHA256, digest[:], signature, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) == nil
}

func readFrames(t *testing.T) [][]byte {
	t.Helper()
	var frames [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(sourcestest.ReadFixture(t, Name, "ws_frames.jsonl")))
	for scanner.Scan() {
		frames = append(frames, bytes.Clone(scanner.Bytes()))
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

func TestBuildEventMarkets_MapSpreadPinsHomeLine(t *testing.T) {
	t.Parallel()
	legacy, liquid := "21afa91e-d459-4f13-8d61-39a296bf8579", "d78e2e45-83ec-4692-92a0-f69b7cdf88b2"
	players := lineup{
		home: competitor{code: "LGC", teamID: legacy, name: "Legacy"},
		away: competitor{code: "TL", teamID: liquid, name: "Liquid"},
	}
	margin := 1.5
	ev := event{EventTicker: "KXCS2SPREAD-26OCT261000LGCTL", Title: "Legacy vs. Liquid: Spread", Markets: []market{
		{Ticker: "KXCS2SPREAD-26OCT261000LGCTL-LGC2", Status: activeStatus, FloorStrike: &margin, StrikeType: strikeGreater, CustomStrike: customStrike{EsportsCompetitor: legacy}, YesSubTitle: "Legacy wins by over 1.5 maps", YesAsk: 0.01},
		{Ticker: "KXCS2SPREAD-26OCT261000LGCTL-TL2", Status: activeStatus, FloorStrike: &margin, StrikeType: strikeGreater, CustomStrike: customStrike{EsportsCompetitor: liquid}, YesSubTitle: "Liquid wins by over 1.5 maps", YesAsk: 0.65, YesBid: 0.5},
	}}
	spec := seriesSpec{"SPREAD", "", canon.CodeMapHcp, canon.PeriodFullTime, shapeHandicap}
	markets := buildEventMarkets("KXCS2GAME-26OCT261000LGCTL", ev, spec, players, func(string) {})
	require.Len(t, markets, 2)
	byTicker := map[string]canon.RawMarket{}
	for _, pm := range markets {
		byTicker[pm.raw.SourceMarketID] = pm.raw
	}
	legacyByTwo := byTicker["KXCS2SPREAD-26OCT261000LGCTL-LGC2"]
	assert.InDelta(t, -1.5, *legacyByTwo.Line, 1e-9)
	assert.Equal(t, []canon.Side{canon.SideHome, canon.SideAway}, []canon.Side{legacyByTwo.Selections[0].Side, legacyByTwo.Selections[1].Side})
	liquidByTwo := byTicker["KXCS2SPREAD-26OCT261000LGCTL-TL2"]
	assert.InDelta(t, 1.5, *liquidByTwo.Line, 1e-9)
	assert.Equal(t, []canon.Side{canon.SideAway, canon.SideHome}, []canon.Side{liquidByTwo.Selections[0].Side, liquidByTwo.Selections[1].Side})
}
