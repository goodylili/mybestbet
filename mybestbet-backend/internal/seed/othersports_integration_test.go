package seed

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var otherSportSources = []string{espn.SourceName, euroleague.SourceName, esports.LiquipediaSource, esports.OpenDotaSource, flashscore.SourceName, bo3.SourceName, acb.SourceName}

func espnRoute(r *http.Request) (string, bool) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	date := r.URL.Query().Get("dates")
	switch path {
	case "basketball/nba/scoreboard":
		if date == "20261024" {
			return "nba_scoreboard_20261024.json", true
		}
		return "scoreboard_empty.json", true
	case "tennis/atp/scoreboard":
		if date == "20260325" {
			return "atp_scoreboard_20260325.json", true
		}
		return "scoreboard_empty.json", true
	case "tennis/wta/scoreboard":
		return "scoreboard_empty.json", true
	case "basketball/nba/teams":
		return "nba_teams.json", true
	case "basketball/nba/teams/30/roster":
		return "nba_roster_30.json", true
	}
	if strings.HasPrefix(path, "basketball/nba/teams/") {
		return "scoreboard_empty.json", true
	}
	return "", false
}

func esportsRoute(r *http.Request) (string, bool) {
	switch path := r.URL.Path; {
	case path == "/dota2/api.php":
		return "liquipedia_dota2_matches.json", true
	case path == "/counterstrike/api.php":
		return "liquipedia_counterstrike_matches.json", true
	case path == "/teams":
		return "opendota_teams.json", true
	case path == "/teams/10149530/players":
		return "opendota_team_10149530_players.json", true
	case strings.HasPrefix(path, "/teams/"):
		return "opendota_team_empty_players.json", true
	}
	return "", false
}

func euroLeagueRoute(r *http.Request) (string, bool) {
	switch r.URL.Path {
	case "/v2/competitions/E/seasons/E2026/games":
		return "e_games.json", true
	case "/v2/competitions/E/seasons/E2026/people":
		return "e_people.json", true
	}
	return "", false
}

func newReplaySeeder(t *testing.T, store *Store, now time.Time) LeagueSeeder {
	t.Helper()
	servers := map[string]sources.Deps{
		espn.SourceName:          sourcestest.Deps(sourcestest.ReplayServer(t, espn.SourceName, espnRoute), sources.Config{}),
		euroleague.SourceName:    sourcestest.Deps(sourcestest.ReplayServer(t, euroleague.SourceName, euroLeagueRoute), sources.Config{}),
		esports.LiquipediaSource: sourcestest.Deps(sourcestest.ReplayServer(t, "esports", esportsRoute), sources.Config{}),
	}
	servers[esports.OpenDotaSource] = servers[esports.LiquipediaSource]
	servers[flashscore.SourceName] = sourcestest.Deps(sourcestest.ReplayServer(t, flashscore.SourceName, flashscoreSeedRoute), sources.Config{})
	servers[bo3.SourceName] = sourcestest.Deps(sourcestest.ReplayServer(t, bo3.SourceName, bo3SeedRoute), sources.Config{})
	acbServer := sourcestest.ReplayServer(t, acb.SourceName, acbSeedRoute)
	servers[acb.SourceName] = sourcestest.Deps(acbServer, sources.Config{Extra: map[string]string{"keyPageUrl": acbServer.URL + "/es/liga"}})
	seeder, err := NewOtherSportsSeeder(SeederDeps{
		Store: store, Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return now },
		SourceDeps: func(source string) (sources.Deps, error) { return servers[source], nil },
	})
	if err != nil {
		t.Fatalf("NewOtherSportsSeeder: %v", err)
	}
	return seeder
}

func removeOtherSportRowsCreatedSince(t *testing.T, pool *pgxpool.Pool, since time.Time) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		statements := []string{
			`DELETE FROM events WHERE "createdAt" >= $2 AND id IN (SELECT "eventId" FROM event_sources WHERE source = ANY($1))`,
			`DELETE FROM player_aliases WHERE "createdAt" >= $2 AND source = ANY($1)`,
			`DELETE FROM players p WHERE p."createdAt" >= $2 AND NOT EXISTS (SELECT 1 FROM player_aliases a WHERE a."playerId" = p.id)
				AND p."teamId" IN (SELECT "teamId" FROM team_sources WHERE source = ANY($1))`,
			`DELETE FROM team_aliases WHERE "createdAt" >= $2 AND source = ANY($1)`,
			`DELETE FROM logos WHERE kind = 'team' AND "createdAt" >= $2 AND "entityId" IN (SELECT "teamId"::text FROM team_sources WHERE source = ANY($1))`,
			`DELETE FROM teams WHERE "createdAt" >= $2 AND id IN (SELECT "teamId" FROM team_sources WHERE source = ANY($1))`,
		}
		for _, stmt := range statements {
			if _, err := pool.Exec(ctx, stmt, otherSportSources, since); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
}

