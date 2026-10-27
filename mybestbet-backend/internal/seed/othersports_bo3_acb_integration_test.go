package seed

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources/sourcestest"
)

var acbRecordedWeeks = map[string]bool{"2990": true, "2991": true, "2992": true}

func acbSeedRoute(r *http.Request) (string, bool) {
	query := r.URL.Query()
	switch r.URL.Path {
	case "/es/liga":
		return "liga.html", true
	case "/_next/static/chunks/0cz1d0mv5g_q7.js":
		return "chunk_other.js", true
	case "/_next/static/chunks/2z438bai2hw49.js":
		return "chunk_key.js", true
	case "/api/matchdata/Menu/competition-data":
		return "competition_data.json", true
	case "/api/matchdata/Menu/matchlist":
		if acbRecordedWeeks[query.Get("weekId")] {
			return "matchlist_week_" + query.Get("weekId") + ".json", true
		}
		return "matchlist_empty.json", true
	case "/api/matchdata/Result/boxscores":
		if query.Get("matchId") == "105410" {
			return "boxscores_105410.json", true
		}
		return "empty_body.json", true
	}
	return "", false
}

func bo3SeedRoute(r *http.Request) (string, bool) {
	switch path := r.URL.Path; {
	case path == "/matches":
		return "cs2_matches.json", r.URL.Query().Get("filter[matches.discipline_id][eq]") == "1"
	case strings.HasPrefix(path, "/teams/"):
		return "team_" + strings.TrimPrefix(path, "/teams/") + ".json", true
	}
	return "", false
}

func TestACBSeedsSeasonFixturesAndRostersFromBoxscores(t *testing.T) {
	pool := openTestPool(t)
	var since time.Time
	if err := pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since); err != nil {
		t.Fatalf("db now: %v", err)
	}
	removeOtherSportRowsCreatedSince(t, pool, since)
	store := NewStore(pool)
	seedTwice(t, store, newReplaySeeder(t, store, sourcestest.FixedNow),
		League{Code: "ACB", Name: "Liga ACB", Country: "Spain", Sport: canon.SportBasketball})

	assertCount(t, pool, `SELECT count(*) FROM event_sources s JOIN events e ON e.id = s."eventId" WHERE s.source = 'acb' AND e."leagueCode" = 'ACB'`, 28)
	assertCount(t, pool, `SELECT count(*) FROM event_sources s JOIN events e ON e.id = s."eventId" WHERE s.source = 'acb' AND s."sourceId" = '105410' AND e.status = 'finished'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM team_sources WHERE source = 'acb'`, 18)
	assertCount(t, pool, `SELECT count(*) FROM team_aliases a JOIN team_sources s ON s."teamId" = a."teamId" WHERE s.source = 'acb' AND s."sourceId" = '8' AND a."sourceName" IN ('Asisa Joventut', 'JOV')`, 2)
	assertCount(t, pool, `SELECT count(*) FROM player_aliases a JOIN players p ON p.id = a."playerId" WHERE a.source = 'acb'
		AND p."teamId" IN (SELECT "teamId" FROM team_sources WHERE source = 'acb' AND "sourceId" IN ('8', '2'))`, 24)
	assertCount(t, pool, `SELECT count(*) FROM logos l JOIN team_sources s ON l.kind = 'team' AND l."entityId" = s."teamId"::text
		WHERE s.source = 'acb' AND l."sourceUrl" LIKE 'https://static.acb.com/%'`, 18)
}

func TestBo3SeedsCounterStrikeSeriesAndUpcomingRosters(t *testing.T) {
	pool := openTestPool(t)
	var since time.Time
	if err := pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since); err != nil {
		t.Fatalf("db now: %v", err)
	}
	removeOtherSportRowsCreatedSince(t, pool, since)
	store := NewStore(pool)
	seedTwice(t, store, newReplaySeeder(t, store, sourcestest.FixedNow),
		League{Code: "CS2", Name: "CS2", Country: "International", Sport: canon.SportEsports})

	assertCount(t, pool, `SELECT count(*) FROM event_sources WHERE source = 'bo3' AND "sourceId" LIKE 'CS2:%' AND "createdAt" >= $1`, 6, since)
	assertCount(t, pool, `SELECT count(*) FROM event_sources s JOIN events e ON e.id = s."eventId"
		WHERE s.source = 'bo3' AND s."sourceId" = 'CS2:big-vs-the-mongolz-22-10-2026' AND e.status = 'finished'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM team_sources WHERE source = 'bo3' AND "createdAt" >= $1`, 9, since)
	assertCount(t, pool, `SELECT count(*) FROM player_aliases WHERE source = 'bo3' AND "createdAt" >= $1`, 33, since)
	assertCount(t, pool, `SELECT count(*) FROM players p JOIN player_aliases a ON a."playerId" = p.id AND a.source = 'bo3' AND a."sourceId" = '25135' WHERE p.name = 'JDC'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM logos l JOIN team_sources s ON l.kind = 'team' AND l."entityId" = s."teamId"::text
		WHERE s.source = 'bo3' AND s."createdAt" >= $1 AND l."sourceUrl" LIKE 'https://files.bo3.gg/uploads/team/%'`, 9, since)
}
