package resolve

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestRecordSourceLeaguesStoresTheFeedLeagueIdempotently(t *testing.T) {
	r, _, pool, rdb := openIntegrationResolver(t)
	suffix := uuid.NewString()[:8]
	source := "resolve-it-" + suffix
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	homeName, awayName := "Kettlewell Rovers "+suffix, "Barnsworth Town "+suffix
	homeID := insertTeam(t, pool, "EPL", homeName)
	awayID := insertTeam(t, pool, "EPL", awayName)
	t.Cleanup(func() { cleanupResolveRows(pool, rdb, source, []string{homeID, awayID}) })
	kickoff := time.Now().UTC().Add(92 * 24 * time.Hour).Truncate(time.Minute)
	_, err = pool.Exec(t.Context(), `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3)`, homeID, awayID, kickoff)
	require.NoError(t, err)
	require.NoError(t, r.Load(t.Context()))

	ev := canon.SourceEvent{SourceID: "game-" + suffix, LeagueHint: "EPL", SourceLeagueID: "2911523", Home: homeName, Away: awayName, Kickoff: kickoff}
	_, err = r.LinkEvents(t.Context(), source, []canon.SourceEvent{ev})
	require.NoError(t, err)
	stored := func() string {
		var sourceLeagueID string
		require.NoError(t, pool.QueryRow(t.Context(), `SELECT "sourceLeagueId" FROM event_links WHERE source = $1 AND "sourceEventId" = $2`, source, ev.SourceID).Scan(&sourceLeagueID))
		return sourceLeagueID
	}
	for range 2 {
		require.NoError(t, r.RecordSourceLeagues(t.Context(), source, []canon.SourceEvent{ev}))
		assert.Equal(t, "2911523", stored())
	}

	ev.SourceLeagueID = ""
	require.NoError(t, r.RecordSourceLeagues(t.Context(), source, []canon.SourceEvent{ev}))
	assert.Equal(t, "2911523", stored(), "an event without a league id keeps the stored one")
}
