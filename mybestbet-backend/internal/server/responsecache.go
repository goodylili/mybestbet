package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	maxCachedResponses     = 5000
	maxCachedResponseBytes = 64 << 20
	staleResponseFactor    = 10
)

var cachedHeaders = []string{"Content-Type", "Content-Encoding", "Content-Length", "ETag", "Cache-Control", "Vary"}

type cachedResponse struct {
	status   int
	header   http.Header
	body     []byte
	storedAt time.Time
}

type responseCache struct {
	mu      sync.Mutex
	entries map[string]*cachedResponse
	bytes   int
	loads   singleflight.Group
}

func (c *responseCache) lookup(key string, ttl time.Duration, now time.Time) (entry *cachedResponse, fresh bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || now.Sub(entry.storedAt) >= staleResponseFactor*ttl {
		return nil, false
	}
	return entry, now.Sub(entry.storedAt) < ttl
}

func (c *responseCache) store(key string, entry *cachedResponse, ttl time.Duration) {
	size := len(entry.body)
	if size > maxCachedResponseBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]*cachedResponse{}
	}
	c.remove(key)
	if c.full(size) {
		for k, e := range c.entries {
			if entry.storedAt.Sub(e.storedAt) >= staleResponseFactor*ttl {
				c.remove(k)
			}
		}
	}
	if c.full(size) {
		clear(c.entries)
		c.bytes = 0
	}
	c.entries[key] = entry
	c.bytes += size
}

func (c *responseCache) full(incoming int) bool {
	return len(c.entries) >= maxCachedResponses || c.bytes+incoming > maxCachedResponseBytes
}

func (c *responseCache) remove(key string) {
	if entry, ok := c.entries[key]; ok {
		c.bytes -= len(entry.body)
		delete(c.entries, key)
	}
}

func (c *responseCache) shared(ttl time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path + "?" + r.URL.RawQuery + "|" + preferredEncoding(r.Header.Get(acceptEncodingKey))
		load := func() (any, error) { return c.load(key, ttl, r, next), nil }
		entry, fresh := c.lookup(key, ttl, time.Now())
		switch {
		case entry == nil:
			loaded, _, _ := c.loads.Do(key, load)
			entry = loaded.(*cachedResponse)
		case !fresh:
			c.loads.DoChan(key, load)
		}
		header := w.Header()
		for name, values := range entry.header {
			header[name] = values
		}
		if etag := entry.header.Get("ETag"); entry.status == http.StatusOK && etag != "" && etagMatches(r.Header.Get("If-None-Match"), etag) {
			header.Del("Content-Length")
			header.Del("Content-Encoding")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(entry.status)
		_, _ = w.Write(entry.body)
	})
}

func (c *responseCache) load(key string, ttl time.Duration, r *http.Request, next http.Handler) *cachedResponse {
	inner := r.Clone(context.WithoutCancel(r.Context()))
	inner.Header.Del("If-None-Match")
	recorder := &bufferedResponse{ResponseWriter: headerOnly{header: http.Header{}}, status: http.StatusOK}
	next.ServeHTTP(recorder, inner)
	entry := &cachedResponse{status: recorder.status, header: http.Header{}, body: recorder.body.Bytes(), storedAt: time.Now()}
	for _, name := range cachedHeaders {
		if values := recorder.Header().Values(name); len(values) > 0 {
			entry.header[http.CanonicalHeaderKey(name)] = values
		}
	}
	if recorder.status == http.StatusOK {
		c.store(key, entry, ttl)
	}
	return entry
}

type headerOnly struct{ header http.Header }

func (h headerOnly) Header() http.Header       { return h.header }
func (headerOnly) Write(p []byte) (int, error) { return len(p), nil }
func (headerOnly) WriteHeader(int)             {}
