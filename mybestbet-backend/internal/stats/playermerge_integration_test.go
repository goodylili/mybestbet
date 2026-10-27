package stats

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type twinWorld struct {
	pool      *pgxpool.Pool
	suffix    string
	national  string
	clubs     []string
	eventID   string
	observed  time.Time
	playerIDs []string
}

func openTwinWorld(t *testing.T) *twinWorld {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	w := &twinWorld{pool: pool, suffix: uuid.NewString()[:8], observed: time.Now().UTC().Truncate(time.Second)}
	w.exec(t, `INSERT INTO leagues (code, name, country) VALUES ('WC', 'FIFA World Cup', 'International'), ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	w.national = w.insertID(t, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('WC', $1, $1) RETURNING id::text`, "twinland "+w.suffix)
	for _, club := range []string{"twin united ", "twin city "} {
		w.clubs = append(w.clubs, w.insertID(t, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, club+w.suffix))
	}
	w.eventID = w.insertID(t, `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff, status) VALUES ('EPL', $1, $2, $3, 'finished') RETURNING id::text`,
		w.clubs[0], w.clubs[1], w.observed.Add(-48*time.Hour))
	w.exec(t, `SELECT ensure_prices_history_partition($1::date)`, w.observed)
	t.Cleanup(func() { w.cleanup(t) })
	return w
}

func (w *twinWorld) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := w.pool.Exec(t.Context(), sql, args...)
	require.NoError(t, err)
}

func (w *twinWorld) insertID(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var id string
	require.NoError(t, w.pool.QueryRow(t.Context(), sql, args...).Scan(&id))
	return id
}

func (w *twinWorld) player(t *testing.T, name, teamID string) string {
	t.Helper()
	id := w.insertID(t, `INSERT INTO players (name, "normalizedName", "teamId") VALUES ($1, $1, $2) RETURNING id::text`, name+" "+w.suffix, teamID)
	w.playerIDs = append(w.playerIDs, id)
	w.exec(t, `INSERT INTO player_teams ("playerId", "teamId") VALUES ($1, $2)`, id, teamID)
	return id
}

func (w *twinWorld) alias(t *testing.T, source, sourceID, playerID string) {
	t.Helper()
	w.exec(t, `INSERT INTO player_aliases (source, "sourceId", "playerId", confidence) VALUES ($1, $2, $3, 1)`, source, sourceID+w.suffix, playerID)
}

func (w *twinWorld) playedFor(t *testing.T, playerID, teamID string, minutes int) {
	t.Helper()
	w.exec(t, `INSERT INTO player_match_stats ("playerId", "eventId", "teamId", "isHome", "playedAt", minutes) VALUES ($1, $2, $3, true, $4, $5)`,
		playerID, w.eventID, teamID, w.observed.Add(-48*time.Hour), minutes)
}

func (w *twinWorld) marketKey(playerID string) string {
	return w.eventID + "|P_SHOTS|1.5|" + playerID + "|"
}

func (w *twinWorld) price(t *testing.T, playerID, source string, raw float64, observed time.Time) {
	t.Helper()
	w.exec(t, `INSERT INTO prices_current ("marketKey", source, side, label, "eventId", code, line, "playerId", raw, form, prob, "observedAt")
		VALUES ($1, $2, 'over', '', $3, 'P_SHOTS', 1.5, $4, $5, 'decimal', 0.4, $6)`, w.marketKey(playerID), source, w.eventID, playerID, raw, observed)
	w.exec(t, `INSERT INTO prices_history ("marketKey", source, side, label, "eventId", raw, form, prob, "observedAt")
		VALUES ($1, $2, 'over', '', $3, $4, 'decimal', 0.4, $5)`, w.marketKey(playerID), source, w.eventID, raw, observed)
	w.exec(t, `INSERT INTO closing_prices ("marketKey", source, side, label, "eventId", raw, prob, "observedAt")
		VALUES ($1, $2, 'over', '', $3, $4, 0.4, $5)`, w.marketKey(playerID), source, w.eventID, raw, observed)
}

func (w *twinWorld) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, w.pool.QueryRow(t.Context(), sql, args...).Scan(&n))
	return n
}

func (w *twinWorld) exists(t *testing.T, playerID string) bool {
	t.Helper()
	return w.count(t, `SELECT count(*) FROM players WHERE id = $1`, playerID) == 1
}

