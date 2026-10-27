package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

type shiftedFixture struct {
	source  string
	home    string
	away    string
	homeID  string
	awayID  string
	kickoff time.Time
	realID  string
	copyID  string
}

func newShiftedFixture(t *testing.T, pool *pgxpool.Pool, cleanup func(source string, teamIDs []string)) shiftedFixture {
	t.Helper()
	suffix := uuid.NewString()[:8]
	f := shiftedFixture{
		source:  "resolve-shift-" + suffix,
		home:    "Qwertyville Rovers " + suffix,
		away:    "Zyxwick Albion " + suffix,
		kickoff: time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Minute),
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	f.homeID = insertTeam(t, pool, "EPL", f.home)
	f.awayID = insertTeam(t, pool, "EPL", f.away)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM signals WHERE "leagueCode" = 'EPL' AND "dedupeKey" LIKE $1`, f.source+"%")
		cleanup(f.source, []string{f.homeID, f.awayID})
	})
	f.realID = insertEvent(t, pool, f.homeID, f.awayID, f.kickoff)
	f.copyID = insertEvent(t, pool, f.homeID, f.awayID, f.kickoff.Add(-time.Hour))
	_, err = pool.Exec(t.Context(), `INSERT INTO event_links (source, "sourceEventId", "eventId") VALUES ($1, 'ev', $2)`, f.source, f.copyID)
	require.NoError(t, err)
	return f
}

func insertEvent(t *testing.T, pool *pgxpool.Pool, homeID, awayID string, kickoff time.Time) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3) RETURNING id::text`,
		homeID, awayID, kickoff).Scan(&id))
	return id
}

func linkedEventOf(t *testing.T, pool *pgxpool.Pool, source string) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "eventId"::text FROM event_links WHERE source = $1 AND "sourceEventId" = 'ev'`, source).Scan(&id))
	return id
}

func TestLinkEventsMovesALinkWhoseKickoffDisagrees(t *testing.T) {
	r, _, pool, rdb := openIntegrationResolver(t)
	f := newShiftedFixture(t, pool, func(source string, teamIDs []string) { cleanupResolveRows(pool, rdb, source, teamIDs) })
	require.NoError(t, r.Load(t.Context()))

	current := canon.SourceEvent{SourceID: "ev", LeagueHint: "EPL", Home: f.home, Away: f.away, Kickoff: f.kickoff}
	for range 2 {
		linked, err := r.LinkEvents(t.Context(), f.source, []canon.SourceEvent{current})
		require.NoError(t, err)
		assert.Equal(t, []string{"ev"}, linked)
		assert.Equal(t, f.realID, linkedEventOf(t, pool, f.source))
		cached, ok := r.LinkedEventID(f.source, "ev")
		assert.True(t, ok)
		assert.Equal(t, f.realID, cached)
	}
}

func TestLinkEventsKeepsALinkWithoutAMatchingFixture(t *testing.T) {
	r, _, pool, rdb := openIntegrationResolver(t)
	f := newShiftedFixture(t, pool, func(source string, teamIDs []string) { cleanupResolveRows(pool, rdb, source, teamIDs) })
	require.NoError(t, r.Load(t.Context()))

	moved := canon.SourceEvent{SourceID: "ev", LeagueHint: "EPL", Home: f.home, Away: f.away, Kickoff: f.kickoff.Add(24 * time.Hour)}
	_, err := r.LinkEvents(t.Context(), f.source, []canon.SourceEvent{moved})
	require.NoError(t, err)
	assert.Equal(t, f.copyID, linkedEventOf(t, pool, f.source))
}

func TestMergeRepointsLinksAndClosesSignalsIdempotently(t *testing.T) {
	r, _, pool, rdb := openIntegrationResolver(t)
	f := newShiftedFixture(t, pool, func(source string, teamIDs []string) { cleanupResolveRows(pool, rdb, source, teamIDs) })
	_, err := pool.Exec(t.Context(), `INSERT INTO signals ("dedupeKey", type, "eventId", "leagueCode", "marketKey", legs, "detectedAt")
		VALUES ($1, 'arb', $2, 'EPL', 'm', '[]', now())`, f.source+"-arb", f.copyID)
	require.NoError(t, err)

	merge := eventMerge{duplicate: f.copyID, canonical: f.realID}
	runMerge := func() (int, int) {
		var moved, closed int
		require.NoError(t, r.store.inMergeLock(t.Context(), func(tx mergeTx) error {
			var err error
			moved, closed, err = tx.merge(t.Context(), merge)
			return err
		}))
		return moved, closed
	}
	moved, closed := runMerge()
	assert.Equal(t, 1, moved)
	assert.Equal(t, 1, closed)
	moved, closed = runMerge()
	assert.Zero(t, moved)
	assert.Zero(t, closed)

	var mergedInto string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "mergedInto"::text FROM events WHERE id = $1`, f.copyID).Scan(&mergedInto))
	assert.Equal(t, f.realID, mergedInto)
	assert.Equal(t, f.realID, linkedEventOf(t, pool, f.source))

	require.NoError(t, r.store.inMergeLock(t.Context(), func(tx mergeTx) error {
		events, err := tx.unmergedEventsSince(t.Context(), f.kickoff.Add(-2*time.Hour))
		for _, e := range events {
			assert.NotEqual(t, f.copyID, e.id)
		}
		return err
	}))
}
