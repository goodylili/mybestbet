package server

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/auth"
	"mybestbet/internal/logos"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

func TestPendingUpdateKeepsLatestCellPerSelection(t *testing.T) {
	t.Parallel()
	key := oneXTwoKey("e1")
	pending := newPendingUpdate("e1")
	pending.merge(canon.BoardUpdate{Cells: []canon.BoardCell{decimalCell(key, "a", canon.SideHome, "", 2.0), decimalCell(key, "a", canon.SideAway, "", 3.0)}})
	pending.merge(canon.BoardUpdate{Cells: []canon.BoardCell{decimalCell(key, "a", canon.SideHome, "", 2.2)}, Signals: []canon.Signal{{ID: "s1", Margin: 0.01}}})
	pending.merge(canon.BoardUpdate{Signals: []canon.Signal{{ID: "s1", Margin: 0.02}}})
	require.False(t, pending.isEmpty())

	update := pending.drain(time.Unix(0, 0))
	require.Len(t, update.Cells, 2)
	assert.Equal(t, "e1", update.EventID)
	for _, cell := range update.Cells {
		if cell.Side == canon.SideHome {
			assert.InDelta(t, 2.2, cell.Price.Raw, 1e-9)
		}
	}
	require.Len(t, update.Signals, 1)
	assert.InDelta(t, 0.02, update.Signals[0].Margin, 1e-9)
	assert.True(t, pending.isEmpty())
}

func TestWantsUpdate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		subscribed string
		eventID    string
		isSignal   bool
		expected   bool
	}{
		{name: "board for subscribed event", subscribed: "e1", eventID: "e1", expected: true},
		{name: "board for other event", subscribed: "e1", eventID: "e2", expected: false},
		{name: "board without event subscription", subscribed: "", eventID: "e1", expected: false},
		{name: "signal for subscribed event", subscribed: "e1", eventID: "e1", isSignal: true, expected: true},
		{name: "signal for other event", subscribed: "e1", eventID: "e2", isSignal: true, expected: false},
		{name: "signal on global stream", subscribed: "", eventID: "e2", isSignal: true, expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, wantsUpdate(tt.subscribed, canon.BoardUpdate{EventID: tt.eventID}, tt.isSignal))
		})
	}
}

func TestSubscriberOverflowMarksLagging(t *testing.T) {
	t.Parallel()
	sub := &subscriber{}
	sub.open(1)
	sub.offer(canon.BoardUpdate{})
	sub.offer(canon.BoardUpdate{})
	sub.offer(canon.BoardUpdate{})
	select {
	case <-sub.lagging:
	default:
		t.Fatal("subscriber should be marked lagging after overflow")
	}
}

func seedStreamEvent(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	_, err := pool.Exec(ctx, `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	var homeID, awayID, eventID string
	insert := `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`
	require.NoError(t, pool.QueryRow(ctx, insert, "Stream Home "+suffix).Scan(&homeID))
	require.NoError(t, pool.QueryRow(ctx, insert, "Stream Away "+suffix).Scan(&awayID))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3) RETURNING id::text`,
		homeID, awayID, time.Now().Add(time.Hour)).Scan(&eventID))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM events WHERE id = $1`, eventID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM teams WHERE id = ANY($1::uuid[])`, []string{homeID, awayID})
	})
	return eventID
}

func TestStreamCoalescesBoardAndSignalUpdates(t *testing.T) {
	pool, rdb := integrationDeps(t)
	s := &Server{pool: pool, rdb: rdb, log: slog.Default(), hub: newStreamHub(rdb, slog.Default()), logos: logos.NewStore(pool)}
	ctx, cancel := context.WithCancel(context.Background())
	hubDone := make(chan error, 1)
	go func() { hubDone <- s.hub.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-hubDone)
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/stream", s.stream)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	eventID := seedStreamEvent(t, pool)
	resp, err := http.Get(httpServer.URL + "/v1/stream?event=" + eventID)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	reader := bufio.NewReader(resp.Body)
	name, data := readFrame(t, reader)
	require.Equal(t, "snapshot", name)
	var snapshot EventDetail
	require.NoError(t, json.Unmarshal([]byte(data), &snapshot))
	assert.Equal(t, eventID, snapshot.Event.ID)

	key := oneXTwoKey(eventID)
	publish := func(channel string, payload any) {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, rdb.Publish(ctx, channel, encoded).Err())
	}
	publish(redis.EventChannel(eventID), canon.BoardUpdate{EventID: eventID, Cells: []canon.BoardCell{decimalCell(key, "betano", canon.SideHome, "", 2.0)}})
	publish(redis.EventChannel(eventID), canon.BoardUpdate{EventID: eventID, Cells: []canon.BoardCell{decimalCell(key, "betano", canon.SideHome, "", 2.3)}})
	publish(redis.ChannelSignals, canon.Signal{ID: "sig-" + eventID, EventID: eventID, Type: canon.SignalArb, Status: canon.SignalOpen})
	publish(redis.ChannelSignals, canon.Signal{ID: "other", EventID: uuid.NewString(), Type: canon.SignalArb, Status: canon.SignalOpen})

	var received canon.BoardUpdate
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (len(received.Cells) == 0 || len(received.Signals) == 0) {
		update := readUpdate(t, reader)
		received.Cells = append(received.Cells, update.Cells...)
		received.Signals = append(received.Signals, update.Signals...)
	}
	require.Len(t, received.Cells, 1)
	assert.InDelta(t, 2.3, received.Cells[0].Price.Raw, 1e-9)
	require.Len(t, received.Signals, 1)
	assert.Equal(t, "sig-"+eventID, received.Signals[0].ID)
}

