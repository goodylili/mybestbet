package browser_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/browser"
)

func TestClient_Fetch(t *testing.T) {
	t.Parallel()
	want := sources.BrowserResult{
		Captured: []sources.Captured{{URL: "https://www.bet365.com/matchbettingcontentapi/coupon", Status: 200, Body: []byte("F|EV;FI=1|")}},
		Cookies:  map[string]string{"pstk": "abc"},
		Headers:  map[string]string{"User-Agent": "Chrome"},
	}
	var gotReq sources.BrowserRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fetch" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(want)
	}))
	t.Cleanup(srv.Close)

	req := sources.BrowserRequest{Profile: "bet365", URL: "https://www.bet365.com/#/AS/B1/", CapturePath: []string{"contentapi"}, CaptureWS: true, WaitMillis: 2500}
	got, err := browser.New(srv.URL+"/", srv.Client()).Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
	if gotReq.URL != req.URL || gotReq.Profile != req.Profile || !gotReq.CaptureWS || gotReq.WaitMillis != req.WaitMillis {
		t.Fatalf("server saw %+v, want %+v", gotReq, req)
	}
	if string(got.Captured[0].Body) != "F|EV;FI=1|" || got.Cookies["pstk"] != "abc" || got.Headers["User-Agent"] != "Chrome" {
		t.Fatalf("Fetch() = %+v, want %+v", got, want)
	}
}

func TestClient_FetchMapsStatusErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{name: "host not allowed", status: http.StatusForbidden, wantErr: sources.ErrForbidden},
		{name: "browser timeout", status: http.StatusGatewayTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.status) }))
			t.Cleanup(srv.Close)
			_, err := browser.New(srv.URL, srv.Client()).Fetch(context.Background(), sources.BrowserRequest{URL: "https://www.bet365.com/"})
			var statusErr *sources.StatusError
			if !errors.As(err, &statusErr) || statusErr.Status != tt.status {
				t.Fatalf("Fetch() error = %v, want status %d", err, tt.status)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Fetch() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_FetchFailsFastWhenBrowserdIsDown(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, err := browser.New(url, http.DefaultClient).Fetch(context.Background(), sources.BrowserRequest{}); err == nil {
		t.Fatal("Fetch() against a closed server returned nil error")
	}
}