func (w *twinWorld) cleanup(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	teams := append([]string{w.national}, w.clubs...)
	statements := []string{
		`DELETE FROM prices_history WHERE "eventId" = $1::uuid`,
		`DELETE FROM closing_prices WHERE "eventId" = $1::uuid`,
		`DELETE FROM prices_current WHERE "eventId" = $1::uuid`,
		`DELETE FROM player_match_stats WHERE "eventId" = $1::uuid OR "playerId" = ANY($2::uuid[])`,
		`DELETE FROM lineups WHERE "eventId" = $1::uuid OR "playerId" = ANY($2::uuid[])`,
		`DELETE FROM player_season_stats WHERE "playerId" = ANY($2::uuid[])`,
		`DELETE FROM player_aliases WHERE "playerId" = ANY($2::uuid[])`,
		`DELETE FROM players WHERE id = ANY($2::uuid[])`,
		`DELETE FROM events WHERE id = $1::uuid`,
		`DELETE FROM teams WHERE id = ANY($3::uuid[])`,
	}
	for _, stmt := range statements {
		if _, err := w.pool.Exec(ctx, `WITH noop AS (SELECT $1::text, $2::uuid[], $3::uuid[]) `+stmt, w.eventID, w.playerIDs, teams); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
}

func newTestPlayerMerger(pool *pgxpool.Pool) *PlayerMerger {
	return NewPlayerMerger(PlayerMergerDeps{Store: NewStore(pool), Log: slog.New(slog.DiscardHandler)})
}

func TestPlayerMergerMovesEveryReferenceToTheClubTwin(t *testing.T) {
	w := openTwinWorld(t)
	duplicate := w.player(t, "bukayo twin", w.national)
	survivor := w.player(t, "bukayo twin", w.clubs[0])
	w.alias(t, "fotmob", "961995-", duplicate)
	w.alias(t, "betway", "B. Twin ", duplicate)
	w.alias(t, "betway", "Bukayo Twin ", survivor)
	w.exec(t, `UPDATE players SET position = 'FW' WHERE id = $1`, duplicate)
	w.playedFor(t, duplicate, w.clubs[0], 90)
	w.exec(t, `INSERT INTO lineups ("eventId", "playerId", status) VALUES ($1, $2, 'starter'), ($1, $3, 'bench')`, w.eventID, duplicate, survivor)
	w.exec(t, `INSERT INTO player_season_stats ("playerId", "leagueCode", season, source, "sourcePlayerId", matches) VALUES ($1, 'EPL', '2026', 'fbref', 'x', 9)`, duplicate)
	w.price(t, duplicate, "betway", 2.5, w.observed.Add(-time.Hour))
	w.price(t, duplicate, "sportybet", 2.4, w.observed.Add(-time.Hour))
	w.price(t, survivor, "sportybet", 2.2, w.observed)

	report, err := newTestPlayerMerger(w.pool).MergeDuplicates(t.Context())
	require.NoError(t, err)
	assert.True(t, report.Locked)
	assert.GreaterOrEqual(t, report.Merged, 1)

	assert.False(t, w.exists(t, duplicate))
	assert.True(t, w.exists(t, survivor))
	for table, want := range map[string]int{
		"player_aliases": 3, "player_match_stats": 1, "lineups": 1, "player_season_stats": 1, "prices_current": 2,
	} {
		assert.Equal(t, want, w.count(t, `SELECT count(*) FROM `+table+` WHERE "playerId" = $1`, survivor), table)
		assert.Zero(t, w.count(t, `SELECT count(*) FROM `+table+` WHERE "playerId" = $1`, duplicate), table)
	}
	assert.Equal(t, 1, w.count(t, `SELECT count(*) FROM player_aliases WHERE source = 'fotmob' AND "sourceId" = $1 AND "playerId" = $2`, "961995-"+w.suffix, survivor))
	assert.Equal(t, "bench", w.scalar(t, `SELECT status FROM lineups WHERE "playerId" = $1`, survivor))
	assert.Equal(t, "FW", w.scalar(t, `SELECT position FROM players WHERE id = $1`, survivor))
	assert.ElementsMatch(t, []string{w.national, w.clubs[0]}, w.strings(t, `SELECT "teamId"::text FROM player_teams WHERE "playerId" = $1`, survivor))
	assert.Equal(t, 2.2, w.float(t, `SELECT raw FROM prices_current WHERE "marketKey" = $1 AND source = 'sportybet'`, w.marketKey(survivor)))
	assert.Equal(t, 2.5, w.float(t, `SELECT raw FROM prices_current WHERE "marketKey" = $1 AND source = 'betway'`, w.marketKey(survivor)))
	for table, want := range map[string]int{"prices_history": 3, "closing_prices": 2} {
		assert.Zero(t, w.count(t, `SELECT count(*) FROM `+table+` WHERE "marketKey" = $1`, w.marketKey(duplicate)), table)
		assert.Equal(t, want, w.count(t, `SELECT count(*) FROM `+table+` WHERE "marketKey" = $1`, w.marketKey(survivor)), table)
	}
	assert.NotContains(t, report.Ambiguous, "bukayo twin "+w.suffix)
}

func TestPlayerMergerIsIdempotent(t *testing.T) {
	w := openTwinWorld(t)
	duplicate := w.player(t, "rerun twin", w.national)
	survivor := w.player(t, "rerun twin", w.clubs[0])
	w.alias(t, "fotmob", "rerun-", duplicate)
	w.playedFor(t, duplicate, w.clubs[0], 70)
	merger := newTestPlayerMerger(w.pool)

	_, err := merger.MergeDuplicates(t.Context())
	require.NoError(t, err)
	snapshot := w.references(t, survivor)
	_, err = merger.MergeDuplicates(t.Context())
	require.NoError(t, err)

	assert.False(t, w.exists(t, duplicate))
	assert.Equal(t, snapshot, w.references(t, survivor))
	result, err := w.mergeAgain(t, duplicate, survivor)
	require.NoError(t, err)
	assert.False(t, result.merged)
}

func TestPlayerMergerLeavesUnprovenAndAmbiguousNamesAlone(t *testing.T) {
	w := openTwinWorld(t)
	unlinked := []string{w.player(t, "lonely twin", w.national), w.player(t, "lonely twin", w.clubs[0])}
	ambiguousNational := w.player(t, "split twin", w.national)
	ambiguousClubs := []string{w.player(t, "split twin", w.clubs[0]), w.player(t, "split twin", w.clubs[1])}
	w.playedFor(t, ambiguousNational, w.clubs[0], 90)
	w.exec(t, `INSERT INTO player_teams ("playerId", "teamId") VALUES ($1, $2)`, ambiguousClubs[1], w.clubs[0])
	conflicting := []string{w.player(t, "rival twin", w.national), w.player(t, "rival twin", w.clubs[1])}
	w.alias(t, "fotmob", "rival-a-", conflicting[0])
	w.alias(t, "fotmob", "rival-b-", conflicting[1])
	w.exec(t, `INSERT INTO player_match_stats ("playerId", "eventId", "teamId", "isHome", "playedAt") VALUES ($1, $2, $3, false, $4)`,
		conflicting[0], w.eventID, w.clubs[1], w.observed)

	report, err := newTestPlayerMerger(w.pool).MergeDuplicates(t.Context())
	require.NoError(t, err)

	for _, id := range append(append(append(unlinked, ambiguousNational), ambiguousClubs...), conflicting...) {
		assert.True(t, w.exists(t, id))
	}
	assert.Contains(t, report.Ambiguous, "split twin "+w.suffix)
}

func TestPlayerMergerSkipsWhileAnotherRunHoldsTheLock(t *testing.T) {
	w := openTwinWorld(t)
	duplicate := w.player(t, "locked twin", w.national)
	w.player(t, "locked twin", w.clubs[0])
	w.playedFor(t, duplicate, w.clubs[0], 90)
	conn, err := w.pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(t.Context(), `SELECT pg_advisory_lock($1)`, playerMergeLockKey)
	require.NoError(t, err)

	report, err := newTestPlayerMerger(w.pool).MergeDuplicates(t.Context())
	_, unlockErr := conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, playerMergeLockKey)
	require.NoError(t, unlockErr)

	require.NoError(t, err)
	assert.False(t, report.Locked)
	assert.True(t, w.exists(t, duplicate))
}

