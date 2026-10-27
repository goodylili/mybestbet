package stats

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const (
	flashscoreFeedPrefix  = "/x/feed/"
	flashscoreRecordedDay = "f_2_-1_3_en_1"
)

func flashscoreRoute(r *http.Request) (string, bool) {
	feed, isFeed := strings.CutPrefix(r.URL.Path, flashscoreFeedPrefix)
	switch {
	case isFeed && strings.HasPrefix(feed, "f_2_") && feed != flashscoreRecordedDay:
		return "feed_empty_day.txt", true
	case isFeed:
		return "feed_" + feed + ".txt", true
	case r.URL.Path == "/tennis/atp-singles/wimbledon/results/":
		return "page_atp-singles_wimbledon_results.html", true
	}
	return "", false
}

func openTennisFixture(t *testing.T) (*pgxpool.Pool, *TennisCollector) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	var since time.Time
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since))
	t.Cleanup(func() { removeFlashscoreRowsCreatedSince(t, pool, since) })

	entities := seed.NewStore(pool)
	require.NoError(t, entities.UpsertLeague(t.Context(), seed.League{Code: "ATP_WIM", Name: "ATP Wimbledon", Country: "England", Sport: canon.SportTennis}))
	client := flashscore.New(sourcestest.Deps(sourcestest.ReplayServer(t, flashscore.SourceName, flashscoreRoute), sources.Config{}))
	return pool, NewTennisCollector(TennisCollectorDeps{
		Flashscore: client, Store: NewStore(pool), Entities: entities, Leagues: []string{"ATP_WIM", "EPL"},
		Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return sourcestest.FixedNow },
	})
}

