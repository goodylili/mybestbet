package server

import (
	"context"
	"sync"
	"time"

	json "github.com/goccy/go-json"
)

const (
	settingsCacheTTL        = 30 * time.Second
	maxSettingsCacheEntries = 10_000
)

type cachedSettings struct {
	settings json.RawMessage
	loadedAt time.Time
}

type settingsCache struct {
	mu      sync.Mutex
	entries map[string]cachedSettings
}

func (c *settingsCache) get(userID string, now time.Time) (json.RawMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[userID]
	if !ok || now.Sub(entry.loadedAt) >= settingsCacheTTL {
		return nil, false
	}
	return entry.settings, true
}

func (c *settingsCache) put(userID string, settings json.RawMessage, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil || len(c.entries) >= maxSettingsCacheEntries {
		c.entries = map[string]cachedSettings{}
	}
	c.entries[userID] = cachedSettings{settings: settings, loadedAt: now}
}

func (s *Server) userSettings(ctx context.Context, userID string) (json.RawMessage, error) {
	if settings, ok := s.settings.get(userID, time.Now()); ok {
		return settings, nil
	}
	settings, err := s.auth.Settings(ctx, userID)
	if err == nil {
		s.settings.put(userID, settings, time.Now())
	}
	return settings, err
}
