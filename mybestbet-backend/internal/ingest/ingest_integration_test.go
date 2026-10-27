package ingest

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/resolve"
	"mybestbet/internal/review"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/fake"
)

const (
	integrationWait     = 20 * time.Second
	expectedFakeDeltas  = 4
	deltaPollInterval   = 200 * time.Millisecond
	fakeEventsPerSource = 2
)

type fakeFixtureTeam struct {
	league string
	name   string
}

var fakeFixtureTeams = []fakeFixtureTeam{
	{"EPL", "Arsenal"}, {"EPL", "Chelsea"}, {"LALIGA", "Real Madrid"}, {"LALIGA", "Barcelona"},
}

func openIntegrationInfra(t *testing.T) (*pgxpool.Pool, *goredis.Client) {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL are required")
	}
	pool, err := postgres.Open(t.Context(), dbURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	rdb, err := redis.Open(t.Context(), redisURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	return pool, rdb
}

func aliasFakeTeams(t *testing.T, pool *pgxpool.Pool, source string) {
	t.Helper()
	ctx := t.Context()
	for _, league := range []config.League{{Code: "EPL", Name: "Premier League", Country: "England"}, {Code: "LALIGA", Name: "La Liga", Country: "Spain"}} {
		_, err := pool.Exec(ctx, `INSERT INTO leagues (code, name, country) VALUES ($1, $2, $3) ON CONFLICT (code) DO NOTHING`, league.Code, league.Name, league.Country)
		require.NoError(t, err)
	}
	for _, team := range fakeFixtureTeams {
		var id string
		err := pool.QueryRow(ctx, `SELECT id::text FROM teams WHERE "leagueCode" = $1 AND name = $2 ORDER BY "createdAt" LIMIT 1`, team.league, team.name).Scan(&id)
		if err != nil {
			require.NoError(t, pool.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ($1, $2, lower($2)) RETURNING id::text`, team.league, team.name).Scan(&id))
		}
		_, err = pool.Exec(ctx, `INSERT INTO team_aliases (source, "sourceName", "teamId", confidence) VALUES ($1, $2, $3, 1) ON CONFLICT DO NOTHING`, source, team.name, id)
		require.NoError(t, err)
	}
}

func cleanupSourceRows(pool *pgxpool.Pool, rdb *goredis.Client, source string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var linkedEvents []string
	_ = pool.QueryRow(ctx, `SELECT coalesce(array_agg("eventId"::text), '{}') FROM event_links WHERE source = $1`, source).Scan(&linkedEvents)
	_, _ = pool.Exec(ctx, `DELETE FROM event_links WHERE source = $1`, source)
	_, _ = pool.Exec(ctx, `DELETE FROM events e WHERE e.id = ANY($1::uuid[]) AND e."sofascoreId" IS NULL
		AND NOT EXISTS (SELECT 1 FROM event_links l WHERE l."eventId" = e.id)`, linkedEvents)
	_, _ = pool.Exec(ctx, `DELETE FROM team_aliases WHERE source = $1`, source)
	_, _ = pool.Exec(ctx, `DELETE FROM review_queue WHERE source = $1`, source)
	_, _ = rdb.Del(ctx, redis.SourceHealthKey(source), "resolve:held:"+source).Result()
}

func readSourceDeltas(t *testing.T, rdb *goredis.Client, source, afterID string) []canon.Delta {
	t.Helper()
	entries, err := rdb.XRange(t.Context(), redis.StreamOddsDelta, "("+afterID, "+").Result()
	require.NoError(t, err)
	var deltas []canon.Delta
	for _, e := range entries {
		raw, isDelta := e.Values[redis.StreamField].(string)
		if !isDelta {
			continue
		}
		var d canon.Delta
		require.NoError(t, json.Unmarshal([]byte(raw), &d))
		if d.Source == source {
			deltas = append(deltas, d)
		}
	}
	return deltas
}

func lastStreamID(t *testing.T, rdb *goredis.Client) string {
	t.Helper()
	last, err := rdb.XRevRangeN(t.Context(), redis.StreamOddsDelta, "+", "-", 1).Result()
	require.NoError(t, err)
	if len(last) == 0 {
		return "0-0"
	}
	return last[0].ID
}

func TestFakeSourceFlowsThroughIngestToOddsDelta(t *testing.T) {
	pool, rdb := openIntegrationInfra(t)
	source := "fake-it-" + uuid.NewString()[:8]
	sources.Register(source, fake.New)
	t.Cleanup(func() { cleanupSourceRows(pool, rdb, source) })
	aliasFakeTeams(t, pool, source)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	resolver, err := resolve.New(pool, rdb, review.NewService(pool, rdb), log)
	require.NoError(t, err)
	require.NoError(t, resolver.Load(t.Context()))

	cfg := config.Config{
		Leagues:   []config.League{{Code: "EPL"}, {Code: "LALIGA"}},
		Tiers:     testTiers,
		Discovery: time.Hour,
		Sources:   map[string]sources.Config{source: {Enabled: true, MaxRPS: 50}},
	}
	svc, err := New(cfg, resolver, rdb, nil, log)
	require.NoError(t, err)
	require.Equal(t, []string{source}, svc.Sources())

	afterID := lastStreamID(t, rdb)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- svc.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	var deltas []canon.Delta
	require.Eventually(t, func() bool {
		deltas = readSourceDeltas(t, rdb, source, afterID)
		return len(deltas) >= expectedFakeDeltas
	}, integrationWait, deltaPollInterval)

	events := map[string]bool{}
	codes := map[canon.MarketCode]int{}
	for _, d := range deltas[:expectedFakeDeltas] {
		_, err := uuid.Parse(d.Market.Key.EventID)
		assert.NoError(t, err, "delta carries a canonical event id")
		events[d.Market.Key.EventID] = true
		codes[d.Market.Key.Code]++
		assert.False(t, d.FetchedAt.IsZero())
		assert.False(t, d.ResolvedAt.Before(d.FetchedAt))
	}
	assert.Len(t, events, fakeEventsPerSource)
	assert.Equal(t, map[canon.MarketCode]int{canon.Code1X2: 2, canon.CodeOU: 2}, codes)

	var links int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM event_links WHERE source = $1`, source).Scan(&links))
	assert.Equal(t, fakeEventsPerSource, links)

	require.Eventually(t, func() bool {
		lastSuccess, err := rdb.HGet(t.Context(), redis.SourceHealthKey(source), "lastSuccess").Result()
		return err == nil && lastSuccess != ""
	}, integrationWait, deltaPollInterval)
	health, err := rdb.HGetAll(t.Context(), redis.SourceHealthKey(source)).Result()
	require.NoError(t, err)
	assert.Equal(t, "0", health["degraded"])
	assert.Equal(t, "0", health["consecutiveFailures"])
}
