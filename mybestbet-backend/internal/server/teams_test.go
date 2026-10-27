package server

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

func TestTeamQueriesAndSourceCoverage(t *testing.T) {
	pool, rdb := integrationDeps(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	source := "coverage-" + suffix
	insertID := func(sql string, args ...any) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, sql, args...).Scan(&id))
		return id
	}
	exec := func(sql string, args ...any) {
		_, err := pool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England'), ('UCL', 'UEFA Champions League', 'Europe') ON CONFLICT DO NOTHING`)
	homeID := insertID(`INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, "Arsenal "+suffix)
	awayID := insertID(`INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, "Chelsea "+suffix)
	exec(`INSERT INTO team_leagues ("teamId", "leagueCode") VALUES ($1, 'UCL') ON CONFLICT DO NOTHING`, homeID)
	eventID := insertID(`INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3) RETURNING id::text`, homeID, awayID, time.Now().Add(time.Hour))
	var boardKeys []string
	insertPrice := func(code canon.MarketCode) {
		marketKey := canon.MarketKey{EventID: eventID, Code: code}.String()
		boardKeys = append(boardKeys, redis.BoardKey(marketKey))
		require.NoError(t, rdb.SAdd(ctx, redis.EventMarketsKey(eventID), marketKey).Err())
		require.NoError(t, rdb.HSet(ctx, redis.BoardKey(marketKey), redis.BoardField(source, string(canon.SideHome), ""), "{}").Err())
	}
	insertPrice(canon.Code1X2)
	t.Cleanup(func() {
		_ = rdb.Del(ctx, append(boardKeys, redis.EventMarketsKey(eventID))...).Err()
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE id = $1`, eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM teams WHERE id = ANY($1::uuid[])`, []string{homeID, awayID})
	})
	s := New(Deps{Config: config.Config{Leagues: []config.League{{Code: "EPL", Sport: canon.SportFootball}}}, Pool: pool, Redis: rdb, Log: slog.Default()})

	t.Run("team detail lists every league", func(t *testing.T) {
		team, err := s.teamByID(ctx, homeID)
		require.NoError(t, err)
		assert.Equal(t, TeamDetail{Team: Team{ID: homeID, Name: "Arsenal " + suffix}, Sport: canon.SportFootball, LeagueCodes: []string{"EPL", "UCL"}}, team)
	})

	t.Run("unknown team is not found", func(t *testing.T) {
		_, err := s.teamByID(ctx, uuid.NewString())
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("opponents are named", func(t *testing.T) {
		matches, err := s.withOpponents(ctx, []PlayerMatchStat{{OpponentTeamID: awayID}, {}})
		require.NoError(t, err)
		assert.Equal(t, "Chelsea "+suffix, matches[0].OpponentTeamName)
		assert.Empty(t, matches[1].OpponentTeamName)
	})

	t.Run("coverage is cached until the ttl lapses", func(t *testing.T) {
		coverage, err := s.sourceCoverage(ctx)
		require.NoError(t, err)
		assert.Empty(t, coverage)
		expected := SourceCoverage{Leagues: []string{"EPL"}, Markets: []canon.MarketCode{canon.Code1X2}}
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			coverage, err := s.sourceCoverage(ctx)
			require.NoError(c, err)
			assert.Equal(c, expected, coverage[source])
		}, 10*time.Second, 50*time.Millisecond)

		insertPrice(canon.CodeBTTS)
		coverage, err = s.sourceCoverage(ctx)
		require.NoError(t, err)
		assert.Equal(t, expected, coverage[source])

		s.coverage.mu.Lock()
		s.coverage.snapshot.loadedAt = time.Now().Add(-coverageTTL)
		s.coverage.mu.Unlock()
		coverage, err = s.sourceCoverage(ctx)
		require.NoError(t, err)
		assert.Equal(t, expected, coverage[source])
		assert.EventuallyWithT(t, func(c *assert.CollectT) {
			coverage, err := s.sourceCoverage(ctx)
			require.NoError(c, err)
			assert.Equal(c, SourceCoverage{Leagues: []string{"EPL"}, Markets: []canon.MarketCode{canon.Code1X2, canon.CodeBTTS}}, coverage[source])
		}, 10*time.Second, 50*time.Millisecond)
	})

	t.Run("event coverage lists sources and open props", func(t *testing.T) {
		coverage, err := s.coverageByEvent(ctx, []string{eventID, uuid.NewString()})
		require.NoError(t, err)
		assert.Equal(t, map[string]eventCoverage{eventID: {sources: []string{source}}}, coverage)
	})
}
