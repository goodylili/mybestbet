package server

import (
	"strings"
	"sync"
	"time"

	"mybestbet/pkg/canon"
)

const (
	boardCacheTTL        = 15 * time.Second
	boardResyncAfter     = time.Minute
	boardIdleAfter       = 10 * time.Minute
	maxBoardCacheEntries = 50_000
)

type boardCacheEntry struct {
	cells    []canon.BoardCell
	loadedAt time.Time
	usedAt   time.Time
}

type eventMarketsEntry struct {
	keys     map[string]struct{}
	loadedAt time.Time
	usedAt   time.Time
}

type boardCache struct {
	mu           sync.Mutex
	isLive       bool
	entries      map[string]*boardCacheEntry
	byEvent      map[string]map[string]struct{}
	generations  map[string]uint64
	dirty        map[string]struct{}
	markets      map[string]*eventMarketsEntry
	dirtyMarkets map[string]struct{}
}

func newBoardCache() *boardCache {
	return &boardCache{
		entries:      map[string]*boardCacheEntry{},
		byEvent:      map[string]map[string]struct{}{},
		generations:  map[string]uint64{},
		dirty:        map[string]struct{}{},
		markets:      map[string]*eventMarketsEntry{},
		dirtyMarkets: map[string]struct{}{},
	}
}

type boardCacheMiss struct {
	marketKey  string
	generation uint64
}

func (c *boardCache) usable(loadedAt, now time.Time) bool {
	return c.isLive || now.Sub(loadedAt) < boardCacheTTL
}

func (c *boardCache) lookup(marketKeys []string, now time.Time) (map[string][]canon.BoardCell, []boardCacheMiss) {
	c.mu.Lock()
	defer c.mu.Unlock()
	hits := make(map[string][]canon.BoardCell, len(marketKeys))
	var misses []boardCacheMiss
	for _, key := range marketKeys {
		if entry, ok := c.entries[key]; ok && c.usable(entry.loadedAt, now) {
			entry.usedAt = now
			hits[key] = entry.cells
			continue
		}
		misses = append(misses, boardCacheMiss{marketKey: key, generation: c.generations[eventOfMarketKey(key)]})
	}
	return hits, misses
}

func (c *boardCache) store(miss boardCacheMiss, cells []canon.BoardCell, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eventID := eventOfMarketKey(miss.marketKey)
	if len(c.entries) >= maxBoardCacheEntries {
		c.evictIdle(now.Add(boardIdleAfter - boardCacheTTL))
	}
	if len(c.entries) >= maxBoardCacheEntries {
		c.reset()
	}
	entry := c.entries[miss.marketKey]
	if entry == nil {
		entry = &boardCacheEntry{usedAt: now}
		c.entries[miss.marketKey] = entry
	}
	entry.cells, entry.loadedAt = cells, now
	keys := c.byEvent[eventID]
	if keys == nil {
		keys = map[string]struct{}{}
		c.byEvent[eventID] = keys
	}
	keys[miss.marketKey] = struct{}{}
	if c.generations[eventID] != miss.generation {
		c.dirty[miss.marketKey] = struct{}{}
	}
}

func (c *boardCache) eventMarkets(eventID string, now time.Time) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.markets[eventID]
	if !ok || !c.usable(entry.loadedAt, now) {
		return nil, false
	}
	entry.usedAt = now
	keys := make([]string, 0, len(entry.keys))
	for key := range entry.keys {
		keys = append(keys, key)
	}
	return keys, true
}

func (c *boardCache) storeEventMarkets(eventID string, keys []string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	entry := c.markets[eventID]
	if entry == nil {
		entry = &eventMarketsEntry{usedAt: now}
		c.markets[eventID] = entry
	}
	entry.keys, entry.loadedAt = set, now
}

func (c *boardCache) invalidate(eventID string, marketKeys []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generations[eventID]++
	markets := c.markets[eventID]
	for _, key := range marketKeys {
		if _, ok := c.entries[key]; ok {
			c.dirty[key] = struct{}{}
		}
		if markets != nil {
			if _, known := markets.keys[key]; !known {
				c.dirtyMarkets[eventID] = struct{}{}
			}
		}
	}
}

func (c *boardCache) takeDirty(now time.Time) ([]boardCacheMiss, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isLive {
		return nil, nil
	}
	c.evictIdle(now)
	for key, entry := range c.entries {
		if now.Sub(entry.loadedAt) >= boardResyncAfter {
			c.dirty[key] = struct{}{}
		}
	}
	for eventID, entry := range c.markets {
		if now.Sub(entry.loadedAt) >= boardResyncAfter {
			c.dirtyMarkets[eventID] = struct{}{}
		}
	}
	misses := make([]boardCacheMiss, 0, len(c.dirty))
	for key := range c.dirty {
		misses = append(misses, boardCacheMiss{marketKey: key, generation: c.generations[eventOfMarketKey(key)]})
	}
	events := make([]string, 0, len(c.dirtyMarkets))
	for eventID := range c.dirtyMarkets {
		events = append(events, eventID)
	}
	clear(c.dirty)
	clear(c.dirtyMarkets)
	return misses, events
}

func (c *boardCache) setLive(isLive bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isLive = isLive
	c.reset()
}

func (c *boardCache) evictIdle(now time.Time) {
	for key, entry := range c.entries {
		if now.Sub(entry.usedAt) >= boardIdleAfter {
			delete(c.entries, key)
			delete(c.byEvent[eventOfMarketKey(key)], key)
			delete(c.dirty, key)
		}
	}
	for eventID, entry := range c.markets {
		if now.Sub(entry.usedAt) >= boardIdleAfter {
			delete(c.markets, eventID)
			delete(c.dirtyMarkets, eventID)
		}
	}
}

func (c *boardCache) reset() {
	clear(c.entries)
	clear(c.byEvent)
	clear(c.dirty)
	clear(c.markets)
	clear(c.dirtyMarkets)
}

func eventOfMarketKey(marketKey string) string {
	eventID, _, _ := strings.Cut(marketKey, "|")
	return eventID
}
