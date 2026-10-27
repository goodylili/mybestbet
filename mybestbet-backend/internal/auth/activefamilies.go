package auth

import (
	"sync"
	"time"
)

const (
	localActiveTTL       = 5 * time.Second
	maxLocalActiveFamily = 100_000
)

type activeFamilies struct {
	mu    sync.Mutex
	until map[string]time.Time
}

func (a *activeFamilies) has(familyID string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return now.Before(a.until[familyID])
}

func (a *activeFamilies) remember(familyID string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.until == nil {
		a.until = map[string]time.Time{}
	}
	if len(a.until) >= maxLocalActiveFamily {
		for id, until := range a.until {
			if !now.Before(until) {
				delete(a.until, id)
			}
		}
	}
	if len(a.until) >= maxLocalActiveFamily {
		clear(a.until)
	}
	a.until[familyID] = now.Add(localActiveTTL)
}

func (a *activeFamilies) forget(familyIDs ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range familyIDs {
		delete(a.until, id)
	}
}
