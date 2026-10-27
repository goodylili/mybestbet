package server

import (
	"errors"
	"hash/fnv"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/internal/auth"
)

const (
	staleWhileRevalidateFactor = 5
	sportsMaxAge               = 30 * time.Second
	calendarMaxAge             = 30 * time.Second
	metaMaxAge                 = time.Minute
	eventsMaxAge               = 2 * time.Second
	eventMaxAge                = time.Second
	signalsMaxAge              = 2 * time.Second
	sourcesMaxAge              = 3 * time.Second
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Error apiError `json:"error"`
}

type rateLimitedBody struct {
	Error    apiError `json:"error"`
	ResendIn int      `json:"resendIn"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		slog.Error("encode response", "err", err)
		http.Error(w, `{"error":{"code":"internal","message":"internal error"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encoded)
}

func writeCachedJSON(w http.ResponseWriter, r *http.Request, body any, maxAge time.Duration) {
	encoded, err := json.Marshal(body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: apiError{Code: "internal", Message: "internal error"}})
		return
	}
	digest := fnv.New64a()
	_, _ = digest.Write(encoded)
	etag := `W/"` + strconv.FormatUint(digest.Sum64(), 36) + `"`
	header := w.Header()
	header.Set("ETag", etag)
	header.Set("Cache-Control", cacheControl(maxAge))
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func cacheControl(maxAge time.Duration) string {
	if maxAge <= 0 {
		return "private, no-cache"
	}
	seconds := int(maxAge.Seconds())
	return "private, max-age=" + strconv.Itoa(seconds) + ", stale-while-revalidate=" + strconv.Itoa(staleWhileRevalidateFactor*seconds)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}})
}

func (s *Server) writeFailure(w http.ResponseWriter, r *http.Request, err error) {
	var invalid invalidInputError
	var authInput *auth.InputError
	var limited *auth.RateLimitError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, "bad_request", invalid.Error())
	case errors.As(err, &authInput):
		writeError(w, http.StatusBadRequest, "bad_request", authInput.Error())
	case errors.As(err, &limited):
		retry := max(seconds(limited.RetryAfter), 1)
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeJSON(w, http.StatusTooManyRequests, rateLimitedBody{Error: apiError{Code: "rate_limited", Message: "too many requests, try again later"}, ResendIn: retry})
	case errors.Is(err, auth.ErrInvalidCode):
		writeError(w, http.StatusBadRequest, "invalid_code", "the code is invalid")
	case errors.Is(err, auth.ErrBusy):
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "busy", "server busy, try again shortly")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, auth.ErrUnauthorized):
		writeUnauthorized(w)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "already decided differently")
	default:
		s.log.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}
