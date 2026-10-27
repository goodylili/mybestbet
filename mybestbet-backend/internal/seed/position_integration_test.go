package seed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestUpsertSourcePlayerRecordsKnownPositionIdempotently(t *testing.T) {
	f := newEntityFixture(t)
	ctx := t.Context()
	league := f.league(t, "POS", canon.SportFootball)
	teamID, err := f.store.UpsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "club", LeagueCode: league, Name: "Club " + f.suffix})
	require.NoError(t, err)
	positionAfter := func(position string) (string, *string, time.Time) {
		t.Helper()
		playerID, err := f.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: f.source, SourceID: "p1", TeamID: teamID, Name: "Ada Defender", Position: position})
		require.NoError(t, err)
		var stored *string
		var updatedAt time.Time
		require.NoError(t, f.pool.QueryRow(ctx, `SELECT position, "updatedAt" FROM players WHERE id = $1`, playerID).Scan(&stored, &updatedAt))
		return playerID, stored, updatedAt
	}

	firstID, first, firstUpdated := positionAfter("")
	assert.Nil(t, first)
	knownID, known, knownUpdated := positionAfter("DF")
	require.NotNil(t, known)
	assert.Equal(t, "DF", *known)
	repeatID, repeated, repeatUpdated := positionAfter("DF")
	require.NotNil(t, repeated)
	assert.Equal(t, "DF", *repeated)
	assert.Equal(t, knownUpdated, repeatUpdated)
	missingID, kept, _ := positionAfter("")
	require.NotNil(t, kept)
	assert.Equal(t, "DF", *kept)
	_, moved, _ := positionAfter("MF")
	require.NotNil(t, moved)
	assert.Equal(t, "MF", *moved)

	assert.Equal(t, []string{firstID, firstID, firstID}, []string{knownID, repeatID, missingID})
	assert.False(t, knownUpdated.Before(firstUpdated))
	var players int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM players WHERE "teamId" = $1`, teamID).Scan(&players))
	assert.Equal(t, 1, players)
}

func TestPlayerPositionRejectsUnnormalizedValues(t *testing.T) {
	f := newEntityFixture(t)
	ctx := t.Context()
	league := f.league(t, "RAW", canon.SportFootball)
	teamID, err := f.store.UpsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "club", LeagueCode: league, Name: "Club " + f.suffix})
	require.NoError(t, err)
	_, err = f.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: f.source, SourceID: "p2", TeamID: teamID, Name: "Raw Midfielder", Position: "midfielders"})
	assert.ErrorContains(t, err, "players_position_check")
}