func (w *twinWorld) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var v string
	require.NoError(t, w.pool.QueryRow(t.Context(), sql, args...).Scan(&v))
	return v
}

func (w *twinWorld) float(t *testing.T, sql string, args ...any) float64 {
	t.Helper()
	var v float64
	require.NoError(t, w.pool.QueryRow(t.Context(), sql, args...).Scan(&v))
	return v
}

func (w *twinWorld) strings(t *testing.T, sql string, args ...any) []string {
	t.Helper()
	var v []string
	require.NoError(t, w.pool.QueryRow(t.Context(), `SELECT COALESCE(array_agg(x), '{}') FROM (`+sql+`) q(x)`, args...).Scan(&v))
	return v
}

func (w *twinWorld) references(t *testing.T, playerID string) map[string]int {
	t.Helper()
	refs := map[string]int{}
	for _, table := range []string{"player_aliases", "player_teams", "player_match_stats", "lineups", "player_season_stats", "prices_current"} {
		refs[table] = w.count(t, `SELECT count(*) FROM `+table+` WHERE "playerId" = $1`, playerID)
	}
	return refs
}

func (w *twinWorld) mergeAgain(t *testing.T, duplicateID, survivorID string) (playerMergeResult, error) {
	t.Helper()
	conn, err := w.pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	return mergePlayerInto(t.Context(), conn, duplicateID, survivorID)
}
