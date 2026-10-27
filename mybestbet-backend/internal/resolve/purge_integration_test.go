package resolve

import (
	"context"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type purgeFixture struct {
	pool      *pgxpool.Pool
	source    string
	homeID    string
	awayID    string
	kickoff   time.Time
	players   []string
	survivor  string
	duplicate string
}

func newPurgeFixture(t *testing.T, pool *pgxpool.Pool) *purgeFixture {
	t.Helper()
	suffix := uuid.NewString()[:8]
	f := &purgeFixture{
		pool:    pool,
		source:  "resolve-purge-" + suffix,
		kickoff: time.Now().UTC().Add(120 * 24 * time.Hour).Truncate(time.Minute),
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	f.homeID = insertTeam(t, pool, "EPL", "Purgeton Wanderers "+suffix)
	f.awayID = insertTeam(t, pool, "EPL", "Mergeford City "+suffix)
	for i := range 2 {
		var id string
		require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO players (name, "normalizedName", "teamId") VALUES ($1, lower($1), $2) RETURNING id::text`,
			"Purge Player "+suffix+string(rune('a'+i)), f.homeID).Scan(&id))
		f.players = append(f.players, id)
	}
	t.Cleanup(f.cleanup)
	f.survivor = insertEvent(t, pool, f.homeID, f.awayID, f.kickoff)
	f.duplicate = insertEvent(t, pool, f.homeID, f.awayID, f.kickoff.Add(-time.Hour))
	return f
}

func (f *purgeFixture) cleanup() {
	ctx := context.Background()
	teams := []string{f.homeID, f.awayID}
	var events []string
	rows, err := f.pool.Query(ctx, `SELECT id::text FROM events WHERE "homeTeamId" = ANY($1::uuid[]) OR "awayTeamId" = ANY($1::uuid[])`, teams)
	if err == nil {
		events, _ = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	for _, table := range []string{"lineups", "player_match_stats", "closing_prices", "prices_current", "prices_history", "signals", "event_links", "event_sources"} {
		_, _ = f.pool.Exec(ctx, `DELETE FROM `+table+` WHERE "eventId" = ANY($1::uuid[])`, events)
	}
	_, _ = f.pool.Exec(ctx, `DELETE FROM event_links WHERE source = $1`, f.source)
	_, _ = f.pool.Exec(ctx, `DELETE FROM events WHERE id = ANY($1::uuid[])`, events)
	_, _ = f.pool.Exec(ctx, `DELETE FROM players WHERE id = ANY($1::uuid[])`, f.players)
	_, _ = f.pool.Exec(ctx, `DELETE FROM teams WHERE id = ANY($1::uuid[])`, teams)
}

func (f *purgeFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), sql, args...)
	require.NoError(t, err)
}

func (f *purgeFixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.pool.QueryRow(t.Context(), sql, args...).Scan(&n))
	return n
}

func (f *purgeFixture) merge(t *testing.T, r *Resolver, duplicate, canonical string, flipped bool) {
	t.Helper()
	require.NoError(t, r.store.inMergeLock(t.Context(), func(tx mergeTx) error {
		_, _, err := tx.merge(t.Context(), eventMerge{duplicate: duplicate, canonical: canonical, flipped: flipped})
		return err
	}))
}

func (f *purgeFixture) seedDuplicateData(t *testing.T, eventID string) {
	t.Helper()
	f.exec(t, `INSERT INTO lineups ("eventId", "playerId", status) VALUES ($1, $2, 'bench'), ($3, $2, 'starter'), ($3, $4, 'starter')`,
		f.survivor, f.players[0], eventID, f.players[1])
	f.exec(t, `INSERT INTO player_match_stats ("playerId", "eventId", "teamId", "isHome", "playedAt", goals) VALUES ($1, $2, $3, true, $4, 2)`,
		f.players[1], eventID, f.homeID, f.kickoff)
	f.exec(t, `INSERT INTO closing_prices ("marketKey", source, side, label, "eventId", raw, prob, "observedAt") VALUES ($1::text || '|1X2|||', $2, 'home', '', $1::uuid, 2.1, 0.47, now())`,
		eventID, f.source)
	f.exec(t, `INSERT INTO prices_current ("marketKey", source, "eventId", code, raw, form, prob, "observedAt") VALUES ($1::text || '|1X2|||', $2, $1::uuid, '1X2', 2.1, 'decimal', 0.47, now())`,
		eventID, f.source)
	f.exec(t, `INSERT INTO prices_history ("marketKey", source, "eventId", raw, form, prob, "observedAt") VALUES ($1::text || '|1X2|||', $2, $1::uuid, 2.1, 'decimal', 0.47, now())`,
		eventID, f.source)
	f.exec(t, `INSERT INTO signals ("dedupeKey", type, status, "eventId", "leagueCode", "marketKey", legs, "detectedAt") VALUES ($1, 'arb', 'closed', $2, 'EPL', 'm', '[]', now())`,
		f.source+"-"+eventID, eventID)
	f.exec(t, `INSERT INTO event_sources (source, "sourceId", "eventId") VALUES ($1, $2::text, $2::uuid)`, f.source, eventID)
}

const purgeTestTimeout = 3 * time.Minute

func (f *purgeFixture) purge(t *testing.T, r *Resolver, grace time.Duration, batch int, extra ...string) PurgeReport {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), purgeTestTimeout)
	defer cancel()
	report, err := r.purgeMergedEvents(ctx, grace, batch, append([]string{f.duplicate}, extra...))
	require.NoError(t, err)
	return report
}

func (f *purgeFixture) exists(t *testing.T, eventID string) bool {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM events WHERE id = $1`, eventID) == 1
}

func (f *purgeFixture) rowsOn(t *testing.T, table, eventID string) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM `+table+` WHERE "eventId" = $1`, eventID)
}

func TestPurgeMovesSurvivorDataAndDeletesMergedChain(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	f := newPurgeFixture(t, pool)
	chained := insertEvent(t, pool, f.homeID, f.awayID, f.kickoff.Add(-2*time.Hour))
	sofascoreID := rand.Int64N(1<<40) + 1<<41
	f.exec(t, `UPDATE events SET "sofascoreId" = $2, score = '{"home": 1, "away": 0}' WHERE id = $1`, f.duplicate, sofascoreID)
	f.exec(t, `INSERT INTO event_links (source, "sourceEventId", "eventId") VALUES ($1, 'chained', $2)`, f.source, chained)
	f.seedDuplicateData(t, f.duplicate)

	f.merge(t, r, f.duplicate, f.survivor, false)
	f.exec(t, `UPDATE events SET "mergedInto" = $2, "mergedAt" = now(), "mergedFlipped" = false WHERE id = $1`, chained, f.duplicate)

	report := f.purge(t, r, 0, 1, chained)
	assert.GreaterOrEqual(t, report.PurgedEvents, 2)
	assert.GreaterOrEqual(t, report.Batches, 2)

	assert.False(t, f.exists(t, f.duplicate))
	assert.False(t, f.exists(t, chained))
	assert.True(t, f.exists(t, f.survivor))
	assert.Equal(t, f.survivor, linkedEventOfKey(t, pool, f.source, "chained"))
	assert.Equal(t, 1, f.rowsOn(t, "event_sources", f.survivor))

	var status string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT status FROM lineups WHERE "eventId" = $1 AND "playerId" = $2`, f.survivor, f.players[0]).Scan(&status))
	assert.Equal(t, "bench", status)
	assert.Equal(t, 2, f.rowsOn(t, "lineups", f.survivor))

	var (
		goals  int
		isHome bool
	)
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT goals, "isHome" FROM player_match_stats WHERE "eventId" = $1 AND "playerId" = $2`, f.survivor, f.players[1]).Scan(&goals, &isHome))
	assert.Equal(t, 2, goals)
	assert.True(t, isHome)

	var marketKey string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "marketKey" FROM closing_prices WHERE "eventId" = $1`, f.survivor).Scan(&marketKey))
	assert.Equal(t, f.survivor+"|1X2|||", marketKey)

	for _, table := range []string{"prices_current", "prices_history", "signals", "lineups", "player_match_stats", "closing_prices", "event_sources"} {
		assert.Zero(t, f.rowsOn(t, table, f.duplicate), table)
	}

	var (
		inherited *int64
		score     *string
	)
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "sofascoreId", score::text FROM events WHERE id = $1`, f.survivor).Scan(&inherited, &score))
	require.NotNil(t, inherited)
	assert.Equal(t, sofascoreID, *inherited)
	require.NotNil(t, score)
	assert.JSONEq(t, `{"home": 1, "away": 0}`, *score)
}

func TestPurgeIsIdempotent(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	f := newPurgeFixture(t, pool)
	f.seedDuplicateData(t, f.duplicate)
	f.merge(t, r, f.duplicate, f.survivor, false)

	f.purge(t, r, 0, 50)
	snapshot := func() []int {
		var counts []int
		for _, table := range []string{"lineups", "player_match_stats", "closing_prices", "event_sources"} {
			counts = append(counts, f.rowsOn(t, table, f.survivor))
		}
		return counts
	}
	first := snapshot()

	f.purge(t, r, 0, 50)
	assert.Equal(t, first, snapshot())
	assert.True(t, f.exists(t, f.survivor))
	assert.False(t, f.exists(t, f.duplicate))
}

func TestPurgeFlipsOrientationForReversedDuplicates(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	f := newPurgeFixture(t, pool)
	reversed := insertEvent(t, pool, f.awayID, f.homeID, f.kickoff.Add(-30*time.Minute))
	f.seedDuplicateData(t, reversed)
	f.merge(t, r, reversed, f.survivor, true)
	f.exec(t, `INSERT INTO event_links (source, "sourceEventId", "eventId", flipped) VALUES ($1, 'late', $2, false)`, f.source, reversed)

	f.purge(t, r, 0, 50, reversed)

	assert.False(t, f.exists(t, reversed))
	var flipped bool
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT flipped FROM event_links WHERE source = $1 AND "sourceEventId" = 'late'`, f.source).Scan(&flipped))
	assert.True(t, flipped)
	var isHome bool
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "isHome" FROM player_match_stats WHERE "eventId" = $1`, f.survivor).Scan(&isHome))
	assert.False(t, isHome)
	assert.Zero(t, f.rowsOn(t, "closing_prices", f.survivor))
}

func TestPurgeWaitsForGracePeriod(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	f := newPurgeFixture(t, pool)
	f.merge(t, r, f.duplicate, f.survivor, false)
	mergedAt := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	f.exec(t, `UPDATE events SET "mergedAt" = $2 WHERE id = $1`, f.duplicate, mergedAt)

	r.now = func() time.Time { return mergedAt.Add(30 * time.Minute) }
	f.purge(t, r, time.Hour, 50)
	assert.True(t, f.exists(t, f.duplicate))

	r.now = func() time.Time { return mergedAt.Add(2 * time.Hour) }
	f.purge(t, r, time.Hour, 50)
	assert.False(t, f.exists(t, f.duplicate))
}

func TestPurgeStampsLegacyMergesBeforeTheGraceClockStarts(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	f := newPurgeFixture(t, pool)
	f.exec(t, `UPDATE events SET "mergedInto" = $2, "mergedAt" = NULL WHERE id = $1`, f.duplicate, f.survivor)

	f.purge(t, r, time.Hour, 50)
	assert.True(t, f.exists(t, f.duplicate))
	assert.Equal(t, 1, f.count(t, `SELECT count(*) FROM events WHERE id = $1 AND "mergedAt" IS NOT NULL`, f.duplicate))
}

func TestJobLockSkipsWhileAnotherReplicaHoldsIt(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	ctx, cancel := context.WithTimeout(t.Context(), purgeTestTimeout)
	defer cancel()

	holder, err := pool.Acquire(ctx)
	require.NoError(t, err)
	_, err = holder.Exec(ctx, `SELECT pg_advisory_lock($1)`, dedupJobLockKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = holder.Exec(context.Background(), `SELECT pg_advisory_unlock_all()`)
		holder.Release()
	})

	called := false
	ran, err := r.store.withJobLock(ctx, func() error { called = true; return nil })
	require.NoError(t, err)
	assert.False(t, ran)
	assert.False(t, called)

	_, err = holder.Exec(ctx, `SELECT pg_advisory_unlock($1)`, dedupJobLockKey)
	require.NoError(t, err)
	ran, err = r.store.withJobLock(ctx, func() error { called = true; return nil })
	require.NoError(t, err)
	assert.True(t, ran)
	assert.True(t, called)

	var relocked bool
	require.NoError(t, holder.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, dedupJobLockKey).Scan(&relocked))
	assert.True(t, relocked)
}

func TestJobLockAdmitsOneConcurrentPurgeAtATime(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	f := newPurgeFixture(t, pool)
	f.seedDuplicateData(t, f.duplicate)
	f.merge(t, r, f.duplicate, f.survivor, false)
	ctx, cancel := context.WithTimeout(t.Context(), purgeTestTimeout)
	defer cancel()

	const runners = 4
	var (
		wg              sync.WaitGroup
		start           = make(chan struct{})
		active, maxSeen atomic.Int32
		ran             atomic.Int32
		errs            = make([]error, runners)
	)
	for i := range runners {
		wg.Go(func() {
			<-start
			var locked bool
			locked, errs[i] = r.store.withJobLock(ctx, func() error {
				n := active.Add(1)
				defer active.Add(-1)
				for {
					seen := maxSeen.Load()
					if n <= seen || maxSeen.CompareAndSwap(seen, n) {
						break
					}
				}
				_, err := r.purgeMergedEvents(ctx, 0, 50, []string{f.duplicate})
				return err
			})
			if locked {
				ran.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.GreaterOrEqual(t, ran.Load(), int32(1))
	assert.Equal(t, int32(1), maxSeen.Load())
	assert.False(t, f.exists(t, f.duplicate))
	assert.Equal(t, 2, f.rowsOn(t, "lineups", f.survivor))
	assert.Equal(t, 1, f.rowsOn(t, "player_match_stats", f.survivor))
}

func linkedEventOfKey(t *testing.T, pool *pgxpool.Pool, source, sourceEventID string) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "eventId"::text FROM event_links WHERE source = $1 AND "sourceEventId" = $2`, source, sourceEventID).Scan(&id))
	return id
}
