package seed

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

type entityFixture struct {
	pool   *pgxpool.Pool
	store  *Store
	source string
	suffix string
}

func newEntityFixture(t *testing.T) entityFixture {
	t.Helper()
	pool := openTestPool(t)
	suffix := uuid.NewString()[:8]
	f := entityFixture{pool: pool, store: NewStore(pool), source: "test-" + suffix, suffix: suffix}
	t.Cleanup(func() { f.removeCreatedRows() })
	return f
}

func (f entityFixture) league(t *testing.T, prefix string, sport canon.Sport) string {
	t.Helper()
	code := prefix + "_" + f.suffix
	require.NoError(t, f.store.UpsertLeague(t.Context(), League{Code: code, Name: code, Country: "Test", Sport: sport}))
	return code
}

func (f entityFixture) removeCreatedRows() {
	ctx := context.Background()
	codes := `SELECT code FROM leagues WHERE code LIKE '%\_` + f.suffix + `'`
	for _, sql := range []string{
		`DELETE FROM events WHERE "leagueCode" IN (` + codes + `)`,
		`DELETE FROM player_aliases WHERE source = $1`,
		`DELETE FROM players WHERE "teamId" IN (SELECT id FROM teams WHERE "leagueCode" IN (` + codes + `))`,
		`DELETE FROM team_aliases WHERE source = $1`,
		`DELETE FROM team_leagues WHERE "leagueCode" IN (` + codes + `)`,
		`DELETE FROM teams WHERE "leagueCode" IN (` + codes + `)`,
		`DELETE FROM leagues WHERE code IN (` + codes + `) AND $1 <> ''`,
	} {
		_, _ = f.pool.Exec(ctx, sql, f.source)
	}
}

func TestUpsertSourceTeamIsIdempotentAndJoinsCups(t *testing.T) {
	f := newEntityFixture(t)
	ctx := t.Context()
	domestic := f.league(t, "DOM", canon.SportFootball)
	cup := f.league(t, "CUP", canon.SportFootball)
	first, err := f.store.UpsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "1", LeagueCode: domestic, Name: "Nacional", Aliases: []string{"CD Nacional"}})
	require.NoError(t, err)
	again, err := f.store.UpsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "1", LeagueCode: domestic, Name: "Nacional"})
	require.NoError(t, err)
	assert.Equal(t, first, again)
	inCup, err := f.store.upsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "1", LeagueCode: cup, Name: "Nacional"}, adoptByIDOrAlias, createMissingTeam)
	require.NoError(t, err)
	assert.Equal(t, first, inCup)
	namesake, err := f.store.upsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "2", LeagueCode: cup, Name: "Nacional"}, adoptByIDOrAlias, createMissingTeam)
	require.NoError(t, err)
	assert.NotEqual(t, first, namesake)
	var homeLeague string
	var memberships int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT "leagueCode", (SELECT count(*) FROM team_leagues WHERE "teamId" = $1) FROM teams WHERE id = $1`, first).Scan(&homeLeague, &memberships))
	assert.Equal(t, domestic, homeLeague)
	assert.Equal(t, 2, memberships)
}

func TestUpsertCompetitorCreatesTeamAndPlayerOnce(t *testing.T) {
	f := newEntityFixture(t)
	ctx := t.Context()
	league := f.league(t, "TEN", canon.SportTennis)
	competitor := SourceTeam{Source: f.source, SourceID: "sinner", LeagueCode: league, Name: "Jannik Sinner"}
	teamID, playerID, err := f.store.UpsertCompetitor(ctx, competitor)
	require.NoError(t, err)
	teamAgain, playerAgain, err := f.store.UpsertCompetitor(ctx, competitor)
	require.NoError(t, err)
	assert.Equal(t, teamID, teamAgain)
	assert.Equal(t, playerID, playerAgain)
	var players int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM players WHERE "teamId" = $1`, teamID).Scan(&players))
	assert.Equal(t, 1, players)
}

