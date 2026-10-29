package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"mybestbet/internal/auth"
)

const (
	accessCookieName  = "mbb_at"
	refreshCookieName = "mbb_rt"
	signupCookieName  = "mbb_signup"
	oauthCookieName   = "mbb_oauth"
	hostCookiePrefix  = "__Host-"
	maxUserAgentLen   = 256
	authPathPrefix    = "/v1/auth/"
)

var publicRoutes = map[string]bool{
	"GET /v1/meta":                  true,
	"POST /v1/auth/signup":          true,
	"POST /v1/auth/verify":          true,
	"POST /v1/auth/otp/resend":      true,
	"POST /v1/auth/login":           true,
	"POST /v1/auth/password/forgot": true,
	"POST /v1/auth/password/reset":  true,
	"POST /v1/auth/refresh":         true,
	"POST /v1/auth/logout":          true,
	"GET /v1/auth/google/start":     true,
	"GET /v1/auth/google/callback":  true,
}

var guestRoutes = map[string]bool{
	"GET /v1/sports":                true,
	"GET /v1/search":                true,
	"GET /v1/leagues":               true,
	"GET /v1/events":                true,
	"GET /v1/events/calendar":       true,
	"GET /v1/events/{id}":           true,
	"GET /v1/events/{id}/props":     true,
	"GET /v1/markets/{key}/history": true,
	"GET /v1/signals":               true,
	"GET /v1/baysed":                true,
	"GET /v1/players/{id}/stats":    true,
	"GET /v1/teams/{id}":            true,
	"GET /v1/sources":               true,
	"GET /v1/logos/{kind}/{id}":     true,
	"GET /v1/stream":                true,
}

var adminRoutes = map[string]bool{
	"GET /v1/admin/review":  true,
	"POST /v1/admin/review": true,
}

func (s *Server) guard(pattern string, handler http.HandlerFunc) http.Handler {
	switch {
	case publicRoutes[pattern]:
	case guestRoutes[pattern]:
		handler = s.optionalUser(handler)
	case adminRoutes[pattern]:
		handler = s.requireAdminToken(handler)
	default:
		handler = s.requireUser(handler)
	}
	return s.checkOrigin(handler)
}

func (s *Server) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			writeUnauthorized(w)
			return
		}
		principal, err := s.auth.Authenticate(r.Context(), s.accessToken(r))
		if errors.Is(err, auth.ErrUnauthorized) {
			writeUnauthorized(w)
			return
		}
		if err != nil {
			s.writeFailure(w, r, fmt.Errorf("authenticate: %w", err))
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	}
}

func (s *Server) optionalUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.accessToken(r) == "" && r.Header.Get("Authorization") == "" {
			next(w, r)
			return
		}
		s.requireUser(next)(w, r)
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
}

func (s *Server) checkOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) || (!strings.HasPrefix(r.URL.Path, authPathPrefix) && !s.hasAuthCookie(r)) || originAllowed(r, s.allowedOrigins()) {
			next(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "forbidden", "cross-site request rejected")
	}
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func originAllowed(r *http.Request, allowed []string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") == "same-origin"
	}
	return slices.Contains(allowed, origin)
}

func (s *Server) allowedOrigins() []string {
	if s.auth == nil {
		return nil
	}
	return s.auth.Config().AllowedOrigins
}

func (s *Server) secureCookies() bool { return s.auth == nil || s.auth.Config().SecureCookies }

func (s *Server) cookieName(base string) string {
	if s.secureCookies() {
		return hostCookiePrefix + base
	}
	return base
}

func (s *Server) cookieValue(r *http.Request, base string) string {
	cookie, err := r.Cookie(s.cookieName(base))
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Server) hasAuthCookie(r *http.Request) bool {
	return s.cookieValue(r, accessCookieName) != "" || s.cookieValue(r, refreshCookieName) != ""
}

func (s *Server) accessToken(r *http.Request) string {
	if token := bearerToken(r); token != "" {
		return token
	}
	return s.cookieValue(r, accessCookieName)
}

func (s *Server) client(r *http.Request) auth.Client {
	userAgent := r.UserAgent()
	if len(userAgent) > maxUserAgentLen {
		userAgent = userAgent[:maxUserAgentLen]
	}
	return auth.Client{IP: clientIP(r, s.auth != nil && s.auth.Config().TrustProxy), UserAgent: userAgent}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		hops := r.Header.Values("X-Forwarded-For")
		var last string
		if len(hops) > 0 {
			chain := hops[len(hops)-1]
			last = chain[strings.LastIndex(chain, ",")+1:]
		}
		if addr, err := netip.ParseAddr(strings.TrimSpace(last)); err == nil {
			return addr.Unmap().String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
