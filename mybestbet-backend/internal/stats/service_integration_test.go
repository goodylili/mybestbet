package stats

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/fbref"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const sofascoreFixtureLock = 7700007

var (
	fixtureKickoff    = time.Date(2026, 10, 19, 19, 0, 0, 0, time.UTC)
	fixtureTeamIDs    = []int64{33, 11}
	fixtureEventID    = int64(16363890)
	statsCollectionAt = fixtureKickoff.Add(17 * time.Hour)
)

type fixtureWorld struct {
	pool     *pgxpool.Pool
	entities *seed.Store
	event    ScheduledEvent
	players  map[string]string
}

func openFixtureWorld(t *testing.T) fixtureWorld {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	lockSofascoreFixtures(t, pool)

	var since time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT now()`).Scan(&since))
	removeFixtureRowsCreatedSince(t, pool, since)

	entities := seed.NewStore(pool)
	require.NoError(t, entities.UpsertLeague(ctx, seed.League{Code: "EPL", Name: "Premier League", Country: "England", UniqueTournamentID: 17}))
	homeID, err := entities.UpsertTeam(ctx, seed.Team{SofascoreID: 33, LeagueCode: "EPL", Name: "Tottenham Hotspur", ShortName: "Tottenham"})
	require.NoError(t, err)
	awayID, err := entities.UpsertTeam(ctx, seed.Team{SofascoreID: 11, LeagueCode: "EPL", Name: "Coventry City", ShortName: "Coventry"})
	require.NoError(t, err)
	eventID, err := entities.UpsertEvent(ctx, seed.Event{
		SofascoreID: fixtureEventID, LeagueCode: "EPL", HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: fixtureKickoff, Status: seed.EventScheduled,
	})
	require.NoError(t, err)

	players := map[string]string{}
	for sofascoreID, name := range map[int64]string{361420: "Dominic Solanke", 873554: "Omar Marmoush", 94758: "Ben Davies"} {
		id, err := entities.UpsertPlayer(ctx, seed.Player{SofascoreID: sofascoreID, TeamID: homeID, Name: name})
		require.NoError(t, err)
		players[name] = id
	}
	sofascoreID := fixtureEventID
	event := ScheduledEvent{
		ID: eventID, SofascoreID: &sofascoreID, LeagueCode: "EPL", Kickoff: fixtureKickoff,
		HomeTeamID: homeID, AwayTeamID: awayID, HomeTeamName: "Tottenham Hotspur", AwayTeamName: "Coventry City",
	}
	return fixtureWorld{pool: pool, entities: entities, event: event, players: players}
}

func lockSofascoreFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	conn, err := pool.Acquire(t.Context())
	require.NoError(t, err)
	_, err = conn.Exec(t.Context(), `SELECT pg_advisory_lock($1)`, sofascoreFixtureLock)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, sofascoreFixtureLock)
		conn.Release()
	})
}

func removeFixtureRowsCreatedSince(t *testing.T, pool *pgxpool.Pool, since time.Time) {
	t.Helper()
	t.Cleanup(func() {
		teams := `SELECT id FROM teams WHERE "sofascoreId" = ANY($1::bigint[])`
		createdPlayers := `SELECT id FROM players WHERE "createdAt" >= $2 AND "teamId" IN (` + teams + `)`
		createdEvent := `SELECT id FROM events WHERE "sofascoreId" = $3 AND "createdAt" >= $2`
		statements := []string{
			`DELETE FROM player_season_stats WHERE "playerId" IN (` + createdPlayers + `)`,
			`DELETE FROM player_match_stats WHERE "eventId" IN (` + createdEvent + `) OR "playerId" IN (` + createdPlayers + `)`,
			`DELETE FROM lineups WHERE "eventId" IN (` + createdEvent + `) OR "playerId" IN (` + createdPlayers + `)`,
			`DELETE FROM player_aliases WHERE "playerId" IN (` + createdPlayers + `)`,
			`DELETE FROM players WHERE id IN (` + createdPlayers + `)`,
			`DELETE FROM team_aliases WHERE "createdAt" >= $2 AND "teamId" IN (` + teams + `)`,
			`DELETE FROM events WHERE id IN (` + createdEvent + `)`,
			`DELETE FROM teams WHERE "sofascoreId" = ANY($1::bigint[]) AND "createdAt" >= $2`,
		}
		for _, stmt := range statements {
			if _, err := pool.Exec(context.Background(), `WITH noop AS (SELECT $1::bigint[], $2::timestamptz, $3::bigint) `+stmt, fixtureTeamIDs, since, fixtureEventID); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
}

func replayDeps(t *testing.T, source, basePath string, route func(path, query string) (string, bool)) sources.Deps {
	t.Helper()
	srv := sourcestest.ReplayServer(t, source, func(r *http.Request) (string, bool) {
		return route(strings.TrimPrefix(r.URL.Path, basePath), r.URL.RawQuery)
	})
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Config.BaseURL = srv.URL + basePath
	return deps
}

func newFixtureService(t *testing.T, w fixtureWorld, isSofascoreUp bool) *Service {
	t.Helper()
	sofascoreFixtures := map[string]string{
		"/event/16363890":           "event_16363890.json",
		"/event/16363890/lineups":   "event_16363890_lineups.json",
		"/event/16363890/incidents": "event_16363890_incidents.json",
	}
	sofascoreDeps := replayDeps(t, sofascore.SourceName, "/api/v1", func(path, _ string) (string, bool) {
		name, ok := sofascoreFixtures[path]
		return name, ok && isSofascoreUp
	})
	fotmobDeps := replayDeps(t, fotmob.SourceName, "/api/data", func(path, query string) (string, bool) {
		switch {
		case path == "/leagues" && query == "id=47":
			return "league_47.json", true
		case path == "/matchDetails" && query == "matchId=5795484":
			return "match_5795484.json", true
		}
		return "", false
	})
	fbrefDeps := replayDeps(t, fbref.SourceName, "/en", func(path, _ string) (string, bool) {
		page, ok := strings.CutPrefix(path, "/comps/9/")
		return "comps_9_" + strings.TrimSuffix(page, "/Premier-League-Stats") + ".html", ok
	})
	return NewService(ServiceDeps{
		Sofascore: sofascore.New(sofascoreDeps),
		FotMob:    fotmob.New(fotmobDeps),
		FBref:     fbref.New(fbrefDeps),
		Store:     NewStore(w.pool),
		Entities:  w.entities,
		Leagues:   []string{"EPL"},
		Log:       slog.New(slog.DiscardHandler),
		Now:       func() time.Time { return statsCollectionAt },
	})
}

func TestStatsPipelineAgainstFixtures(t *testing.T) {
	w := openFixtureWorld(t)
	ctx := t.Context()
	store := NewStore(w.pool)
	reader := NewReader(w.pool)

	t.Run("fotmob fills gaps while sofascore is down", func(t *testing.T) {
		svc := newFixtureService(t, w, false)
		svc.collectMatchStats(ctx, statsCollectionAt)

		solanke, err := reader.PlayerStats(ctx, w.players["Dominic Solanke"], 5)
		require.NoError(t, err)
		require.Len(t, solanke, 1)
		assert.Equal(t, 3, solanke[0].Shots)
		assert.Equal(t, 90, solanke[0].Minutes)
		assert.True(t, solanke[0].IsHome)
		assert.Nil(t, solanke[0].Rating)
		assert.Equal(t, "fotmob", solanke[0].ValueSources["shots"])
	})

	t.Run("sofascore overrides fallback and is idempotent", func(t *testing.T) {
		svc := newFixtureService(t, w, true)
		awaiting, err := store.EventsAwaitingStats(ctx, StatsWindow{From: statsCollectionAt.Add(-statsRetryWindow), To: statsCollectionAt.Add(-statsDelayAfterKickoff), Limit: 50}, true)
		require.NoError(t, err)
		event := findEvent(t, awaiting, w.event.ID)
		assert.True(t, event.HasAnyStatsRows)

		for range 2 {
			svc.collectEventStats(ctx, event)
		}

		rows, err := store.EventMatchStats(ctx, w.event.ID)
		require.NoError(t, err)
		sofascoreRows := 0
		for _, r := range rows {
			if r.ValueSources["minutes"] == "sofascore" {
				sofascoreRows++
			}
		}
		assert.Equal(t, 16, sofascoreRows)

		davies, err := reader.PlayerStats(ctx, w.players["Ben Davies"], 5)
		require.NoError(t, err)
		require.Len(t, davies, 1)
		assert.Equal(t, 21, davies[0].Minutes)
		assert.Equal(t, 1, davies[0].Goals)
		assert.False(t, davies[0].Started)
		assert.InDelta(t, 7.9, *davies[0].Rating, 1e-9)
		assert.Equal(t, w.event.AwayTeamID, davies[0].OpponentTeamID)

		var status string
		require.NoError(t, w.pool.QueryRow(ctx, `SELECT status FROM events WHERE id = $1`, w.event.ID).Scan(&status))
		assert.Equal(t, "finished", status)

		awaiting, err = store.EventsAwaitingStats(ctx, StatsWindow{From: statsCollectionAt.Add(-statsRetryWindow), To: statsCollectionAt.Add(-statsDelayAfterKickoff), Limit: 50}, true)
		require.NoError(t, err)
		for _, e := range awaiting {
			assert.NotEqual(t, w.event.ID, e.ID)
		}
	})

	t.Run("late fotmob run cannot overwrite sofascore rows", func(t *testing.T) {
		svc := newFixtureService(t, w, false)
		require.NoError(t, svc.statsFromFotMob(ctx, w.event))
		davies, err := reader.PlayerStats(ctx, w.players["Ben Davies"], 1)
		require.NoError(t, err)
		assert.Equal(t, "sofascore", davies[0].ValueSources["minutes"])
		assert.NotNil(t, davies[0].Rating)
	})

	t.Run("hit rates count only full matches", func(t *testing.T) {
		vanDeVen := playerIDBySofascore(t, w.pool, 998247)
		rates, err := reader.HitRates(ctx, vanDeVen, canon.PropShots, 0.5, nil)
		require.NoError(t, err)
		assert.Equal(t, HitRate{Window: 5, Matches: 1, Hits: 1, Rate: 1}, rates.Windows[0])
		assert.Equal(t, HitRate{Window: 1, Matches: 1, Hits: 1, Rate: 1}, rates.Home)

		davies, err := reader.HitRates(ctx, w.players["Ben Davies"], canon.PropAnytime, 0, []int{10})
		require.NoError(t, err)
		assert.Equal(t, 0, davies.Windows[0].Matches)

		vs, matches, err := reader.VsOpponent(ctx, vanDeVen, w.event.AwayTeamID, canon.PropRating, 6.5)
		require.NoError(t, err)
		assert.Equal(t, HitRate{Window: 1, Matches: 1, Hits: 1, Rate: 1}, vs)
		assert.Len(t, matches, 1)

		distribution, err := reader.RatingDistribution(ctx, vanDeVen)
		require.NoError(t, err)
		assert.Equal(t, []float64{7}, distribution.Ratings)

		trend, err := reader.MinutesTrend(ctx, w.players["Ben Davies"])
		require.NoError(t, err)
		assert.Equal(t, []MinutesPoint{{EventID: w.event.ID, PlayedAt: fixtureKickoff, Minutes: 21}}, utcPoints(trend))

		_, err = reader.HitRates(ctx, "not-a-uuid", canon.PropShots, 0.5, nil)
		assert.ErrorIs(t, err, ErrInvalidID)
	})

	t.Run("confirmed sofascore lineup marks starter bench and out", func(t *testing.T) {
		svc := newFixtureService(t, w, true)
		for range 2 {
			svc.refreshLineup(ctx, w.event, fixtureKickoff.Add(-30*time.Minute))
		}
		var counts struct{ starter, bench, out int }
		require.NoError(t, w.pool.QueryRow(ctx, `SELECT
				count(*) FILTER (WHERE status = 'starter'), count(*) FILTER (WHERE status = 'bench'), count(*) FILTER (WHERE status = 'out')
			FROM lineups WHERE "eventId" = $1`, w.event.ID).Scan(&counts.starter, &counts.bench, &counts.out))
		assert.Equal(t, struct{ starter, bench, out int }{10, 6, 4}, counts)

		status, err := reader.LineupStatus(ctx, w.event.ID, w.players["Ben Davies"])
		require.NoError(t, err)
		assert.Equal(t, LineupBench, status)
		status, err = reader.LineupStatus(ctx, w.event.ID, w.players["Omar Marmoush"])
		require.NoError(t, err)
		assert.Equal(t, LineupUnknown, status)
	})

	t.Run("fbref season stats link by exact name and upsert", func(t *testing.T) {
		svc := newFixtureService(t, w, true)
		for range 2 {
			aliases, err := store.PlayerAliases(ctx, fbref.SourceName)
			require.NoError(t, err)
			require.NoError(t, svc.refreshLeagueSeason(ctx, "EPL", aliases))
		}
		seasons, err := reader.SeasonStats(ctx, w.players["Dominic Solanke"])
		require.NoError(t, err)
		require.Len(t, seasons, 1)
		assert.Equal(t, "2026-2027", seasons[0].Season)
		assert.Equal(t, 12, seasons[0].Shots)
		assert.Equal(t, 8, seasons[0].Fouls)
		assert.InDelta(t, 3.06, seasons[0].ShotsPer90, 0.01)

		var linked int
		require.NoError(t, w.pool.QueryRow(ctx, `SELECT count(*) FROM player_season_stats WHERE "sourcePlayerId" IN ('e77dc3b2', '0e0102eb', 'b38ac6a4')`).Scan(&linked))
		assert.Equal(t, 3, linked)
	})
}

func findEvent(t *testing.T, events []ScheduledEvent, id string) ScheduledEvent {
	t.Helper()
	for _, e := range events {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("event %s not awaiting stats", id)
	return ScheduledEvent{}
}

func playerIDBySofascore(t *testing.T, pool *pgxpool.Pool, sofascoreID int64) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT id FROM players WHERE "sofascoreId" = $1`, sofascoreID).Scan(&id))
	return id
}

func utcPoints(points []MinutesPoint) []MinutesPoint {
	for i := range points {
		points[i].PlayedAt = points[i].PlayedAt.UTC()
	}
	return points
}
