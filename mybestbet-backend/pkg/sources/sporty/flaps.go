package sporty

import (
	"strings"
	"sync"
	"time"

	"mybestbet/pkg/canon"
)

const (
	flapHold       = 15 * time.Minute
	flapTrailTTL   = 2 * time.Hour
	flapPruneEvery = 10 * time.Minute
)

type quoteTrail struct {
	last, previous float64
	flappedAt      time.Time
	seenAt         time.Time
}

type flapGuard struct {
	mu       sync.Mutex
	trails   map[string]*quoteTrail
	prunedAt time.Time
}

func newFlapGuard() *flapGuard { return &flapGuard{trails: map[string]*quoteTrail{}} }

func (g *flapGuard) suspendFlapping(markets []canon.RawMarket, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(now)
	for i := range markets {
		m := &markets[i]
		if m.Player == nil {
			continue
		}
		for j := range m.Selections {
			sel := &m.Selections[j]
			key := strings.Join([]string{m.EventSourceID, m.SourceMarketID, string(sel.Side), sel.Label}, "|")
			if g.observe(key, sel.Price.Raw, now) {
				sel.Price.Suspended = true
			}
		}
	}
}

func (g *flapGuard) observe(key string, raw float64, now time.Time) bool {
	trail, ok := g.trails[key]
	if !ok {
		g.trails[key] = &quoteTrail{last: raw, seenAt: now}
		return false
	}
	trail.seenAt = now
	if raw != trail.last {
		if raw == trail.previous {
			trail.flappedAt = now
		}
		trail.previous, trail.last = trail.last, raw
	}
	return !trail.flappedAt.IsZero() && now.Sub(trail.flappedAt) < flapHold
}

func (g *flapGuard) prune(now time.Time) {
	if now.Sub(g.prunedAt) < flapPruneEvery {
		return
	}
	g.prunedAt = now
	for key, trail := range g.trails {
		if now.Sub(trail.seenAt) > flapTrailTTL {
			delete(g.trails, key)
		}
	}
}
