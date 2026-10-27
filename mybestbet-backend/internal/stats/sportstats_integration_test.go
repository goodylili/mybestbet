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
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var sportFixtureSources = []string{espn.SourceName, euroleague.SourceName, esports.LiquipediaSource, esports.OpenDotaSource, bo3.SourceName, acb.SourceName}

type sportFixture struct {
	pool     *pgxpool.Pool
	entities *seed.Store
	store    *Store
}

func openSportFixture(t *testing.T) sportFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	lockSofascoreFixtures(t, pool)
	var since time.Time
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since))
	t.Cleanup(func() { removeSportRowsCreatedSince(t, pool, since) })
	return sportFixture{pool: pool, entities: seed.NewStore(pool), store: NewStore(pool)}
}

func removeSportRowsCreatedSince(t *testing.T, pool *pgxpool.Pool, since time.Time) {
	createdEvents := `SELECT "eventId" FROM event_sources WHERE source = ANY($1) AND "createdAt" >= $2`
	statements := []string{
		`DELETE FROM player_match_stats WHERE "eventId" IN (` + createdEvents + `)`,
		`DELETE FROM events WHERE id IN (` + createdEvents + `)`,
		`DELETE FROM player_aliases WHERE "createdAt" >= $2 AND source = ANY($1)`,
		`DELETE FROM players p WHERE p."createdAt" >= $2 AND NOT EXISTS (SELECT 1 FROM player_aliases a WHERE a."playerId" = p.id)
			AND p."teamId" IN (SELECT "teamId" FROM team_sources WHERE source = ANY($1))`,
		`DELETE FROM team_aliases WHERE "createdAt" >= $2 AND source = ANY($1)`,
		`DELETE FROM teams WHERE "createdAt" >= $2 AND id IN (SELECT "teamId" FROM team_sources WHERE source = ANY($1))`,
	}
	for _, stmt := range statements {
		if _, err := pool.Exec(context.Background(), stmt, sportFixtureSources, since); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
}

func (f sportFixture) event(t *testing.T, league seed.League, source, sourceID string, kickoff time.Time, home, away seed.SourceTeam) SourcedEvent {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, f.entities.UpsertLeague(ctx, league))
	homeID, err := f.entities.UpsertSourceTeam(ctx, home)
	require.NoError(t, err)
	awayID, err := f.entities.UpsertSourceTeam(ctx, away)
	require.NoError(t, err)
	eventID, err := f.entities.UpsertSourceEvent(ctx, seed.SourceEvent{
		Source: source, SourceID: sourceID, LeagueCode: league.Code, HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: kickoff,
	})
	require.NoError(t, err)
	return SourcedEvent{ID: eventID, Source: source, SourceID: sourceID, LeagueCode: league.Code, Kickoff: kickoff,
		HomeTeamID: homeID, AwayTeamID: awayID, HomeTeamName: home.Name, AwayTeamName: away.Name}
}

func sportReplayDeps(t *testing.T, fixtureDir string, route sourcestest.Route) sources.Deps {
	t.Helper()
	return sourcestest.Deps(sourcestest.ReplayServer(t, fixtureDir, route), sources.Config{})
}

func newSportService(t *testing.T, f sportFixture) *Service {
	t.Helper()
	espnDeps := sportReplayDeps(t, espn.SourceName, func(r *http.Request) (string, bool) {
		return "nba_summary_401909848.json", r.URL.Path == "/basketball/nba/summary" && r.URL.Query().Get("event") == "401909848"
	})
	euroLeagueDeps := sportReplayDeps(t, euroleague.SourceName, func(r *http.Request) (string, bool) {
		return "e_game_69_stats.json", r.URL.Path == "/v2/competitions/E/seasons/E2026/games/69/stats"
	})
	openDotaDeps := sportReplayDeps(t, "esports", func(r *http.Request) (string, bool) {
		if r.URL.Path == "/proMatches" {
			return "opendota_pro_matches.json", true
		}
		return "opendota_match_" + strings.TrimPrefix(r.URL.Path, "/matches/") + ".json", strings.HasPrefix(r.URL.Path, "/matches/")
	})
	acbServer := sourcestest.ReplayServer(t, acb.SourceName, acbStatsRoute)
	acbDeps := sourcestest.Deps(acbServer, sources.Config{Extra: map[string]string{"keyPageUrl": acbServer.URL + "/es/liga"}})
	return NewService(ServiceDeps{
		ESPN: espn.New(espnDeps), EuroLeague: euroleague.New(euroLeagueDeps), OpenDota: esports.NewOpenDota(openDotaDeps),
		Bo3: bo3.New(sportReplayDeps(t, bo3.SourceName, bo3StatsRoute)), ACB: acb.New(acbDeps),
		Store: f.store, Entities: f.entities, Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return sourcestest.FixedNow },
	})
}

