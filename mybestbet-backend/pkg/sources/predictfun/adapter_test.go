package predictfun

import (
	"bufio"
	"bytes"
	"context"
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

var fixtureMatches = []string{"epl-liv-bri-2026-10-25", "lal-bar-rea-2026-10-25", "fl1-psg-lyo-2026-10-25"}

const testAPIKey = "test-key"

func routeFor(t *testing.T, seenKeys chan<- string) sourcestest.Route {
	dir := sourcestest.TestdataDir(t, Name)
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	return func(r *http.Request) (string, bool) {
		select {
		case seenKeys <- r.Header.Get(apiKeyHeader):
		default:
		}
		path := r.URL.Path
		switch {
		case path == "/v1/categories":
			name := "categories_" + r.URL.Query().Get("marketVariant") + ".json"
			if !exists(name) {
				name = "categories_empty.json"
			}
			return name, true
		case strings.HasPrefix(path, "/v1/markets/") && strings.HasSuffix(path, "/orderbook"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/markets/"), "/orderbook")
			name := "orderbook_" + id + ".json"
			return name, exists(name)
		}
		return "", false
	}
}

func newTestAdapter(t *testing.T, cfg sources.Config, seenKeys chan<- string) *Adapter {
	t.Helper()
	srv := sourcestest.ReplayServer(t, Name, routeFor(t, seenKeys))
	adapter, ok := New(sourcestest.Deps(srv, cfg)).(*Adapter)
	require.True(t, ok)
	return adapter
}

func TestAdapter_Discover(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Discover(context.Background(), []string{"LIGUE1", "EPL"})
	require.NoError(t, err)
	leagues := map[string]int{}
	for _, ev := range snap.Events {
		leagues[ev.LeagueHint]++
	}
	assert.Equal(t, map[string]int{"LIGUE1": 3, "EPL": 2}, leagues)
	sourcestest.AssertGolden(t, Name, "discover", snap)
}

func TestAdapter_DiscoverKickoffIsTwoHoursBeforeSettlement(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Discover(context.Background(), []string{"LIGUE1"})
	require.NoError(t, err)
	for _, ev := range snap.Events {
		if ev.SourceID == "fl1-psg-lyo-2026-10-25" {
			assert.Equal(t, time.Date(2026, 10, 25, 19, 45, 0, 0, time.UTC), ev.Kickoff)
			return
		}
	}
	t.Fatal("fl1-psg-lyo-2026-10-25 not discovered")
}

func TestAdapter_Price(t *testing.T) {
	t.Parallel()
	seenKeys := make(chan string, 1)
	snap, err := newTestAdapter(t, sources.Config{APIKey: testAPIKey}, seenKeys).Price(context.Background(), fixtureMatches)
	require.NoError(t, err)
	assert.Equal(t, testAPIKey, <-seenKeys)
	sourcestest.RequireMarkets(t, snap, canon.Code1X2, canon.Code1H1X2, canon.CodeCS, canon.CodeOU, canon.CodeBTTS, canon.CodeAH, canon.CodeCornOU)
	sourcestest.AssertGolden(t, Name, "price", snap)
}

func TestBoard_ApplyRecordedFrames(t *testing.T) {
	t.Parallel()
	b, err := newTestAdapter(t, sources.Config{}, nil).loadBoard(context.Background(), fixtureMatches[:1])
	require.NoError(t, err)
	var updates [][]canon.RawMarket
	var replies [][]byte
	var rejected int
	for _, frame := range readFrames(t) {
		changed, reply, err := b.apply(frame, sourcestest.FixedNow)
		if err != nil {
			rejected++
			continue
		}
		if reply != nil {
			replies = append(replies, reply)
		}
		if len(changed) > 0 {
			updates = append(updates, changed)
		}
	}
	assert.Equal(t, 1, rejected)
	require.Len(t, replies, 1)
	assert.JSONEq(t, `{"method":"heartbeat","data":1792909602907}`, string(replies[0]))
	require.Len(t, updates, 2)
	home := findSelection(t, updates[0], "epl-liv-bri-2026-10-25", canon.SideHome)
	assert.InDelta(t, 0.46, *home.Price.Ask, 1e-9)
	assert.InDelta(t, 0.45, *home.Price.Bid, 1e-9)
	assert.InDelta(t, 0.46*41152.918391304345, *home.Price.Liquidity, 1e-6)
	under := findSelection(t, updates[1], "3228730", canon.SideUnder)
	assert.InDelta(t, 0.39, *under.Price.Ask, 1e-9)
	assert.InDelta(t, 0.38, *under.Price.Bid, 1e-9)
	assert.InDelta(t, 0.39*1636.115, *under.Price.Liquidity, 1e-6)
}

func TestAdapter_StreamWithoutKeyAsksRuntimeToPoll(t *testing.T) {
	t.Parallel()
	err := newTestAdapter(t, sources.Config{}, nil).Stream(context.Background(), fixtureMatches, make(chan canon.Snapshot))
	assert.ErrorIs(t, err, ErrStreamNeedsAPIKey)
}

func TestAdapter_StreamSubscribesAndAnswersHeartbeat(t *testing.T) {
	t.Parallel()
	frames := readFrames(t)
	topics := make(chan string, 128)
	heartbeats := make(chan []byte, 1)
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(apiKeyHeader) != testAPIKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for _, frame := range frames {
			if conn.Write(r.Context(), websocket.MessageText, frame) != nil {
				return
			}
		}
		for {
			_, msg, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var req subscribeRequest
			if json.Unmarshal(msg, &req) != nil {
				continue
			}
			switch req.Method {
			case subscribeMethod:
				topics <- req.Params[0]
			case heartbeatMethod:
				heartbeats <- msg
			}
		}
	}))
	t.Cleanup(wsSrv.Close)
	a := newTestAdapter(t, sources.Config{APIKey: testAPIKey, Extra: map[string]string{"wsUrl": "ws" + strings.TrimPrefix(wsSrv.URL, "http")}}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := make(chan canon.Snapshot, 8)
	done := make(chan error, 1)
	go func() { done <- a.Stream(ctx, fixtureMatches[:1], out) }()
	assert.NotEmpty(t, (<-out).Markets)
	assert.NotEmpty(t, (<-out).Markets)
	assert.JSONEq(t, `{"method":"heartbeat","data":1792909602907}`, string(<-heartbeats))
	assert.True(t, strings.HasPrefix(<-topics, orderbookTopic))
	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
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
