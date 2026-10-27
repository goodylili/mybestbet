package seed

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var fixtureTeamIDs = []int64{42, 17, 33, 35, 44, 11, 32, 14, 40, 48}

var fixtureEventIDs = []int64{16363897, 16363892, 16363891}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	lockSofascoreFixtures(t, pool)
	return pool
}

const sofascoreFixtureLock = 7700007

func lockSofascoreFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire lock connection: %v", err)
	}
	if _, err := conn.Exec(t.Context(), `SELECT pg_advisory_lock($1)`, sofascoreFixtureLock); err != nil {
		t.Fatalf("lock sofascore fixtures: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, sofascoreFixtureLock)
		conn.Release()
	})
}

func newReplaySofascore(t *testing.T) *sofascore.Client {
	t.Helper()
	srv := sourcestest.ReplayServer(t, sofascore.SourceName, func(r *http.Request) (string, bool) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		switch {
		case path == "/unique-tournament/17/seasons":
			return "unique_tournament_17_seasons.json", true
		case path == "/unique-tournament/17/season/96668/teams":
			return "season_17_96668_teams.json", true
		case path == "/unique-tournament/17/season/96668/events/next/0":
			return "season_17_96668_events_next_0.json", true
		case path == "/team/33/players":
			return "team_33_players.json", true
		case strings.HasPrefix(path, "/team/"):
			return "team_empty_players.json", true
		}
		return "", false
	})
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Config.BaseURL = srv.URL + "/api/v1"
	return sofascore.New(deps)
}