func collectTwice(t *testing.T, svc *Service, e SourcedEvent) {
	t.Helper()
	for range 2 {
		require.NoError(t, svc.collectSportEventStats(t.Context(), e))
	}
}

func TestESPNBasketballStatsAndHitRates(t *testing.T) {
	f := openSportFixture(t)
	nba := seed.League{Code: "NBA", Name: "NBA", Country: "USA", Sport: canon.SportBasketball}
	kickoff := time.Date(2026, 10, 23, 20, 0, 0, 0, time.UTC)
	e := f.event(t, nba, espn.SourceName, "nba:401909848", kickoff,
		seed.SourceTeam{Source: espn.SourceName, SourceID: "nba:15", LeagueCode: "NBA", Name: "Milwaukee Bucks"},
		seed.SourceTeam{Source: espn.SourceName, SourceID: "nba:4", LeagueCode: "NBA", Name: "Chicago Bulls"})

	awaiting, err := f.store.EventsAwaitingSportStats(t.Context(), StatsWindow{From: kickoff.Add(-time.Hour), To: kickoff.Add(time.Hour), Limit: 100}, []string{"NBA"}, []string{espn.SourceName})
	require.NoError(t, err)
	assert.Contains(t, awaiting, e)

	collectTwice(t, newSportService(t, f), e)

	stats, err := f.store.EventMatchStats(t.Context(), e.ID)
	require.NoError(t, err)
	require.NotEmpty(t, stats)
	var smith MatchStat
	for _, s := range stats {
		var name string
		require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT name FROM players WHERE id = $1`, s.PlayerID).Scan(&name))
		if name == "Jalen Smith" {
			smith = s
		}
	}
	assert.Equal(t, statkey.Values{statkey.Points: 14, statkey.Rebounds: 13, statkey.Assists: 1, statkey.Threes: 2}, smith.Stats)
	assert.Equal(t, e.AwayTeamID, smith.TeamID)
	assert.False(t, smith.IsHome)
	assert.Equal(t, 24, smith.Minutes)

	rates, err := NewReader(f.pool).HitRates(t.Context(), smith.PlayerID, canon.PropPtsRebAst, 25.5, []int{5})
	require.NoError(t, err)
	assert.Equal(t, HitRate{Window: 5, Matches: 1, Hits: 1, Rate: 1}, rates.Windows[0])

	var status string
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT status FROM events WHERE id = $1`, e.ID).Scan(&status))
	assert.Equal(t, "finished", status)
}

func TestEuroLeagueStats(t *testing.T) {
	f := openSportFixture(t)
	league := seed.League{Code: "EUROLEAGUE", Name: "EuroLeague", Country: "Europe", Sport: canon.SportBasketball}
	e := f.event(t, league, euroleague.SourceName, "E2026:69", time.Date(2026, 10, 23, 18, 45, 0, 0, time.UTC),
		seed.SourceTeam{Source: euroleague.SourceName, SourceID: "E:PRS", LeagueCode: league.Code, Name: "Paris Basketball"},
		seed.SourceTeam{Source: euroleague.SourceName, SourceID: "E:PAM", LeagueCode: league.Code, Name: "Valencia Basket"})

	collectTwice(t, newSportService(t, f), e)

	stats, err := f.store.EventMatchStats(t.Context(), e.ID)
	require.NoError(t, err)
	assert.Len(t, stats, 8)
}

func TestDotaKillsAcrossSeriesMaps(t *testing.T) {
	f := openSportFixture(t)
	league := seed.League{Code: "DOTA2", Name: "Dota 2", Country: "International", Sport: canon.SportEsports}
	e := f.event(t, league, esports.LiquipediaSource, "DOTA2:fixture-series", time.Unix(1792778400, 0).UTC(),
		seed.SourceTeam{Source: esports.LiquipediaSource, SourceID: "DOTA2:Inner Circle x Insanity", LeagueCode: league.Code, Name: "Inner Circle x Insanity"},
		seed.SourceTeam{Source: esports.LiquipediaSource, SourceID: "DOTA2:HULIGANI", LeagueCode: league.Code, Name: "HULIGANI"})

	collectTwice(t, newSportService(t, f), e)

	stats, err := f.store.EventMatchStats(t.Context(), e.ID)
	require.NoError(t, err)
	require.Len(t, stats, 10)
	homePlayers := 0
	for _, s := range stats {
		assert.Equal(t, 3.0, s.Stats[statkey.Maps])
		if s.IsHome {
			homePlayers++
		}
	}
	assert.Equal(t, 5, homePlayers)
}