func readFrame(t *testing.T, reader *bufio.Reader) (name, data string) {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		line = strings.TrimSpace(line)
		if event, ok := strings.CutPrefix(line, "event: "); ok {
			name = event
		}
		if payload, ok := strings.CutPrefix(line, "data: "); ok {
			return name, payload
		}
	}
}

func readUpdate(t *testing.T, reader *bufio.Reader) canon.BoardUpdate {
	t.Helper()
	name, data := readFrame(t, reader)
	require.Equal(t, "update", name)
	var update canon.BoardUpdate
	require.NoError(t, json.Unmarshal([]byte(data), &update))
	return update
}

func TestStreamEndsWhenAccessTokenExpires(t *testing.T) {
	pool, rdb := integrationDeps(t)
	s := &Server{pool: pool, rdb: rdb, log: slog.Default(), hub: newStreamHub(rdb, slog.Default()), logos: logos.NewStore(pool)}
	ctx, cancel := context.WithCancel(context.Background())
	hubDone := make(chan error, 1)
	go func() { hubDone <- s.hub.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-hubDone)
	})
	expiresAt := time.Now().Add(2 * time.Second)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.stream(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{UserID: uuid.NewString(), SessionID: uuid.NewString(), ExpiresAt: expiresAt})))
	}))
	t.Cleanup(httpServer.Close)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(httpServer.URL + "/v1/stream?event=" + seedStreamEvent(t, pool))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err, "the server closes the stream cleanly")
	assert.WithinDuration(t, expiresAt, time.Now(), time.Second)
}

func TestStreamEndsWhenSessionIsRevoked(t *testing.T) {
	pool, rdb := integrationDeps(t)
	cfg := config.Config{Env: "test", Auth: testAuthConfig()}
	svc := newTestAuth(t, cfg, pool, rdb, newResendMock(t).URL, auth.GoogleEndpoints{})
	s := &Server{pool: pool, rdb: rdb, log: slog.Default(), hub: newStreamHub(rdb, slog.Default()), logos: logos.NewStore(pool), auth: svc, heartbeat: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	hubDone := make(chan error, 1)
	go func() { hubDone <- s.hub.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-hubDone)
	})
	email := testEmail()
	createUser(t, pool, email)
	session, err := svc.Login(ctx, email, testPassword, auth.Client{IP: testIP()})
	require.NoError(t, err)
	httpServer := httptest.NewServer(s.guard("GET /v1/stream", s.stream))
	t.Cleanup(httpServer.Close)

	req, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/stream?event="+seedStreamEvent(t, pool), nil)
	require.NoError(t, err)
	authorize(req, session.AccessToken)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, line, "retry:")

	revokedAt := time.Now()
	require.NoError(t, svc.Logout(ctx, session.RefreshToken, "", false))
	_, err = io.ReadAll(reader)
	require.NoError(t, err, "the server closes the stream")
	assert.WithinDuration(t, revokedAt, time.Now(), time.Second)
}

func TestStreamQuotaCapsConcurrentStreamsPerUser(t *testing.T) {
	t.Parallel()
	var quota streamQuota
	user, other := uuid.NewString(), uuid.NewString()
	releases := make([]func(), 0, maxStreamsPerUser)
	for range maxStreamsPerUser {
		release, ok := quota.acquire(user)
		require.True(t, ok)
		releases = append(releases, release)
	}
	_, ok := quota.acquire(user)
	assert.False(t, ok, "one stream past the cap is refused")
	_, ok = quota.acquire(other)
	assert.True(t, ok, "other users keep their own budget")
	releases[0]()
	releases[0]()
	_, ok = quota.acquire(user)
	assert.True(t, ok, "a closed stream frees exactly one slot")
	_, ok = quota.acquire(user)
	assert.False(t, ok, "a double release does not free a second slot")
}

func TestStreamRejectsUserOverQuota(t *testing.T) {
	t.Parallel()
	s := &Server{log: slog.Default()}
	principal := auth.Principal{UserID: uuid.NewString(), SessionID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Minute)}
	for range maxStreamsPerUser {
		_, ok := s.streams.acquire(principal.UserID)
		require.True(t, ok)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/stream", nil)
	rec := httptest.NewRecorder()
	s.stream(rec, req.WithContext(auth.WithPrincipal(req.Context(), principal)))
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}
