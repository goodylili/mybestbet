package browserd

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/sources"
)

type stubBrowser struct {
	result sources.BrowserResult
	err    error
}

func (s stubBrowser) Fetch(context.Context, sources.BrowserRequest) (sources.BrowserResult, error) {
	return s.result, s.err
}

func TestFetchHandler(t *testing.T) {
	t.Parallel()
	policy := RequestPolicy{AllowedHostSuffixes: []string{"bet365.com"}, MaxWait: 10 * time.Second}
	okResult := sources.BrowserResult{Captured: []sources.Captured{{URL: "https://www.bet365.com/SportsBook.API/web", Status: 200, Body: []byte("F|EV;FI=1|")}}}
	tests := []struct {
		name       string
		body       string
		browser    stubBrowser
		wantStatus int
	}{
		{name: "returns captured payloads", body: `{"url":"https://www.bet365.com/#/AS/B1/","capturePath":["SportsBook.API"]}`, browser: stubBrowser{result: okResult}, wantStatus: http.StatusOK},
		{name: "malformed json", body: `{"url":`, wantStatus: http.StatusBadRequest},
		{name: "host not allowed", body: `{"url":"https://example.com/"}`, wantStatus: http.StatusForbidden},
		{name: "browser timeout", body: `{"url":"https://www.bet365.com/"}`, browser: stubBrowser{err: context.DeadlineExceeded}, wantStatus: http.StatusGatewayTimeout},
		{name: "pool closed", body: `{"url":"https://www.bet365.com/"}`, browser: stubBrowser{err: ErrPoolClosed}, wantStatus: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewFetchHandler(tt.browser, policy, slog.New(slog.NewTextHandler(io.Discard, nil)))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/fetch", strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var got sources.BrowserResult
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(got.Captured) != 1 || string(got.Captured[0].Body) != "F|EV;FI=1|" {
				t.Fatalf("captured = %+v, want the stub payload", got.Captured)
			}
		})
	}
}
