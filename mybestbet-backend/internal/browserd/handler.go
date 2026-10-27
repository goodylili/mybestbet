package browserd

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/sources"
)

const maxRequestBodyBytes = 64 << 10

type FetchHandler struct {
	browser sources.Browser
	policy  RequestPolicy
	log     *slog.Logger
}

func NewFetchHandler(browser sources.Browser, policy RequestPolicy, log *slog.Logger) *FetchHandler {
	return &FetchHandler{browser: browser, policy: policy, log: log}
}

func (h *FetchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req sources.BrowserRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)).Decode(&req); err != nil {
		http.Error(w, "malformed fetch request", http.StatusBadRequest)
		return
	}
	req, err := h.policy.Normalize(req)
	if err != nil {
		http.Error(w, err.Error(), statusForError(err))
		return
	}
	result, err := h.browser.Fetch(r.Context(), req)
	if err != nil {
		h.log.Warn("browser fetch failed", "profile", req.Profile, "err", err)
		http.Error(w, "browser fetch failed", statusForError(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		h.log.Warn("write fetch response", "profile", req.Profile, "err", err)
	}
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, ErrHostNotAllowed):
		return http.StatusForbidden
	case errors.Is(err, ErrPoolClosed):
		return http.StatusServiceUnavailable
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}
