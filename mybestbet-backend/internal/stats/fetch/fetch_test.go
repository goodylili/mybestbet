package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"mybestbet/pkg/sources"
)

type stubBrowser struct {
	got    sources.BrowserRequest
	result sources.BrowserResult
}

func (b *stubBrowser) Fetch(_ context.Context, req sources.BrowserRequest) (sources.BrowserResult, error) {
	b.got = req
	return b.result, nil
}

func TestBodyFallsBackToBrowserWhenForbidden(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	t.Cleanup(srv.Close)
	browser := &stubBrowser{result: sources.BrowserResult{Captured: []sources.Captured{
		{URL: srv.URL + "/api/v1/x", Status: http.StatusOK, Body: []byte(`{"ok":true}`)},
	}}}
	f := Fetcher{Deps: sources.Deps{HTTP: srv.Client(), Browser: browser}, Profile: "sofascore"}

	var out struct{ OK bool }
	if err := f.JSON(t.Context(), srv.URL+"/api/v1/x", &out); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !out.OK || browser.got.Profile != "sofascore" || browser.got.CapturePath[0] != "/api/v1/x" {
		t.Fatalf("unexpected browser request %+v / out %+v", browser.got, out)
	}
}

func TestBodyReturnsForbiddenWithoutBrowser(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	t.Cleanup(srv.Close)
	f := Fetcher{Deps: sources.Deps{HTTP: srv.Client()}}
	if _, err := f.Body(t.Context(), srv.URL); err == nil {
		t.Fatal("expected forbidden error")
	}
}

func TestBodyFallsBackToBrowserWhenConnectionRefused(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	refusedURL := srv.URL + "/api/v1/y"
	srv.Close()
	browser := &stubBrowser{result: sources.BrowserResult{Captured: []sources.Captured{
		{URL: refusedURL, Status: http.StatusOK, Body: []byte(`{"ok":true}`)},
	}}}
	f := Fetcher{Deps: sources.Deps{HTTP: http.DefaultClient, Browser: browser}, Profile: "sofascore"}

	body, err := f.Body(t.Context(), refusedURL)
	if err != nil || string(body) != `{"ok":true}` {
		t.Fatalf("Body = %q, %v", body, err)
	}
}

func TestBodyDoesNotUseBrowserForNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	browser := &stubBrowser{}
	f := Fetcher{Deps: sources.Deps{HTTP: srv.Client(), Browser: browser}}
	if _, err := f.Body(t.Context(), srv.URL); err == nil {
		t.Fatal("expected not found error")
	}
	if browser.got.URL != "" {
		t.Fatalf("browser used for a 404: %+v", browser.got)
	}
}
