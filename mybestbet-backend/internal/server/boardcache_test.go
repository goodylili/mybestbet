package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestBoardCache(t *testing.T) {
	t.Parallel()
	now := time.Now()
	key := "ev-1|1X2|||"
	cells := []canon.BoardCell{{MarketKey: key, Source: "bet9ja", Side: canon.SideHome}}

	t.Run("cells expire after the ttl while the feed is down", func(t *testing.T) {
		t.Parallel()
		c := newBoardCache()
		_, misses := c.lookup([]string{key}, now)
		c.store(misses[0], cells, now)
		hits, misses := c.lookup([]string{key}, now)
		assert.Equal(t, map[string][]canon.BoardCell{key: cells}, hits)
		assert.Empty(t, misses)
		hits, _ = c.lookup([]string{key}, now.Add(boardCacheTTL))
		assert.Empty(t, hits)
	})

	t.Run("a live feed keeps cells and marks changed markets for refresh", func(t *testing.T) {
		t.Parallel()
		c := newBoardCache()
		c.setLive(true)
		other := "ev-1|OU|2.5||"
		_, misses := c.lookup([]string{key, other}, now)
		c.store(misses[0], cells, now)
		c.store(misses[1], nil, now)
		c.invalidate("ev-1", []string{key})
		hits, misses := c.lookup([]string{key, other}, now.Add(boardCacheTTL))
		assert.Len(t, hits, 2)
		assert.Empty(t, misses)
		dirty, events := c.takeDirty(now)
		assert.Equal(t, []string{key}, marketKeysOf(dirty))
		assert.Empty(t, events)
		dirty, _ = c.takeDirty(now)
		assert.Empty(t, dirty)
	})

	t.Run("a read that raced a change is refreshed again", func(t *testing.T) {
		t.Parallel()
		c := newBoardCache()
		c.setLive(true)
		_, inFlight := c.lookup([]string{key}, now)
		c.invalidate("ev-1", []string{key})
		c.store(inFlight[0], cells, now)
		dirty, _ := c.takeDirty(now)
		assert.Equal(t, []string{key}, marketKeysOf(dirty))
	})

	t.Run("an unseen market of a cached event reloads its market list", func(t *testing.T) {
		t.Parallel()
		c := newBoardCache()
		c.setLive(true)
		c.storeEventMarkets("ev-1", []string{key}, now)
		c.invalidate("ev-1", []string{key})
		_, events := c.takeDirty(now)
		assert.Empty(t, events)
		c.invalidate("ev-1", []string{"ev-1|OU|2.5||"})
		_, events = c.takeDirty(now)
		assert.Equal(t, []string{"ev-1"}, events)
		keys, ok := c.eventMarkets("ev-1", now)
		assert.True(t, ok)
		assert.Equal(t, []string{key}, keys)
	})

	t.Run("idle entries are evicted", func(t *testing.T) {
		t.Parallel()
		c := newBoardCache()
		c.setLive(true)
		_, misses := c.lookup([]string{key}, now)
		c.store(misses[0], cells, now)
		c.takeDirty(now.Add(boardIdleAfter))
		_, misses = c.lookup([]string{key}, now.Add(boardIdleAfter))
		assert.Len(t, misses, 1)
	})
}

func marketKeysOf(misses []boardCacheMiss) []string {
	keys := make([]string, len(misses))
	for i, miss := range misses {
		keys[i] = miss.marketKey
	}
	return keys
}

func TestPricedSinceKickoff(t *testing.T) {
	t.Parallel()
	kickoff := time.Now().Add(-time.Hour)
	before := canon.BoardCell{Source: "bet9ja", Price: canon.Price{ObservedAt: kickoff.Add(-time.Minute)}}
	after := canon.BoardCell{Source: "polymarket", Price: canon.Price{ObservedAt: kickoff.Add(time.Minute)}}
	assert.Equal(t, []canon.BoardCell{after}, pricedSinceKickoff([]canon.BoardCell{before, after}, kickoff, time.Now()))
	assert.Len(t, pricedSinceKickoff([]canon.BoardCell{before, after}, kickoff, kickoff.Add(-time.Second)), 2)
}
