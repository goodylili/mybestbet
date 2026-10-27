package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var requestLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "api_request_seconds",
	Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
}, []string{"route", "status"})

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) instrument(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		elapsed := time.Since(started)
		requestLatency.WithLabelValues(route, strconv.Itoa(recorder.status)).Observe(elapsed.Seconds())
		s.log.DebugContext(r.Context(), "request", "route", route, "status", recorder.status, "duration_ms", elapsed.Milliseconds())
	})
}

func (s *Server) requireAdminToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAdminToken(s.cfg.AdminToken, bearerToken(r)) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid bearer token")
			return
		}
		next(w, r)
	}
}

func bearerToken(r *http.Request) string {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func isAdminToken(expected, presented string) bool {
	if expected == "" || presented == "" {
		return false
	}
	expectedDigest := sha256.Sum256([]byte(expected))
	presentedDigest := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(expectedDigest[:], presentedDigest[:]) == 1
}
