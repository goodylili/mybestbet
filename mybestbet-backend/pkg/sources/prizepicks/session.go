package prizepicks

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"mybestbet/pkg/sources"
)

const (
	browserProfile    = "prizepicks"
	defaultSessionURL = "https://app.prizepicks.com/"
	defaultSessionTTL = 20 * time.Minute
	sessionWarmWait   = 5 * time.Second
	sessionWarmBudget = 60 * time.Second
)

var sessionCapturePaths = []string{"/projections", "/leagues"}

type session struct {
	cookieHeader string
	headers      map[string]string
	expiresAt    time.Time
}

func (s session) apply(req *http.Request) {
	for name, value := range s.headers {
		req.Header.Set(name, value)
	}
	if s.cookieHeader != "" {
		req.Header.Set("Cookie", s.cookieHeader)
	}
}

type sessionCache struct {
	browser    sources.Browser
	sessionURL string
	ttl        time.Duration
	now        func() time.Time

	mu      sync.Mutex
	current session
}

func (c *sessionCache) get(ctx context.Context) (session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.browser == nil || c.now().Before(c.current.expiresAt) {
		return c.current, nil
	}
	warmed, err := c.warm(ctx)
	if err != nil {
		return session{}, err
	}
	c.current = warmed
	return warmed, nil
}

func (c *sessionCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = session{}
}

func (c *sessionCache) warm(ctx context.Context) (session, error) {
	ctx, cancel := context.WithTimeout(ctx, sessionWarmBudget)
	defer cancel()
	result, err := c.browser.Fetch(ctx, sources.BrowserRequest{
		Profile:     browserProfile,
		URL:         c.sessionURL,
		CapturePath: sessionCapturePaths,
		WaitMillis:  int(sessionWarmWait.Milliseconds()),
	})
	if err != nil {
		return session{}, fmt.Errorf("warm prizepicks session: %w", err)
	}
	return session{
		cookieHeader: cookieHeader(result.Cookies),
		headers:      result.Headers,
		expiresAt:    c.now().Add(c.ttl),
	}, nil
}

func cookieHeader(cookies map[string]string) string {
	pairs := make([]string, 0, len(cookies))
	for name, value := range cookies {
		pairs = append(pairs, name+"="+value)
	}
	slices.Sort(pairs)
	return strings.Join(pairs, "; ")
}
