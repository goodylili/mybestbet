package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

func TestTouchesMainMarket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		marketKey canon.MarketKey
		expected  bool
	}{
		{name: "football 1X2", marketKey: canon.MarketKey{EventID: "e1", Code: canon.Code1X2}, expected: true},
		{name: "moneyline", marketKey: canon.MarketKey{EventID: "e1", Code: canon.CodeMoneyline}, expected: true},
		{name: "moneyline of one map", marketKey: canon.MarketKey{EventID: "e1", Code: canon.CodeMoneyline, Period: canon.PeriodFirstMap}, expected: false},
		{name: "first half 1X2", marketKey: canon.MarketKey{EventID: "e1", Code: canon.Code1H1X2}, expected: false},
		{name: "team scoped moneyline", marketKey: canon.MarketKey{EventID: "e1", Code: canon.CodeMoneyline, TeamID: "t1"}, expected: false},
		{name: "over under", marketKey: canon.MarketKey{EventID: "e1", Code: canon.CodeOU, Line: canon.Line(2.5)}, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			payload, err := json.Marshal(canon.BoardUpdate{EventID: "e1", Cells: []canon.BoardCell{decimalCell(tt.marketKey.String(), "a", canon.SideHome, "", 2)}})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, touchesMainMarket(string(payload)))
		})
	}
}

func TestBoardChangesTakeKeepsLatestPerEvent(t *testing.T) {
	t.Parallel()
	changes := newBoardChanges()
	changes.markMoved("e1")
	changes.markMoved("e1")
	changes.merge(EventBest{EventID: "e2", Score: &canon.EventScore{}})
	changes.merge(EventBest{EventID: "e2", Best: &BestOutcomes{Market: canon.CodeMoneyline}})
	changes.merge(EventBest{EventID: "e2", Score: &canon.EventScore{Home: 1}})

	moved, published := changes.take()
	assert.Equal(t, []string{"e1"}, moved)
	require.Contains(t, published, "e2")
	assert.Equal(t, &canon.EventScore{Home: 1}, published["e2"].Score)
	require.NotNil(t, published["e2"].Best)
	assert.Equal(t, canon.CodeMoneyline, published["e2"].Best.Market)

	moved, published = changes.take()
	assert.Empty(t, moved)
	assert.Empty(t, published)
}

func TestBoardScopeIncludes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		scope    boardScope
		league   string
		expected bool
	}{
		{name: "everything", scope: boardScope{}, league: "EPL", expected: true},
		{name: "unknown event league", scope: boardScope{}, league: "", expected: false},
		{name: "matching league", scope: boardScope{league: "EPL"}, league: "EPL", expected: true},
		{name: "other league", scope: boardScope{league: "EPL"}, league: "NBA", expected: false},
		{name: "league of sport", scope: boardScope{leagues: []string{"NBA"}}, league: "NBA", expected: true},
		{name: "league outside sport", scope: boardScope{leagues: []string{"NBA"}}, league: "EPL", expected: false},
		{name: "sport without leagues", scope: boardScope{leagues: []string{}}, league: "EPL", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.scope.includes(tt.league))
		})
	}
}

func TestBoardStreamPushesScopedMainMarketBestAndScore(t *testing.T) {
	pool, rdb := integrationDeps(t)
	f := seedContractFixture(t, pool, rdb)
	s := newTestServer(t, pool, rdb, f)
	ctx, cancel := context.WithCancel(context.Background())
	hubDone := make(chan error, 1)
	go func() { hubDone <- s.RunStreamHub(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-hubDone)
	})
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/stream?scope=board&sport=basketball", nil)
	require.NoError(t, err)
	authorize(req, userToken(t, s, pool))
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	reader := bufio.NewReader(resp.Body)
	require.Eventually(t, func() bool {
		patterns, err := rdb.PubSubNumPat(ctx).Result()
		return err == nil && patterns > 0
	}, 2*time.Second, 10*time.Millisecond)

	publish := func(channel string, payload any) {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, rdb.Publish(ctx, channel, encoded).Err())
	}
	oneXTwo := oneXTwoKey(f.eventID)
	publish(redis.EventChannel(f.eventID), canon.BoardUpdate{EventID: f.eventID, Cells: []canon.BoardCell{decimalCell(oneXTwo, "bet9ja", canon.SideHome, "", 2.1)}})
	publish(redis.EventChannel(f.nbaEventID), canon.BoardUpdate{EventID: f.nbaEventID, Cells: []canon.BoardCell{decimalCell(f.nbaMoneyline, "betking", canon.SideHome, "", 1.75)}})
	publish(redis.ChannelBoard, map[string]any{"eventId": f.nbaEventID, "score": map[string]any{"home": 51, "away": 48, "period": "Q3"}})
	publish(redis.ChannelBoard, map[string]any{"eventId": f.eventID, "score": map[string]any{"home": 1, "away": 0}})

	var best *BestOutcomes
	var score *canon.EventScore
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (best == nil || score == nil) {
		update := readEventBest(t, reader)
		require.Equal(t, f.nbaEventID, update.EventID, "only events of the subscribed sport are pushed")
		if update.Best != nil {
			best = update.Best
		}
		if update.Score != nil {
			score = update.Score
		}
	}
	require.NotNil(t, best)
	assert.Equal(t, canon.CodeMoneyline, best.Market)
	require.NotNil(t, best.Home)
	assert.Equal(t, "betking", best.Home.Source)
	assert.Equal(t, &canon.EventScore{Home: 51, Away: 48, Period: "Q3"}, score)

	_ = resp.Body.Close()
	require.Eventually(t, func() bool { return len(s.hub.boardScopes()) == 0 }, 2*time.Second, 10*time.Millisecond, "disconnect should drop the board subscriber")
}

func readEventBest(t *testing.T, reader *bufio.Reader) EventBest {
	t.Helper()
	isBest := false
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		line = strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			isBest = name == "best"
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		require.True(t, isBest, "board stream frames are named best")
		var update EventBest
		require.NoError(t, json.Unmarshal([]byte(data), &update))
		return update
	}
}