func seedTwice(t *testing.T, store *Store, seeder LeagueSeeder, leagues ...League) {
	t.Helper()
	for run := range 2 {
		for _, league := range leagues {
			if err := store.UpsertLeague(t.Context(), league); err != nil {
				t.Fatalf("upsert league %s: %v", league.Code, err)
			}
			if err := seeder.SeedLeague(t.Context(), league); err != nil {
				t.Fatalf("seed %s run %d: %v", league.Code, run, err)
			}
		}
	}
}

func TestOtherSportsSeedIsIdempotent(t *testing.T) {
	pool := openTestPool(t)
	var since time.Time
	if err := pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since); err != nil {
		t.Fatalf("db now: %v", err)
	}
	removeOtherSportRowsCreatedSince(t, pool, since)
	store := NewStore(pool)
	seedTwice(t, store, newReplaySeeder(t, store, sourcestest.FixedNow),
		League{Code: "NBA", Name: "NBA", Country: "USA", Sport: canon.SportBasketball},
		League{Code: "EUROLEAGUE", Name: "EuroLeague", Country: "Europe", Sport: canon.SportBasketball},
		League{Code: "DOTA2", Name: "Dota 2", Country: "International", Sport: canon.SportEsports},
		League{Code: "ACB", Name: "Liga ACB", Country: "Spain", Sport: canon.SportBasketball},
	)

	assertCount(t, pool, `SELECT count(*) FROM team_sources WHERE source = 'espn' AND "sourceId" LIKE 'nba:%'`, 30)
	assertCount(t, pool, `SELECT count(*) FROM event_sources s JOIN events e ON e.id = s."eventId" WHERE s.source = 'espn' AND e."leagueCode" = 'NBA' AND s."sourceId" LIKE 'nba:4019098%'`, 3)
	assertCount(t, pool, `SELECT count(*) FROM player_aliases a JOIN players p ON p.id = a."playerId" WHERE a.source = 'espn' AND a."sourceId" LIKE 'nba:%' AND p."teamId" = (SELECT "teamId" FROM team_sources WHERE source = 'espn' AND "sourceId" = 'nba:30')`, 6)
	assertCount(t, pool, `SELECT count(*) FROM team_aliases a JOIN team_sources s ON s."teamId" = a."teamId" WHERE s.source = 'espn' AND s."sourceId" = 'nba:30' AND a."sourceName" IN ('Charlotte Hornets', 'Hornets', 'CHA')`, 3)
	assertCount(t, pool, `SELECT count(*) FROM event_sources WHERE source = 'euroleague' AND "sourceId" IN ('E2026:69', 'E2026:71', 'E2026:72', 'E2026:74')`, 4)
	assertCount(t, pool, `SELECT count(*) FROM player_aliases WHERE source = 'euroleague' AND "createdAt" >= $1`, 20, since)
	assertCount(t, pool, `SELECT count(*) FROM event_sources WHERE source = 'liquipedia' AND "sourceId" LIKE 'DOTA2:%' AND "createdAt" >= $1`, 6, since)
	assertCount(t, pool, `SELECT count(*) FROM team_sources WHERE source = 'opendota' AND "sourceId" = '10149530'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM player_aliases a JOIN players p ON p.id = a."playerId"
		JOIN team_sources s ON s."teamId" = p."teamId" AND s.source = 'opendota' AND s."sourceId" = '10149530' WHERE a.source = 'opendota'`, 5)
}

func TestTennisSeedCreatesCompetitorTeamAndPlayer(t *testing.T) {
	pool := openTestPool(t)
	var since time.Time
	if err := pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since); err != nil {
		t.Fatalf("db now: %v", err)
	}
	removeOtherSportRowsCreatedSince(t, pool, since)
	store := NewStore(pool)
	miamiDay := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC)
	seedTwice(t, store, newReplaySeeder(t, store, miamiDay),
		League{Code: "ATP_MIAMI", Name: "ATP Miami", Country: "USA", Sport: canon.SportTennis},
		League{Code: "WTA_MIAMI", Name: "WTA Miami", Country: "USA", Sport: canon.SportTennis},
	)

	assertCount(t, pool, `SELECT count(*) FROM event_sources s JOIN events e ON e.id = s."eventId" WHERE s.source = 'espn' AND e."leagueCode" = 'ATP_MIAMI' AND s."sourceId" LIKE 'tennis:%'`, 3)
	assertCount(t, pool, `SELECT count(*) FROM event_sources s JOIN events e ON e.id = s."eventId" WHERE s.source = 'espn' AND e."leagueCode" = 'WTA_MIAMI'`, 2)
	assertCount(t, pool, `SELECT count(*) FROM team_sources s JOIN players p ON p."teamId" = s."teamId" JOIN teams t ON t.id = s."teamId"
		WHERE s.source = 'espn' AND s."sourceId" LIKE 'tennis:%' AND p.name = t.name AND s."createdAt" >= $1`, 10, since)
}
