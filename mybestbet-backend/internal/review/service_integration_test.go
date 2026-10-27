package review

import (
	"context"
	"os"
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
	"mybestbet/pkg/canon"
)

const aliasUpdateWait = 5 * time.Second

func openIntegrationService(t *testing.T) (*Service, *pgxpool.Pool, *goredis.Client) {
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
	return NewService(pool, rdb), pool, rdb
}

func ensureLeagueTeam(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	var id string
	require.NoError(t, pool.QueryRow(t.Context(), `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, lower($1)) RETURNING id::text`, name).Scan(&id))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM team_aliases WHERE "teamId" = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM teams WHERE id = $1`, id)
	})
	return id
}

func pendingItemID(t *testing.T, s *Service, pool *pgxpool.Pool, it NewItem) string {
	t.Helper()
	require.NoError(t, s.Enqueue(t.Context(), it))
	require.NoError(t, s.Enqueue(t.Context(), it), "enqueue is idempotent")
	var id string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT id::text FROM review_queue WHERE kind = $1 AND source = $2 AND "sourceKey" = $3`, it.Kind, it.Source, it.SourceKey).Scan(&id))
	return id
}

func cleanupSource(pool *pgxpool.Pool, source string) {
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM team_aliases WHERE source = $1`, source)
	_, _ = pool.Exec(ctx, `DELETE FROM review_queue WHERE source = $1`, source)
}

func TestDecideLinkIsIdempotentAndPublishesAlias(t *testing.T) {
	s, pool, rdb := openIntegrationService(t)
	source := "review-it-" + uuid.NewString()[:8]
	t.Cleanup(func() { cleanupSource(pool, source) })
	teamID := ensureLeagueTeam(t, pool, "Review Target "+source)
	id := pendingItemID(t, s, pool, NewItem{
		Kind: canon.AliasTeam, Source: source, SourceKey: "Revw Target",
		Payload:    TeamPayload{League: "EPL", Name: "Revw Target"},
		Candidates: []Candidate{{ID: teamID, Name: "Review Target", Score: 0.88}},
	})

	sub := rdb.Subscribe(t.Context(), redis.ChannelAliasUpdate)
	_, err := sub.Receive(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Close() })

	pending, err := s.List(t.Context(), StatusPending, maxListLimit)
	require.NoError(t, err)
	for _, it := range pending {
		assert.Equal(t, StatusPending, it.Status)
	}
	if len(pending) < maxListLimit {
		assert.Contains(t, itemIDs(pending), id)
	}

	link := Decision{ReviewID: id, Action: ActionLink, TargetID: teamID}
	first, err := s.Decide(t.Context(), link)
	require.NoError(t, err)
	assert.Equal(t, StatusLinked, first.Status)
	require.Len(t, first.Candidates, 1)

	assert.Equal(t, canon.AliasUpdate{Kind: canon.AliasTeam, Source: source, SourceID: "Revw Target", TargetID: teamID}, awaitAliasUpdateFrom(t, sub.Channel(), source))

	var aliasTeam string
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT "teamId"::text FROM team_aliases WHERE source = $1 AND "sourceName" = 'Revw Target'`, source).Scan(&aliasTeam))
	assert.Equal(t, teamID, aliasTeam)

	again, err := s.Decide(t.Context(), link)
	require.NoError(t, err)
	assert.Equal(t, first, again)

	_, err = s.Decide(t.Context(), Decision{ReviewID: id, Action: ActionIgnore})
	assert.ErrorIs(t, err, ErrAlreadyDecided)
	_, err = s.Decide(t.Context(), Decision{ReviewID: id, Action: ActionLink, TargetID: uuid.NewString()})
	assert.ErrorIs(t, err, ErrAlreadyDecided)
}

func TestDecideCreateTeamInsertsEntity(t *testing.T) {
	s, pool, _ := openIntegrationService(t)
	source := "review-it-" + uuid.NewString()[:8]
	name := "Brand New " + source
	t.Cleanup(func() {
		cleanupSource(pool, source)
		_, _ = pool.Exec(context.Background(), `DELETE FROM teams WHERE name = $1`, name)
	})
	_, err := pool.Exec(t.Context(), `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	id := pendingItemID(t, s, pool, NewItem{Kind: canon.AliasTeam, Source: source, SourceKey: name, Payload: TeamPayload{League: "EPL", Name: name}})

	created, err := s.Decide(t.Context(), Decision{ReviewID: id, Action: ActionCreate})
	require.NoError(t, err)
	assert.Equal(t, StatusCreated, created.Status)
	_, err = s.Decide(t.Context(), Decision{ReviewID: id, Action: ActionCreate})
	require.NoError(t, err)

	var teams int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM teams WHERE name = $1 AND "leagueCode" = 'EPL'`, name).Scan(&teams))
	assert.Equal(t, 1, teams)
}

func TestDecideRejectsInvalidInput(t *testing.T) {
	s, pool, _ := openIntegrationService(t)
	source := "review-it-" + uuid.NewString()[:8]
	t.Cleanup(func() { cleanupSource(pool, source) })
	id := pendingItemID(t, s, pool, NewItem{Kind: canon.AliasTeam, Source: source, SourceKey: "x", Payload: TeamPayload{League: "EPL", Name: "x"}})
	tests := []struct {
		name     string
		decision Decision
		expected error
	}{
		{name: "unknown action", decision: Decision{ReviewID: id, Action: "merge"}, expected: ErrInvalidDecision},
		{name: "review id not uuid", decision: Decision{ReviewID: "nope", Action: ActionIgnore}, expected: ErrInvalidDecision},
		{name: "link without target", decision: Decision{ReviewID: id, Action: ActionLink}, expected: ErrInvalidDecision},
		{name: "link to missing team", decision: Decision{ReviewID: id, Action: ActionLink, TargetID: uuid.NewString()}, expected: ErrInvalidDecision},
		{name: "missing review", decision: Decision{ReviewID: uuid.NewString(), Action: ActionIgnore}, expected: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Decide(t.Context(), tt.decision)
			assert.ErrorIs(t, err, tt.expected)
		})
	}
}

func itemIDs(items []Item) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	return ids
}

func awaitAliasUpdateFrom(t *testing.T, updates <-chan *goredis.Message, source string) canon.AliasUpdate {
	t.Helper()
	deadline := time.After(aliasUpdateWait)
	for {
		select {
		case msg := <-updates:
			var u canon.AliasUpdate
			require.NoError(t, json.Unmarshal([]byte(msg.Payload), &u))
			if u.Source == source {
				return u
			}
		case <-deadline:
			t.Fatal("alias update not published")
			return canon.AliasUpdate{}
		}
	}
}
