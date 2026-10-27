package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferredEncoding(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"":                       "",
		"gzip, deflate, br":      "br",
		"gzip":                   "gzip",
		"br;q=0, gzip":           "gzip",
		"identity":               "",
		"deflate, gzip;q=0.5, *": "gzip",
	}
	for accept, expected := range tests {
		assert.Equal(t, expected, preferredEncoding(accept), accept)
	}
}

func TestCompressResponses(t *testing.T) {
	t.Parallel()
	large := `{"items":"` + strings.Repeat("odds ", 1000) + `"}`
	handler := compressResponses(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := large
		if r.URL.Query().Has("small") {
			body = `{"ok":true}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	serve := func(path, accept string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept-Encoding", accept)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := serve("/", "gzip, br")
	assert.Equal(t, "br", rec.Header().Get("Content-Encoding"))
	decoded, err := io.ReadAll(brotli.NewReader(rec.Body))
	require.NoError(t, err)
	assert.Equal(t, large, string(decoded))

	rec = serve("/", "gzip")
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	decoded, err = io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, large, string(decoded))

	rec = serve("/?small", "br")
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.JSONEq(t, `{"ok":true}`, rec.Body.String())
	assert.Contains(t, rec.Header().Values("Vary"), "Accept-Encoding")

	rec = serve("/", "")
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, large, rec.Body.String())
}

func TestWriteCachedJSONRevalidates(t *testing.T) {
	t.Parallel()
	first := httptest.NewRecorder()
	writeCachedJSON(first, httptest.NewRequest(http.MethodGet, "/", nil), map[string]int{"n": 1}, 2*time.Second)
	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, "private, max-age=2, stale-while-revalidate=10", first.Header().Get("Cache-Control"))
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	again := httptest.NewRecorder()
	writeCachedJSON(again, req, map[string]int{"n": 1}, 2*time.Second)
	assert.Equal(t, http.StatusNotModified, again.Code)
	assert.Empty(t, again.Body.String())

	changed := httptest.NewRecorder()
	writeCachedJSON(changed, req, map[string]int{"n": 2}, 0)
	assert.Equal(t, http.StatusOK, changed.Code)
	assert.Equal(t, "private, no-cache", changed.Header().Get("Cache-Control"))
}

func TestSharedResponseCache(t *testing.T) {
	t.Parallel()
	var c responseCache
	calls := 0
	handler := c.shared(time.Minute, compressResponses(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Has("missing") {
			writeError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		writeCachedJSON(w, r, map[string]string{"items": strings.Repeat("odds ", 500)}, time.Second)
	})))
	serve := func(path string, headers ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	first := serve("/v1/events", "Accept-Encoding", "br")
	require.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, "br", first.Header().Get("Content-Encoding"))
	etag := first.Header().Get("ETag")
	require.NotEmpty(t, etag)
	second := serve("/v1/events", "Accept-Encoding", "br")
	assert.Equal(t, first.Body.Bytes(), second.Body.Bytes())
	assert.Equal(t, 1, calls, "the second request is served from the cache")

	plain := serve("/v1/events")
	assert.Empty(t, plain.Header().Get("Content-Encoding"))
	assert.Equal(t, 2, calls, "each encoding is cached on its own")

	revalidated := serve("/v1/events", "Accept-Encoding", "gzip", "If-None-Match", etag)
	assert.Equal(t, http.StatusNotModified, revalidated.Code)
	assert.Empty(t, revalidated.Body.Bytes())

	c.mu.Lock()
	for _, entry := range c.entries {
		entry.storedAt = entry.storedAt.Add(-2 * time.Minute)
	}
	c.mu.Unlock()
	stale := serve("/v1/events", "Accept-Encoding", "br")
	assert.Equal(t, first.Body.Bytes(), stale.Body.Bytes(), "a stale entry is served at once")
	assert.Eventually(t, func() bool {
		_, fresh := c.lookup("/v1/events?|br", time.Minute, time.Now())
		return fresh
	}, time.Second, 5*time.Millisecond, "and refreshed in the background")

	missing := serve("/v1/events?missing")
	assert.Equal(t, http.StatusNotFound, missing.Code)
	serve("/v1/events?missing")
	assert.Equal(t, 6, calls, "errors are never cached")
}

func TestResponseCacheBoundsBytes(t *testing.T) {
	t.Parallel()
	var c responseCache
	now := time.Now()
	half := &cachedResponse{status: http.StatusOK, body: make([]byte, maxCachedResponseBytes/2), storedAt: now}
	c.store("a", half, time.Second)
	c.store("a", half, time.Second)
	assert.Equal(t, maxCachedResponseBytes/2, c.bytes, "replacing an entry does not count it twice")
	c.store("b", half, time.Second)
	assert.Len(t, c.entries, 2)
	c.store("c", &cachedResponse{status: http.StatusOK, body: make([]byte, 1), storedAt: now}, time.Second)
	assert.Len(t, c.entries, 1, "going over the byte cap drops the cache")
	assert.Equal(t, 1, c.bytes)
	c.store("huge", &cachedResponse{status: http.StatusOK, body: make([]byte, maxCachedResponseBytes+1), storedAt: now}, time.Second)
	assert.NotContains(t, c.entries, "huge", "a body over the cap is never stored")
	assert.Equal(t, 1, c.bytes)
}
