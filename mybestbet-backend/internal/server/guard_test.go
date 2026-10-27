package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/auth"
)

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

func TestEveryNonPublicRouteRequiresAuth(t *testing.T) {
	t.Parallel()
	s := &Server{log: slog.Default()}
	handler := s.Handler()
	routes := s.routes()
	for pattern := range publicRoutes {
		assert.Contains(t, routes, pattern, "stale public route")
	}
	for pattern := range adminRoutes {
		assert.Contains(t, routes, pattern, "stale admin route")
	}
	for pattern := range routes {
		if publicRoutes[pattern] {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()
			method, path, _ := strings.Cut(pattern, " ")
			req := httptest.NewRequest(method, pathParam.ReplaceAllString(path, "x"), strings.NewReader("{}"))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Equal(t, "Bearer", rec.Header().Get("WWW-Authenticate"))
		})
	}
}

func TestOriginAllowed(t *testing.T) {
	t.Parallel()
	allowed := []string{"https://odds.test"}
	tests := []struct {
		name     string
		origin   string
		fetch    string
		expected bool
	}{
		{name: "allowed origin", origin: "https://odds.test", expected: true},
		{name: "foreign origin", origin: "https://evil.test", expected: false},
		{name: "scheme mismatch", origin: "http://odds.test", expected: false},
		{name: "null origin", origin: "null", expected: false},
		{name: "no origin but same-origin fetch", fetch: "same-origin", expected: true},
		{name: "no origin cross-site fetch", fetch: "cross-site", expected: false},
		{name: "no origin no fetch metadata", expected: false},
		{name: "foreign origin wins over fetch metadata", origin: "https://evil.test", fetch: "same-origin", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.fetch != "" {
				req.Header.Set("Sec-Fetch-Site", tt.fetch)
			}
			assert.Equal(t, tt.expected, originAllowed(req, allowed))
		})
	}
}

func TestCheckOriginScope(t *testing.T) {
	t.Parallel()
	s := &Server{log: slog.Default()}
	next := s.checkOrigin(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	tests := []struct {
		name     string
		method   string
		path     string
		cookie   bool
		expected int
	}{
		{name: "safe method passes", method: http.MethodGet, path: "/v1/me", cookie: true, expected: http.StatusNoContent},
		{name: "auth post without origin is rejected", method: http.MethodPost, path: "/v1/auth/login", expected: http.StatusForbidden},
		{name: "cookie write without origin is rejected", method: http.MethodPut, path: "/v1/me/settings", cookie: true, expected: http.StatusForbidden},
		{name: "bearer-only write passes", method: http.MethodPost, path: "/v1/admin/review", expected: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.cookie {
				req.AddCookie(&http.Cookie{Name: hostCookiePrefix + accessCookieName, Value: "token"})
			}
			rec := httptest.NewRecorder()
			next(rec, req)
			assert.Equal(t, tt.expected, rec.Code)
		})
	}
}

func TestClientIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		forwarded string
		trust     bool
		expected  string
	}{
		{name: "remote address when not trusted", forwarded: "1.2.3.4", expected: "192.0.2.1"},
		{name: "hop appended by the trusted proxy wins over client supplied hops", forwarded: "1.2.3.4, 10.0.0.1", trust: true, expected: "10.0.0.1"},
		{name: "spoofed leading hop is ignored", forwarded: "203.0.113.9,198.51.100.7", trust: true, expected: "198.51.100.7"},
		{name: "garbage proxy hop falls back to remote", forwarded: "1.2.3.4, junk", trust: true, expected: "192.0.2.1"},
		{name: "ipv6 hop", forwarded: " 2001:db8::1 ", trust: true, expected: "2001:db8::1"},
		{name: "garbage falls back to remote", forwarded: "not-an-ip", trust: true, expected: "192.0.2.1"},
		{name: "missing header", trust: true, expected: "192.0.2.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			assert.Equal(t, tt.expected, clientIP(req, tt.trust))
		})
	}
}

func TestSessionCookiesAreHardened(t *testing.T) {
	t.Parallel()
	s := &Server{log: slog.Default()}
	rec := httptest.NewRecorder()
	s.clearSessionCookies(rec)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 2)
	for _, cookie := range cookies {
		assert.True(t, strings.HasPrefix(cookie.Name, hostCookiePrefix), cookie.Name)
		assert.True(t, cookie.HttpOnly)
		assert.True(t, cookie.Secure)
		assert.Equal(t, "/", cookie.Path)
		assert.Empty(t, cookie.Domain)
	}
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
	assert.Equal(t, http.SameSiteStrictMode, cookies[1].SameSite)
}

func TestBusyMapsToServiceUnavailable(t *testing.T) {
	t.Parallel()
	s := &Server{log: slog.Default()}
	rec := httptest.NewRecorder()
	s.writeFailure(rec, httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil), fmt.Errorf("verify password: %w", auth.ErrBusy))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	assert.JSONEq(t, `{"error":{"code":"busy","message":"server busy, try again shortly"}}`, rec.Body.String())
}
