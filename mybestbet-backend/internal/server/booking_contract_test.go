package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/booking"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/fake"
)

func TestBookingEndpointsMatchOpenAPIContract(t *testing.T) {
	pool, rdb := integrationDeps(t)
	f := seedContractFixture(t, pool, rdb)
	s := newTestServer(t, pool, rdb, f)
	book := fake.Name
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"BK42"}`))
	}))
	t.Cleanup(upstream.Close)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO event_links (source, "sourceEventId", "eventId") VALUES ($1, 'fake-1', $2)
		ON CONFLICT (source, "sourceEventId") DO UPDATE SET "eventId" = EXCLUDED."eventId"`, book, f.eventID)
	require.NoError(t, err)
	marketKey := oneXTwoKey(f.eventID)
	cell, err := json.Marshal(decimalCell(marketKey, book, canon.SideHome, "", 2.4))
	require.NoError(t, err)
	field := redis.BoardField(book, string(canon.SideHome), "")
	require.NoError(t, rdb.HSet(ctx, redis.BoardKey(marketKey), field, cell).Err())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM event_links WHERE source = $1 AND "eventId" = $2`, book, f.eventID)
		_ = rdb.HDel(context.Background(), redis.BoardKey(marketKey), field).Err()
	})
	cfg := config.Config{
		Sources:  map[string]sources.Config{book: {Enabled: true, BaseURL: upstream.URL}, "kalshi": {Enabled: true}},
		Profiles: map[string]config.SourceProfile{book: {Type: "sportsbook"}, "kalshi": {Type: "prediction"}},
	}
	s.bookings, err = booking.New(booking.Deps{Config: cfg, Pool: pool, Redis: rdb, Log: slog.Default()})
	require.NoError(t, err)
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)
	token := userToken(t, s, pool)

	slip := func(mode, target string) string {
		encoded, _ := json.Marshal(booking.Request{Mode: booking.Mode(mode), Book: target, Stake: 10, Legs: []booking.Leg{
			{EventID: f.eventID, MarketKey: marketKey, Source: book, Side: "home"},
			{EventID: f.eventID, MarketKey: f.ouKey, Source: "kalshi", Side: "over"},
		}})
		return string(encoded)
	}
	post := func(name, path, body string, status int, check func(t *testing.T, body map[string]any)) contractCase {
		return contractCase{name: name, method: "post", template: path, path: path, body: body, status: status, check: check}
	}
	cases := []contractCase{
		post("book split", "/v1/bookings", slip("", ""), http.StatusOK, func(t *testing.T, body map[string]any) {
			assert.Equal(t, "split", body["mode"])
			result := asMap(asSlice(body["bookings"])[0])
			assert.Equal(t, book, result["book"])
			assert.Equal(t, "ok", result["status"])
			assert.Equal(t, "BK42", result["code"])
			assert.InDelta(t, 24, result["return"], 1e-9)
			manual := asMap(asSlice(body["manual"])[0])
			assert.Equal(t, "kalshi", manual["source"])
			assert.InDelta(t, 1, manual["index"], 0)
			comparison := asMap(body["comparison"])
			assert.Equal(t, book, asMap(asSlice(comparison["books"])[0])["book"])
		}),
		post("book again returns the cached code", "/v1/bookings", slip("", ""), http.StatusOK, func(t *testing.T, body map[string]any) {
			result := asMap(asSlice(body["bookings"])[0])
			assert.Equal(t, "BK42", result["code"])
			assert.Equal(t, true, result["cached"])
		}),
		post("book single", "/v1/bookings", slip("single", book), http.StatusOK, func(t *testing.T, body map[string]any) {
			assert.Equal(t, book, body["book"])
			assert.Len(t, body["bookings"], 1)
		}),
		post("book single without a book", "/v1/bookings", slip("single", ""), http.StatusBadRequest, nil),
		post("book without legs", "/v1/bookings", `{"legs":[]}`, http.StatusBadRequest, nil),
		post("book malformed body", "/v1/bookings", "{", http.StatusBadRequest, nil),
		{name: "book anonymous", method: "post", template: "/v1/bookings", path: "/v1/bookings", body: slip("", ""), anonymous: true, status: http.StatusUnauthorized},
		post("compare", "/v1/bookings/compare", slip("", ""), http.StatusOK, func(t *testing.T, body map[string]any) {
			assert.InDelta(t, 2, body["totalLegs"], 0)
			assert.InDelta(t, 4.6154, asMap(body["bestSplit"])["odds"], 1e-9)
			assert.Len(t, body["books"], 1)
		}),
		post("compare unknown source", "/v1/bookings/compare", strings.Replace(slip("", ""), "kalshi", "nowhere", 1), http.StatusBadRequest, nil),
	}
	runContractCases(t, httpServer.URL, loadSpec(t), token, cases)

	t.Run("cookie write from a foreign origin", func(t *testing.T) {
		for origin, status := range map[string]int{"https://evil.test": http.StatusForbidden, testWebOrigin: http.StatusOK} {
			req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/bookings/compare", strings.NewReader(slip("", "")))
			require.NoError(t, err)
			req.Header.Set("Origin", origin)
			req.AddCookie(&http.Cookie{Name: s.cookieName(accessCookieName), Value: token})
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, status, resp.StatusCode, origin)
		}
	})
}