func removeFlashscoreRowsCreatedSince(t *testing.T, pool *pgxpool.Pool, since time.Time) {
	createdEvents := `SELECT "eventId" FROM event_sources WHERE source = $1 AND "createdAt" >= $2`
	statements := []string{
		`DELETE FROM player_match_stats WHERE "eventId" IN (` + createdEvents + `)`,
		`DELETE FROM events WHERE id IN (` + createdEvents + `)`,
		`DELETE FROM player_aliases WHERE "createdAt" >= $2 AND source = $1`,
		`DELETE FROM players p WHERE p."createdAt" >= $2 AND NOT EXISTS (SELECT 1 FROM player_aliases a WHERE a."playerId" = p.id)
			AND p."teamId" IN (SELECT "teamId" FROM team_sources WHERE source = $1)`,
		`DELETE FROM team_aliases WHERE "createdAt" >= $2 AND source = $1`,
		`DELETE FROM teams WHERE "createdAt" >= $2 AND id IN (SELECT "teamId" FROM team_sources WHERE source = $1)`,
	}
	for _, stmt := range statements {
		if _, err := pool.Exec(context.Background(), stmt, flashscore.SourceName, since); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	createdLeagues := `SELECT code FROM leagues WHERE "createdAt" >= $1 AND sport = 'tennis'`
	for _, stmt := range []string{
		`DELETE FROM team_leagues WHERE "leagueCode" IN (` + createdLeagues + `)`,
		`DELETE FROM leagues l WHERE l.code IN (` + createdLeagues + `)
			AND NOT EXISTS (SELECT 1 FROM events e WHERE e."leagueCode" = l.code) AND NOT EXISTS (SELECT 1 FROM teams t WHERE t."leagueCode" = l.code)`,
	} {
		if _, err := pool.Exec(context.Background(), stmt, since); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(t.Context(), sql, args...).Scan(&n))
	return n
}

func flashscorePlayerID(t *testing.T, pool *pgxpool.Pool, participantID string) string {
	t.Helper()
	var playerID string
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT "playerId"::text FROM player_aliases WHERE source = $1 AND "sourceId" = $2`, flashscore.SourceName, participantID).Scan(&playerID))
	return playerID
}

func TestTennisCollectorStoresAcesAndDoubleFaultsOnce(t *testing.T) {
	pool, collector := openTennisFixture(t)
	const flashscoreStatRows = `SELECT count(*) FROM player_match_stats s JOIN event_sources src ON src."eventId" = s."eventId"
		WHERE src.source = 'flashscore' AND src."sourceId" = ANY($1)`
	finishedIDs := []string{"n5DWLXD5", "nkXJ8mYa", "dGA7OFrf", "vXyt3VEd"}

	collector.CollectRound(t.Context())
	collector.CollectRound(t.Context())

	assert.Equal(t, 8, countRows(t, pool, flashscoreStatRows, finishedIDs))
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM event_sources WHERE source = 'flashscore' AND "sourceId" IN ('Q9aiCq5E', 'QRnYFc63', 'pWTSioKD', '84gXeZ4e')`))
	assert.Equal(t, 4, countRows(t, pool, `SELECT count(*) FROM events e JOIN event_sources src ON src."eventId" = e.id
		WHERE src.source = 'flashscore' AND src."sourceId" = ANY($1) AND e."leagueCode" = 'ATP_WIM' AND e.status = 'finished'`, finishedIDs))

	sinner := flashscorePlayerID(t, pool, "6HdC3z4H")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM players p JOIN teams t ON t.id = p."teamId"
		WHERE p.id = $1 AND p.name = 'Jannik Sinner' AND t.name = 'Jannik Sinner'`, sinner))

	reader := NewReader(pool)
	aces, err := reader.HitRates(t.Context(), sinner, canon.PropAces, 15.5, []int{5})
	require.NoError(t, err)
	assert.Equal(t, HitRate{Window: 5, Matches: 2, Hits: 1, Rate: 0.5}, aces.Windows[0])

	faults, err := reader.HitRates(t.Context(), sinner, canon.PropDoubleFlt, 1.5, []int{5})
	require.NoError(t, err)
	assert.Equal(t, HitRate{Window: 5, Matches: 2, Hits: 1, Rate: 0.5}, faults.Windows[0])

	require.NoError(t, collector.storeMatchStats(t.Context(), flashscore.Match{
		ID: "n5DWLXD5", LeagueCode: "ATP_WIM", Kickoff: time.Unix(1783869000, 0).UTC(), Status: flashscore.StatusFinished,
		Home: flashscore.Player{ID: "6HdC3z4H", Name: "Jannik Sinner", ShortName: "Sinner J."},
		Away: flashscore.Player{ID: "dGbUhw9m", Name: "Alexander Zverev", ShortName: "Zverev A."},
	}))
	assert.Equal(t, 8, countRows(t, pool, flashscoreStatRows, finishedIDs))
}

func TestTennisCollectorStoresDiscoveredTournamentsUnderTheirOwnLeague(t *testing.T) {
	pool, collector := openTennisFixture(t)
	discoveredIDs := []string{"hd9LBSct", "27fjhO64", "44GtNVPa", "QmxjgN3B"}

	collector.CollectRound(t.Context())
	collector.CollectRound(t.Context())

	assert.Equal(t, 8, countRows(t, pool, `SELECT count(*) FROM player_match_stats s JOIN event_sources src ON src."eventId" = s."eventId"
		WHERE src.source = 'flashscore' AND src."sourceId" = ANY($1)`, discoveredIDs))
	rows, err := pool.Query(t.Context(), `SELECT src."sourceId", e."leagueCode", l.name, l.country, l.sport
		FROM event_sources src JOIN events e ON e.id = src."eventId" JOIN leagues l ON l.code = e."leagueCode"
		WHERE src.source = 'flashscore' AND src."sourceId" = ANY($1) ORDER BY src."sourceId" COLLATE "C"`, discoveredIDs)
	require.NoError(t, err)
	type storedLeague struct {
		SourceID, Code, Name, Country, Sport string
	}
	stored, err := pgx.CollectRows(rows, pgx.RowToStructByPos[storedLeague])
	require.NoError(t, err)
	assert.Equal(t, []storedLeague{
		{SourceID: "27fjhO64", Code: "ATP_ALMATY", Name: "ATP Almaty", Country: "Kazakhstan", Sport: "tennis"},
		{SourceID: "44GtNVPa", Code: "WTA_NINGBO", Name: "WTA Ningbo", Country: "China", Sport: "tennis"},
		{SourceID: "QmxjgN3B", Code: "ATP_CH_CALI", Name: "ATP Challenger Cali", Country: "Colombia", Sport: "tennis"},
		{SourceID: "hd9LBSct", Code: "ATP_ALMATY", Name: "ATP Almaty", Country: "Kazakhstan", Sport: "tennis"},
	}, stored)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM leagues WHERE code = 'ATP_WIM' AND name = 'ATP Wimbledon' AND country = 'England'`))
}
