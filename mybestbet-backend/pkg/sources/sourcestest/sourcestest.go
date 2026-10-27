package sourcestest

import (
	"bytes"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

var update = flag.Bool("update", false, "rewrite golden snapshots")

var FixedNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func RepoRoot(t testing.TB) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func TestdataDir(t testing.TB, source string) string {
	return filepath.Join(RepoRoot(t), "testdata", source)
}

func ReadFixture(t testing.TB, source, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(TestdataDir(t, source), name))
	if err != nil {
		t.Fatalf("read fixture %s/%s: %v", source, name, err)
	}
	return body
}

type Route func(r *http.Request) (fixture string, ok bool)

func ReplayServer(t testing.TB, source string, route Route) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := route(r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(ReadFixture(t, source, name))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func Deps(srv *httptest.Server, cfg sources.Config) sources.Deps {
	cfg.BaseURL = srv.URL
	return sources.Deps{HTTP: srv.Client(), Config: cfg, Now: func() time.Time { return FixedNow }}
}

func AssertGolden(t testing.TB, source, name string, snap canon.Snapshot) {
	t.Helper()
	sortSnapshot(&snap)
	got, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	path := filepath.Join(TestdataDir(t, source), "golden", name+".json")
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update): %v", path, err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("snapshot differs from %s; run with -update and review the diff", path)
	}
}

func RequireMarkets(t testing.TB, snap canon.Snapshot, codes ...canon.MarketCode) {
	t.Helper()
	seen := map[canon.MarketCode]bool{}
	for _, m := range snap.Markets {
		seen[m.Code] = true
	}
	for _, c := range codes {
		if !seen[c] {
			t.Errorf("snapshot missing market %s", c)
		}
	}
}

type LinkCase struct {
	Name string
	Ref  sources.EventRef
	Want string
}

func EPLMatch(sourceEventID string) sources.EventRef {
	return sources.EventRef{
		SourceEventID: sourceEventID,
		Sport:         canon.SportFootball,
		League:        "EPL",
		Country:       "England",
		LeagueName:    "Premier League",
		Home:          "Chelsea",
		Away:          "Manchester United",
	}
}

func AssertEventURLs(t *testing.T, source string, cfg sources.Config, cases []LinkCase) {
	t.Helper()
	factory, ok := sources.Lookup(source)
	if !ok {
		t.Fatalf("source %s is not registered", source)
	}
	linker, ok := factory(sources.Deps{Config: cfg}).(sources.EventLinker)
	if !ok {
		t.Fatalf("source %s does not implement sources.EventLinker", source)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			if got := linker.EventURL(tc.Ref); got != tc.Want {
				t.Errorf("EventURL() = %q, want %q", got, tc.Want)
			}
		})
	}
}

func sortSnapshot(snap *canon.Snapshot) {
	sort.SliceStable(snap.Events, func(i, j int) bool { return snap.Events[i].SourceID < snap.Events[j].SourceID })
	sort.SliceStable(snap.Markets, func(i, j int) bool {
		a, b := snap.Markets[i], snap.Markets[j]
		if a.EventSourceID != b.EventSourceID {
			return a.EventSourceID < b.EventSourceID
		}
		return a.SourceMarketID < b.SourceMarketID
	})
}
