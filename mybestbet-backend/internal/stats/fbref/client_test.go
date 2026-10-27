package fbref

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	srv := sourcestest.ReplayServer(t, SourceName, func(r *http.Request) (string, bool) {
		page, ok := strings.CutPrefix(r.URL.Path, "/en/comps/9/")
		if !ok || !strings.HasSuffix(page, "/Premier-League-Stats") {
			return "", false
		}
		return "comps_9_" + strings.TrimSuffix(page, "/Premier-League-Stats") + ".html", true
	})
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Config.BaseURL = srv.URL + "/en"
	return New(deps)
}

func TestLeagueSeasonMergesPages(t *testing.T) {
	t.Parallel()
	seasons, err := newReplayClient(t).LeagueSeason(t.Context(), "EPL")
	if err != nil {
		t.Fatalf("LeagueSeason: %v", err)
	}
	if len(seasons) != 3 {
		t.Fatalf("got %d players, want 3", len(seasons))
	}
	solanke := seasons[2]
	expected := PlayerSeason{
		FBrefID: "e77dc3b2", Name: "Dominic Solanke", Team: "Tottenham", Season: "2026-2027",
		Matches: 7, Starts: 4, Minutes: 353, Goals: 1, Assists: 1, Shots: 12, ShotsOnTarget: 3,
		Fouls: 8, Tackles: 1, Yellow: 1,
	}
	if solanke != expected {
		t.Fatalf("solanke = %+v\nwant %+v", solanke, expected)
	}
	if got := solanke.Per90(solanke.Shots); math.Abs(got-3.06) > 0.01 {
		t.Errorf("shots per 90 = %v, want 3.06", got)
	}
	if seasons[0].Name != "Antonín Kinský" || seasons[0].Per90(0) != 0 {
		t.Errorf("kinsky = %+v", seasons[0])
	}
}

func TestLeagueSeasonUnknownLeague(t *testing.T) {
	t.Parallel()
	if _, err := newReplayClient(t).LeagueSeason(t.Context(), "UCL"); !errors.Is(err, ErrUnknownLeague) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseTableReadsUncommentedTable(t *testing.T) {
	t.Parallel()
	page := `<html><body><table id="stats_misc"><caption>x <span>2026 MLS</span></caption><tbody>
<tr><th data-stat="ranker">1</th><td data-stat="player"><a href="/en/players/abc123/X">X</a></td><td data-stat="fouls">1,204</td></tr>
<tr><td data-stat="player">No link</td></tr></tbody></table></body></html>`
	rows, season, err := parseTable([]byte(page), "stats_misc")
	if err != nil {
		t.Fatalf("parseTable: %v", err)
	}
	if season != "2026" || len(rows) != 1 || rows[0].fbrefID != "abc123" || atoi(rows[0].cells["fouls"]) != 1204 {
		t.Fatalf("rows = %+v season = %q", rows, season)
	}
	if _, _, err := parseTable([]byte(page), "stats_standard"); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("missing table err = %v", err)
	}
}