func TestUpsertSourceEventAdoptsWithinSportTolerance(t *testing.T) {
	f := newEntityFixture(t)
	f.store.WithKickoffTolerance(func(sport canon.Sport) time.Duration {
		if sport == canon.SportTennis {
			return 12 * time.Hour
		}
		return 15 * time.Minute
	})
	ctx := t.Context()
	league := f.league(t, "TEV", canon.SportTennis)
	home, _, err := f.store.UpsertCompetitor(ctx, SourceTeam{Source: f.source, SourceID: "a", LeagueCode: league, Name: "Player A"})
	require.NoError(t, err)
	away, _, err := f.store.UpsertCompetitor(ctx, SourceTeam{Source: f.source, SourceID: "b", LeagueCode: league, Name: "Player B"})
	require.NoError(t, err)
	kickoff := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	first, err := f.store.UpsertSourceEvent(ctx, SourceEvent{Source: f.source, SourceID: "m1", LeagueCode: league, HomeTeamID: home, AwayTeamID: away, Kickoff: kickoff})
	require.NoError(t, err)
	replayed, err := f.store.UpsertSourceEvent(ctx, SourceEvent{Source: f.source, SourceID: "m1", LeagueCode: league, HomeTeamID: home, AwayTeamID: away, Kickoff: kickoff.Add(time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, first, replayed)
	otherSource, err := f.store.UpsertSourceEvent(ctx, SourceEvent{Source: f.source + "-b", SourceID: "x9", LeagueCode: league, HomeTeamID: away, AwayTeamID: home, Kickoff: kickoff.Add(5 * time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, first, otherSource)
	var stored time.Time
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT kickoff FROM events WHERE id = $1`, first).Scan(&stored))
	assert.True(t, stored.Equal(kickoff.Add(5*time.Hour)))
}

func TestUpsertSourcePlayerReusesClubPlayerForSquadMember(t *testing.T) {
	f := newEntityFixture(t)
	ctx := t.Context()
	clubLeague := f.league(t, "CLUB", canon.SportFootball)
	squadLeague := f.league(t, "NAT", canon.SportFootball)
	clubID, err := f.store.UpsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "club-9825", LeagueCode: clubLeague, Name: "Arsenal"})
	require.NoError(t, err)
	squadID, err := f.store.UpsertSourceTeam(ctx, SourceTeam{Source: f.source, SourceID: "nat-8491", LeagueCode: squadLeague, Name: "England"})
	require.NoError(t, err)
	clubPlayer, err := f.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: f.source, SourceID: "club-roster-1", TeamID: clubID, Name: "Bukayo Saka"})
	require.NoError(t, err)

	tests := []struct {
		name         string
		sourceID     string
		playerName   string
		clubSourceID string
		wantClub     bool
	}{
		{name: "same name at the linked club", sourceID: "961995", playerName: "Bukayo Saka", clubSourceID: "club-9825", wantClub: true},
		{name: "replay is idempotent", sourceID: "961995", playerName: "Bukayo Saka", clubSourceID: "club-9825", wantClub: true},
		{name: "no club link creates a new player", sourceID: "303919", playerName: "Jordan Pickford", clubSourceID: "club-8668", wantClub: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := f.store.UpsertSourcePlayer(ctx, SourcePlayer{Source: f.source, SourceID: tt.sourceID, TeamID: squadID, Name: tt.playerName, ClubSourceID: tt.clubSourceID})
			require.NoError(t, err)
			assert.Equal(t, tt.wantClub, got == clubPlayer)
		})
	}
	var named int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM players WHERE "normalizedName" = 'bukayo saka' AND id IN (SELECT "playerId" FROM player_aliases WHERE source = $1)`, f.source).Scan(&named))
	assert.Equal(t, 1, named)
	var linked bool
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM player_teams WHERE "playerId" = $1 AND "teamId" = $2)`, clubPlayer, squadID).Scan(&linked))
	assert.True(t, linked)
}
