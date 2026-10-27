package engine

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	eventuallyWithin = 60 * time.Second
	eventuallyTick   = 50 * time.Millisecond
)

func integrationDeps(t *testing.T) (*pgxpool.Pool, *goredis.Client) {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Open(ctx, dbURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	rdb, err := redis.Open(ctx, redisURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	return pool, rdb
}

func createTestEvent(t *testing.T, pool *pgxpool.Pool, kickoff time.Time) string {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	suffix := uuid.NewString()
	var home, away, eventID string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, "engine home "+suffix).Scan(&home))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, "engine away "+suffix).Scan(&away))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3) RETURNING id::text`, home, away, kickoff).Scan(&eventID))
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM signals WHERE "eventId" = $1`,
			`DELETE FROM closing_prices WHERE "eventId" = $1`,
			`DELETE FROM prices_history WHERE "eventId" = $1`,
			`DELETE FROM prices_current WHERE "eventId" = $1`,
			`DELETE FROM events WHERE id = $1`,
		} {
			_, _ = pool.Exec(ctx, stmt, eventID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM teams WHERE id IN ($1, $2)`, home, away)
	})
	return eventID
}

func publishDelta(t *testing.T, rdb *goredis.Client, d canon.Delta) {
	t.Helper()
	encoded, err := json.Marshal(d)
	require.NoError(t, err)
	require.NoError(t, rdb.XAdd(context.Background(), &goredis.XAddArgs{Stream: redis.StreamOddsDelta, Values: []any{redis.StreamField, encoded}}).Err())
}

func signalRowsFor(cfg config.Config, deltas []canon.Delta, key canon.MarketKey, at time.Time) int {
	b := newBoard(cfg, func(string) bool { return false })
	rows := 0
	for _, d := range deltas {
		d.Market.Key = key
		rows += len(signalsOf(b.apply(d, at), "", canon.SignalClosed))
	}
	for _, scope := range b.openByScope {
		rows += len(scope)
	}
	return rows
}

func countRows(t *testing.T, pool *pgxpool.Pool, query, eventID string) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), query, eventID).Scan(&n))
	return n
}

func TestEngineIntegrationDeltasToBoardHistoryAndSignal(t *testing.T) {
	pool, rdb := integrationDeps(t)
	eventID := createTestEvent(t, pool, time.Now().Add(time.Hour))
	group := "engine-test-" + uuid.NewString()
	ctx, cancel := context.WithCancel(context.Background())
	key := canon.MarketKey{EventID: eventID, Code: canon.Code1X2}
	t.Cleanup(func() {
		cancel()
		bg := context.Background()
		_ = rdb.XGroupDestroy(bg, redis.StreamOddsDelta, group).Err()
		for _, code := range []canon.MarketCode{canon.Code1X2, canon.CodeDC, canon.CodeDNB} {
			_ = rdb.Del(bg, redis.BoardKey(canon.MarketKey{EventID: eventID, Code: code}.String())).Err()
		}
		_ = rdb.Del(bg, redis.EventMarketsKey(eventID)).Err()
	})

	cfg := testConfig()
	cfg.EngineShards, cfg.FlushEvery, cfg.FlushRows = 2, 50*time.Millisecond, 100
	eng, err := New(ctx, Deps{Config: cfg, Pool: pool, Redis: rdb, Log: slog.New(slog.DiscardHandler), Group: group, Consumer: group, SpoolDir: t.TempDir()})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- eng.Run(ctx) }()

	now := time.Now().UTC().Truncate(time.Millisecond)
	deltas := []canon.Delta{
		matchResultDelta("itest-a", 2.3, 3.4, 3.2, now),
		matchResultDelta("itest-b", 2.1, 3.9, 3.3, now),
		matchResultDelta("itest-c", 2.0, 3.5, 4.4, now),
	}
	for _, d := range deltas {
		d.Market.Key = key
		publishDelta(t, rdb, d)
		require.Eventually(t, func() bool {
			fields, err := rdb.HGetAll(ctx, redis.BoardKey(key.String())).Result()
			return err == nil && fields[redis.BoardField(d.Source, "home", "")] != ""
		}, eventuallyWithin, eventuallyTick)
	}
	assert.Eventually(t, func() bool {
		fields, err := rdb.HGetAll(ctx, redis.BoardKey(key.String())).Result()
		return err == nil && fields[redis.BoardField(consensusSource, "home", "")] != ""
	}, eventuallyWithin, eventuallyTick)
	isMember, err := rdb.SIsMember(ctx, redis.EventMarketsKey(eventID), key.String()).Result()
	require.NoError(t, err)
	assert.True(t, isMember)

	assert.Eventually(t, func() bool {
		return countRows(t, pool, `SELECT count(*) FROM prices_history WHERE "eventId" = $1`, eventID) == 9 &&
			countRows(t, pool, `SELECT count(*) FROM prices_current WHERE "eventId" = $1`, eventID) == 9 &&
			countRows(t, pool, `SELECT count(*) FROM closing_prices WHERE "eventId" = $1`, eventID) == 9
	}, eventuallyWithin, eventuallyTick)

	assert.Eventually(t, func() bool {
		return countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1 AND type = 'arb' AND status = 'open' AND "leagueCode" = 'EPL'`, eventID) == 1
	}, eventuallyWithin, eventuallyTick)
	assert.Eventually(t, func() bool {
		entries, err := rdb.XRevRangeN(ctx, redis.StreamSignals, "+", "-", 50).Result()
		if err != nil {
			return false
		}
		for _, entry := range entries {
			if raw, _ := entry.Values[redis.StreamField].(string); strings.Contains(raw, eventID) {
				return true
			}
		}
		return false
	}, eventuallyWithin, eventuallyTick)

	signalsBeforeReplay := signalRowsFor(cfg, deltas, key, now)
	assert.Eventually(t, func() bool {
		return countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1`, eventID) == signalsBeforeReplay
	}, eventuallyWithin, eventuallyTick)
	replay := deltas[0]
	replay.Market.Key = key
	publishDelta(t, rdb, replay)
	assert.Eventually(t, func() bool {
		pending, err := rdb.XPending(ctx, redis.StreamOddsDelta, group).Result()
		return err == nil && pending.Count == 0
	}, eventuallyWithin, eventuallyTick)
	time.Sleep(4 * cfg.FlushEvery)
	assert.Equal(t, 9, countRows(t, pool, `SELECT count(*) FROM prices_history WHERE "eventId" = $1`, eventID))
	assert.Equal(t, signalsBeforeReplay, countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1`, eventID))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1 AND type = 'arb' AND status = 'open'`, eventID))

	_, err = pool.Exec(ctx, `UPDATE events SET kickoff = now() - interval '1 minute' WHERE id = $1`, eventID)
	require.NoError(t, err)
	eng.closeStartedEventSignals(ctx, time.Now())
	assert.Eventually(t, func() bool {
		return countRows(t, pool, `SELECT count(*) FROM signals WHERE "eventId" = $1 AND status = 'open'`, eventID) == 0
	}, eventuallyWithin, eventuallyTick)

	cancel()
	require.NoError(t, <-done)
}