func removeRowsCreatedSince(t *testing.T, pool *pgxpool.Pool, since time.Time) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		teamsCreated := `SELECT id FROM teams WHERE "sofascoreId" = ANY($1::bigint[])`
		statements := []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM events WHERE "sofascoreId" = ANY($1::bigint[]) AND "createdAt" >= $2`, []any{fixtureEventIDs, since}},
			{`DELETE FROM player_aliases WHERE "createdAt" >= $2 AND "playerId" IN (SELECT id FROM players WHERE "teamId" IN (` + teamsCreated + `))`, []any{fixtureTeamIDs, since}},
			{`DELETE FROM players WHERE "createdAt" >= $2 AND "teamId" IN (` + teamsCreated + `)`, []any{fixtureTeamIDs, since}},
			{`DELETE FROM team_aliases WHERE "createdAt" >= $2 AND "teamId" IN (` + teamsCreated + `)`, []any{fixtureTeamIDs, since}},
			{`DELETE FROM teams WHERE "sofascoreId" = ANY($1::bigint[]) AND "createdAt" >= $2`, []any{fixtureTeamIDs, since}},
		}
		for _, stmt := range statements {
			if _, err := pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
}

func TestSeedLeagueIsIdempotent(t *testing.T) {
	pool := openTestPool(t)
	var dbNow time.Time
	if err := pool.QueryRow(t.Context(), `SELECT now()`).Scan(&dbNow); err != nil {
		t.Fatalf("db now: %v", err)
	}
	removeRowsCreatedSince(t, pool, dbNow)

	svc := NewService(newReplaySofascore(t), nil, NewStore(pool), slog.New(slog.DiscardHandler), func() time.Time { return sourcestest.FixedNow })
	league := League{Code: "EPL", Name: "Premier League", Country: "England", UniqueTournamentID: DefaultUniqueTournaments["EPL"]}

	for run := range 2 {
		if err := svc.SeedLeague(t.Context(), league); err != nil {
			t.Fatalf("seed run %d: %v", run, err)
		}
	}

	assertCount(t, pool, `SELECT count(*) FROM teams WHERE "sofascoreId" = ANY($1) AND "leagueCode" = 'EPL'`, 10, fixtureTeamIDs)
	assertCount(t, pool, `SELECT count(*) FROM team_aliases a JOIN teams t ON t.id = a."teamId" WHERE t."sofascoreId" = 33 AND a.source = 'sofascore' AND a."sourceName" IN ('Tottenham Hotspur', 'Tottenham')`, 2)
	assertCount(t, pool, `SELECT count(*) FROM players p JOIN teams t ON t.id = p."teamId" WHERE t."sofascoreId" = 33 AND p."sofascoreId" IS NOT NULL`, 6)
	assertCount(t, pool, `SELECT count(*) FROM player_aliases WHERE source = 'sofascore' AND "sourceId" = '361420'`, 1)
	assertCount(t, pool, `SELECT count(*) FROM events WHERE "sofascoreId" = ANY($1) AND status = 'scheduled'`, 3, fixtureEventIDs)

	var normalized string
	if err := pool.QueryRow(t.Context(), `SELECT "normalizedName" FROM players WHERE "sofascoreId" = 361420`).Scan(&normalized); err != nil {
		t.Fatalf("read player: %v", err)
	}
	if normalized != "dominic solanke" {
		t.Errorf("normalizedName = %q", normalized)
	}
}

func assertCount(t *testing.T, pool *pgxpool.Pool, query string, expected int, args ...any) {
	t.Helper()
	var got int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&got); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	if got != expected {
		t.Errorf("%s = %d, want %d", query, got, expected)
	}
}

func TestSeedLeagueFallsBackToFotMobWhenSofascoreFails(t *testing.T) {
	pool := openTestPool(t)
	const leagueCode = "FOTMOBIT"
	t.Cleanup(func() {
		ctx := context.Background()
		for _, stmt := range []string{
			`DELETE FROM events WHERE "leagueCode" = $1`,
			`DELETE FROM player_aliases WHERE "playerId" IN (SELECT p.id FROM players p JOIN teams t ON t.id = p."teamId" WHERE t."leagueCode" = $1)`,
			`DELETE FROM players WHERE "teamId" IN (SELECT id FROM teams WHERE "leagueCode" = $1)`,
			`DELETE FROM team_aliases WHERE "teamId" IN (SELECT id FROM teams WHERE "leagueCode" = $1)`,
			`DELETE FROM teams WHERE "leagueCode" = $1`,
			`DELETE FROM leagues WHERE code = $1`,
		} {
			if _, err := pool.Exec(ctx, stmt, leagueCode); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
	unreachableSofascore := sourcestest.ReplayServer(t, sofascore.SourceName, func(*http.Request) (string, bool) { return "", false })
	fotmobServer := sourcestest.ReplayServer(t, fotmob.SourceName, func(r *http.Request) (string, bool) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/leagues"):
			return "league_47.json", true
		case strings.HasSuffix(r.URL.Path, "/teams"):
			return "team_9902.json", true
		}
		return "", false
	})
	fotmobDeps := sourcestest.Deps(fotmobServer, sources.Config{LeagueIDs: map[string]string{leagueCode: "47"}})
	svc := NewService(sofascore.New(sourcestest.Deps(unreachableSofascore, sources.Config{})), fotmob.New(fotmobDeps), NewStore(pool), slog.New(slog.DiscardHandler), func() time.Time { return sourcestest.FixedNow })
	league := League{Code: leagueCode, Name: "FotMob fallback", Country: "Test", UniqueTournamentID: 999999}

	for run := range 2 {
		if err := svc.SeedLeague(t.Context(), league); err != nil {
			t.Fatalf("seed run %d: %v", run, err)
		}
	}

	var teams, aliases, events, playersPerTeam int
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM teams WHERE "leagueCode" = $1),
		(SELECT count(*) FROM team_aliases a JOIN teams t ON t.id = a."teamId" WHERE t."leagueCode" = $1 AND a.source = 'fotmob'),
		(SELECT count(*) FROM events WHERE "leagueCode" = $1),
		(SELECT max(n) FROM (SELECT count(*) AS n FROM players p JOIN teams t ON t.id = p."teamId" WHERE t."leagueCode" = $1 GROUP BY p."teamId") squads)`, leagueCode).Scan(&teams, &aliases, &events, &playersPerTeam); err != nil {
		t.Fatalf("count: %v", err)
	}
	if teams == 0 || aliases != teams || events != 1 || playersPerTeam != 12 {
		t.Fatalf("teams=%d aliases=%d events=%d playersPerTeam=%d, want teams>0, one alias per team, one upcoming event and 12 squad players without the coach", teams, aliases, events, playersPerTeam)
	}
}
