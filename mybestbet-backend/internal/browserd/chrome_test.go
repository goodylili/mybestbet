package browserd

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"mybestbet/pkg/sources"
)

const chromeTestPage = `<!doctype html><html><body><script>
fetch('/api/odds').then(r => r.text()).then(t => { document.body.innerText = t; });
</script></body></html>`

func TestPool_FetchCapturesXHRWithRealChrome(t *testing.T) {
	if os.Getenv("BROWSERD_CHROME_TEST") == "" {
		t.Skip("set BROWSERD_CHROME_TEST=1 to run against a real Chrome")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/odds", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "warm", Path: "/"})
		_, _ = io.WriteString(w, "F|EV;FI=42;NA=Arsenal v Chelsea|")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, chromeTestPage) })
	site := httptest.NewServer(mux)
	t.Cleanup(site.Close)

	pool := NewPool(Options{ProfileRoot: t.TempDir(), Size: 1, ChromeBin: os.Getenv("BROWSERD_CHROME_BIN")}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	req := sources.BrowserRequest{Profile: "test", URL: site.URL + "/", CapturePath: []string{"/api/odds"}, WaitMillis: 1500}
	for attempt := range 2 {
		result, err := pool.Fetch(ctx, req)
		if err != nil {
			t.Fatalf("attempt %d: Fetch() error: %v", attempt, err)
		}
		if len(result.Captured) == 0 || !strings.Contains(string(result.Captured[0].Body), "FI=42") {
			t.Fatalf("attempt %d: captured = %+v, want the odds payload", attempt, result.Captured)
		}
		if result.Cookies["session"] != "warm" {
			t.Fatalf("attempt %d: cookies = %v, want session=warm", attempt, result.Cookies)
		}
	}
}
