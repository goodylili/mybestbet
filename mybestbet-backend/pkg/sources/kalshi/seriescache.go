package kalshi

import (
	"sync"
	"time"
)

const seriesCacheTTL = 5 * time.Second

type cachedSeries struct {
	listedAt time.Time
	events   map[string]event
}

type seriesCache struct {
	mu      sync.Mutex
	entries map[string]cachedSeries
}

func (c *seriesCache) get(series string, now time.Time) (map[string]event, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[series]
	if !ok || now.Sub(entry.listedAt) >= seriesCacheTTL {
		return nil, false
	}
	return entry.events, true
}

func (c *seriesCache) put(series string, events map[string]event, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]cachedSeries{}
	}
	for key, entry := range c.entries {
		if now.Sub(entry.listedAt) >= seriesCacheTTL {
			delete(c.entries, key)
		}
	}
	c.entries[series] = cachedSeries{listedAt: now, events: events}
}
