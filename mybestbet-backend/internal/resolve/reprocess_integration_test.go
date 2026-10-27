package resolve

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/review"
	"mybestbet/pkg/canon"
)

const reprocessWait = 10 * time.Second

func openIntegrationResolver(t *testing.T) (*Resolver, *review.Service, *pgxpool.Pool, *goredis.Client) {
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
	reviews := review.NewService(pool, rdb)
	r, err := New(pool, rdb, reviews, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	require.NoError(t, err)
	return r, reviews, pool, rdb
}

func insertTeam(t *testing.T, pool *pgxpool.Pool, league, name string) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ($1, $2, lower($2)) RETURNING id::text`, league, name).Scan(&id))
	return id
}

func cleanupResolveRows(pool *pgxpool.Pool, rdb *goredis.Client, source string, teamIDs []string) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM event_links WHERE source = $1`, source)
	_, _ = pool.Exec(ctx, `DELETE FROM team_aliases WHERE source = $1 OR "teamId" = ANY($2::uuid[])`, source, teamIDs)
	_, _ = pool.Exec(ctx, `DELETE FROM review_queue WHERE source = $1`, source)
	_, _ = pool.Exec(ctx, `DELETE FROM events WHERE "homeTeamId" = ANY($1::uuid[]) OR "awayTeamId" = ANY($1::uuid[])`, teamIDs)
	_, _ = pool.Exec(ctx, `DELETE FROM teams WHERE id = ANY($1::uuid[])`, teamIDs)
	_, _ = rdb.Del(ctx, heldEventsKey(source)).Result()
}

func TestReviewDecisionReprocessesBufferedMarkets(t *testing.T) {
	r, reviews, pool, rdb := openIntegrationResolver(t)
	suffix := uuid.NewString()[:8]
	source := "resolve-it-" + suffix
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	homeID := insertTeam(t, pool, "EPL", "Qwertyville Rovers "+suffix)
	awayID := insertTeam(t, pool, "EPL", "Zyxwick Albion "+suffix)
	t.Cleanup(func() { cleanupResolveRows(pool, rdb, source, []string{homeID, awayID}) })
	require.NoError(t, r.Load(t.Context()))

	sub, err := SubscribeAliasUpdates(t.Context(), rdb)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- NewReprocessor(r).Run(ctx, sub) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	kickoff := time.Now().UTC().Add(90 * 24 * time.Hour).Truncate(time.Minute)
	sourceEvent := canon.SourceEvent{SourceID: "ev-" + suffix, LeagueHint: "EPL", Home: "Unrecognisable Name", Away: "Zyxwick Albion " + suffix, Kickoff: kickoff}
	linked, err := r.LinkEvents(t.Context(), source, []canon.SourceEvent{sourceEvent})
	require.NoError(t, err)
	assert.Empty(t, linked)

	afterID := "0-0"
	if last, err := rdb.XRevRangeN(t.Context(), redis.StreamOddsDelta, "+", "-", 1).Result(); err == nil && len(last) > 0 {
		afterID = last[0].ID
	}
	market := canon.RawMarket{
		EventSourceID: sourceEvent.SourceID, Code: canon.CodeAH, Line: canon.Line(-0.5), SourceMarketID: "ah",
		Selections: []canon.Selection{
			{Side: canon.SideHome, Price: canon.NewPrice(1.9, canon.FormDecimal, time.Now().UTC())},
			{Side: canon.SideAway, Price: canon.NewPrice(1.95, canon.FormDecimal, time.Now().UTC())},
		},
	}
	published, err := r.ResolveAndPublish(t.Context(), source, time.Now().UTC(), []canon.RawMarket{market}, nil)
	require.NoError(t, err)
	assert.Zero(t, published)

	var reviewID string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT id::text FROM review_queue WHERE source = $1 AND kind = 'team' AND "sourceKey" = 'Unrecognisable Name'`, source).Scan(&reviewID))
	_, err = reviews.Decide(t.Context(), review.Decision{ReviewID: reviewID, Action: review.ActionLink, TargetID: homeID})
	require.NoError(t, err)

	var delta canon.Delta
	require.Eventually(t, func() bool {
		entries, err := rdb.XRange(t.Context(), redis.StreamOddsDelta, "("+afterID, "+").Result()
		if err != nil {
			return false
		}
		for _, e := range entries {
			if raw, isDelta := e.Values[redis.StreamField].(string); isDelta && json.Unmarshal([]byte(raw), &delta) == nil && delta.Source == source {
				return true
			}
		}
		return false
	}, reprocessWait, 100*time.Millisecond)
	assert.Equal(t, canon.CodeAH, delta.Market.Key.Code)
	assert.Equal(t, -0.5, *delta.Market.Key.Line)

	var link EventLink
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "eventId"::text, flipped FROM event_links WHERE source = $1 AND "sourceEventId" = $2`, source, sourceEvent.SourceID).Scan(&link.EventID, &link.Flipped))
	assert.Equal(t, link.EventID, delta.Market.Key.EventID)
	held, err := r.pending.heldEvents(t.Context(), source)
	require.NoError(t, err)
	assert.Empty(t, held)
}

