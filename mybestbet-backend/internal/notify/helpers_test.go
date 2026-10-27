package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
)

const testToken = "123:secret-token"

func testRedis(t *testing.T) *goredis.Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	rdb, err := redis.Open(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := postgres.Open(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

type sentMessage struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type fakeTelegram struct {
	mu      sync.Mutex
	sent    []sentMessage
	updates []telegramUpdate
	paths   []string
	fail    bool
}

func (f *fakeTelegram) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.paths = append(f.paths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		if f.fail {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests"}`))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var msg sentMessage
			_ = json.Unmarshal(body, &msg)
			f.sent = append(f.sent, msg)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			encoded, _ := json.Marshal(telegramResponse[[]telegramUpdate]{OK: true, Result: f.updates})
			f.updates = nil
			_, _ = w.Write(encoded)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeTelegram) messages() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMessage(nil), f.sent...)
}