func TestFlippedSourceEventLinksToExistingEvent(t *testing.T) {
	r, _, pool, rdb := openIntegrationResolver(t)
	suffix := uuid.NewString()[:8]
	source := "resolve-it-" + suffix
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	homeName, awayName := "Plumstead Wanderers "+suffix, "Grimsditch Athletic "+suffix
	homeID := insertTeam(t, pool, "EPL", homeName)
	awayID := insertTeam(t, pool, "EPL", awayName)
	t.Cleanup(func() { cleanupResolveRows(pool, rdb, source, []string{homeID, awayID}) })
	kickoff := time.Now().UTC().Add(91 * 24 * time.Hour).Truncate(time.Minute)
	var eventID string
	require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3) RETURNING id::text`, homeID, awayID, kickoff).Scan(&eventID))
	require.NoError(t, r.Load(t.Context()))

	flipped := canon.SourceEvent{SourceID: "flip-" + suffix, LeagueHint: "EPL", Home: awayName, Away: homeName, Kickoff: kickoff.Add(10 * time.Minute)}
	for range 2 {
		linked, err := r.LinkEvents(t.Context(), source, []canon.SourceEvent{flipped})
		require.NoError(t, err)
		assert.Equal(t, []string{flipped.SourceID}, linked)
	}
	link, ok := r.cache.eventLink(source, flipped.SourceID)
	require.True(t, ok)
	assert.Equal(t, EventLink{EventID: eventID, Flipped: true}, link)

	var events int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE "homeTeamId" = ANY($1::uuid[])`, []string{homeID, awayID}).Scan(&events))
	assert.Equal(t, 1, events, "a flipped source event must not create a second event")
}

func TestUnchangedLinkedEventsPublishOneFreshnessMarkEach(t *testing.T) {
	r, _, pool, rdb := openIntegrationResolver(t)
	suffix := uuid.NewString()[:8]
	source := "resolve-it-" + suffix
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	homeName, awayName := "Freshby United "+suffix, "Stalewick Town "+suffix
	homeID := insertTeam(t, pool, "EPL", homeName)
	awayID := insertTeam(t, pool, "EPL", awayName)
	t.Cleanup(func() { cleanupResolveRows(pool, rdb, source, []string{homeID, awayID}) })
	require.NoError(t, r.Load(t.Context()))
	sourceEvent := canon.SourceEvent{SourceID: "fresh-" + suffix, LeagueHint: "EPL", Home: homeName, Away: awayName, Kickoff: time.Now().UTC().Add(92 * 24 * time.Hour).Truncate(time.Minute)}
	linked, err := r.LinkEvents(t.Context(), source, []canon.SourceEvent{sourceEvent})
	require.NoError(t, err)
	require.Equal(t, []string{sourceEvent.SourceID}, linked)
	link, _ := r.cache.eventLink(source, sourceEvent.SourceID)

	afterID := "0-0"
	if last, err := rdb.XRevRangeN(t.Context(), redis.StreamOddsDelta, "+", "-", 1).Result(); err == nil && len(last) > 0 {
		afterID = last[0].ID
	}
	fetchedAt := time.Now().UTC().Truncate(time.Millisecond)
	published, err := r.ResolveAndPublish(t.Context(), source, fetchedAt, nil, []PolledEvent{
		{SourceEventID: sourceEvent.SourceID, SourceKeys: []string{"m1"}, Complete: true},
		{SourceEventID: sourceEvent.SourceID, SourceKeys: []string{"m2"}, Complete: true},
		{SourceEventID: "never-linked-" + suffix, SourceKeys: []string{"m3"}, Complete: true},
	})
	require.NoError(t, err)
	assert.Zero(t, published)

	entries, err := rdb.XRange(t.Context(), redis.StreamOddsDelta, "("+afterID, "+").Result()
	require.NoError(t, err)
	var marks []canon.Freshness
	for _, e := range entries {
		body, ok := e.Values[redis.FreshnessField].(string)
		if !ok {
			continue
		}
		var f canon.Freshness
		require.NoError(t, json.Unmarshal([]byte(body), &f))
		if f.Source == source {
			marks = append(marks, f)
		}
	}
	require.Len(t, marks, 1)
	assert.Equal(t, canon.Freshness{Source: source, EventID: link.EventID, ObservedAt: fetchedAt, SourceKeys: []string{"m1", "m2"}, Complete: true}, marks[0])
}

func TestFreshAutoLinkReplacesFuzzyButNotExactAlias(t *testing.T) {
	r, _, pool, _ := openIntegrationResolver(t)
	source := "resolve-it-" + uuid.NewString()[:8]
	var players [3]string
	for i := range players {
		name := "Alias Strength " + source + strconv.Itoa(i)
		require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO players (name, "normalizedName") VALUES ($1, lower($1)) RETURNING id::text`, name).Scan(&players[i]))
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM player_aliases WHERE source = $1`, source)
		_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id = ANY($1::uuid[])`, players[:])
	})
	steps := []struct {
		playerID   string
		confidence float64
		expected   string
	}{
		{playerID: players[0], confidence: 0.95, expected: players[0]},
		{playerID: players[2], confidence: 0.93, expected: players[2]},
		{playerID: players[1], confidence: 1, expected: players[1]},
		{playerID: players[0], confidence: 0.95, expected: players[1]},
		{playerID: players[1], confidence: 1, expected: players[1]},
	}
	for _, step := range steps {
		stored, err := r.store.linkPlayerAlias(t.Context(), source, "sr:player:1", step.playerID, step.confidence)
		require.NoError(t, err)
		assert.Equal(t, step.expected, stored)
	}
}
