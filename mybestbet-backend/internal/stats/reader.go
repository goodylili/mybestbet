package stats

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/pkg/canon"
)

const (
	minutesTrendMatches       = 10
	ratingDistributionMatches = 20
	vsOpponentMatches         = 20
	maxPlayerStatsMatches     = 100
)

const matchStatColumns = `"playerId", "eventId",
	COALESCE((SELECT l.sport FROM events e JOIN leagues l ON l.code = e."leagueCode" WHERE e.id = player_match_stats."eventId"), ''),
	COALESCE("teamId"::text, ''), COALESCE("opponentTeamId"::text, ''), "isHome", "playedAt",
	minutes, started, goals, assists, shots, "shotsOnTarget", fouls, tackles, yellow, red, rating, stats, "valueSources"`

type Reader struct {
	pool *pgxpool.Pool
}

func NewReader(pool *pgxpool.Pool) *Reader { return &Reader{pool: pool} }

func (r *Reader) PlayerStats(ctx context.Context, playerID string, last int) ([]MatchStat, error) {
	if err := validateIDs(playerID); err != nil {
		return nil, err
	}
	limit := min(max(last, 1), maxPlayerStatsMatches)
	return r.queryMatchStats(ctx, `SELECT `+matchStatColumns+` FROM player_match_stats
		WHERE "playerId" = $1 ORDER BY "playedAt" DESC LIMIT $2`, playerID, limit)
}

func (r *Reader) Histories(ctx context.Context, playerIDs []string) (map[string]History, error) {
	if err := validateIDs(playerIDs...); err != nil {
		return nil, err
	}
	matches, err := r.queryMatchStats(ctx, `SELECT `+matchStatColumns+` FROM (
			SELECT *, row_number() OVER (PARTITION BY "playerId" ORDER BY "playedAt" DESC) AS recency
			FROM player_match_stats WHERE "playerId" = ANY($1::uuid[])) player_match_stats
		WHERE recency <= $2 ORDER BY "playerId", "playedAt" DESC`, playerIDs, maxHistoryMatches)
	if err != nil {
		return nil, err
	}
	histories := make(map[string]History, len(playerIDs))
	for _, id := range playerIDs {
		histories[id] = History{PlayerID: id}
	}
	for _, m := range matches {
		h := histories[m.PlayerID]
		h.newestFirst = append(h.newestFirst, m)
		histories[m.PlayerID] = h
	}
	return histories, nil
}

func (r *Reader) history(ctx context.Context, playerID string) (History, error) {
	histories, err := r.Histories(ctx, []string{playerID})
	if err != nil {
		return History{}, err
	}
	return histories[playerID], nil
}

func (r *Reader) HitRates(ctx context.Context, playerID string, code canon.MarketCode, line float64, lastN []int) (HitRates, error) {
	if _, _, err := ruleFor(code, line); err != nil {
		return HitRates{}, err
	}
	h, err := r.history(ctx, playerID)
	if err != nil {
		return HitRates{}, err
	}
	return h.HitRates(code, line, lastN)
}

func (r *Reader) VsOpponent(ctx context.Context, playerID, opponentTeamID string, code canon.MarketCode, line float64) (HitRate, []MatchStat, error) {
	if _, _, err := ruleFor(code, line); err != nil {
		return HitRate{}, nil, err
	}
	if err := validateIDs(opponentTeamID); err != nil {
		return HitRate{}, nil, err
	}
	h, err := r.history(ctx, playerID)
	if err != nil {
		return HitRate{}, nil, err
	}
	return h.VsOpponent(opponentTeamID, code, line)
}

func (r *Reader) MinutesTrend(ctx context.Context, playerID string) ([]MinutesPoint, error) {
	h, err := r.history(ctx, playerID)
	if err != nil {
		return nil, err
	}
	return h.MinutesTrend(), nil
}

func (r *Reader) RatingDistribution(ctx context.Context, playerID string) (RatingDistribution, error) {
	h, err := r.history(ctx, playerID)
	if err != nil {
		return RatingDistribution{}, err
	}
	return h.RatingDistribution(), nil
}

func (r *Reader) LineupStatuses(ctx context.Context, eventID string, playerIDs []string) (map[string]LineupStatus, error) {
	if err := validateIDs(append([]string{eventID}, playerIDs...)...); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT "playerId"::text, status FROM lineups WHERE "eventId" = $1 AND "playerId" = ANY($2::uuid[])`, eventID, playerIDs)
	if err != nil {
		return nil, fmt.Errorf("read lineup statuses: %w", err)
	}
	statuses := map[string]LineupStatus{}
	var playerID, status string
	_, err = pgx.ForEachRow(rows, []any{&playerID, &status}, func() error {
		statuses[playerID] = LineupStatus(status)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan lineup statuses: %w", err)
	}
	return statuses, nil
}

func (r *Reader) LineupStatus(ctx context.Context, eventID, playerID string) (LineupStatus, error) {
	statuses, err := r.LineupStatuses(ctx, eventID, []string{playerID})
	if err != nil {
		return LineupUnknown, err
	}
	return statuses[playerID], nil
}

func (r *Reader) SeasonStats(ctx context.Context, playerID string) ([]SeasonStat, error) {
	if err := validateIDs(playerID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT "leagueCode", season, source, team, matches, starts, minutes, goals, assists, shots,
			"shotsOnTarget", fouls, tackles, yellow, red, "goalsPer90", "assistsPer90", "shotsPer90", "shotsOnTargetPer90", "updatedAt"
		FROM player_season_stats WHERE "playerId" = $1 ORDER BY season DESC, "leagueCode"`, playerID)
	if err != nil {
		return nil, fmt.Errorf("query season stats: %w", err)
	}
	seasons, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SeasonStat])
	if err != nil {
		return nil, fmt.Errorf("scan season stats: %w", err)
	}
	return seasons, nil
}

func (r *Reader) queryMatchStats(ctx context.Context, sql string, args ...any) ([]MatchStat, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query player match stats: %w", err)
	}
	matches, err := pgx.CollectRows(rows, scanMatchStat)
	if err != nil {
		return nil, fmt.Errorf("scan player match stats: %w", err)
	}
	return matches, nil
}

func scanMatchStat(row pgx.CollectableRow) (MatchStat, error) {
	var m MatchStat
	err := row.Scan(&m.PlayerID, &m.EventID, &m.Sport, &m.TeamID, &m.OpponentTeamID, &m.IsHome, &m.PlayedAt,
		&m.Minutes, &m.Started, &m.Goals, &m.Assists, &m.Shots, &m.ShotsOnTarget, &m.Fouls, &m.Tackles,
		&m.Yellow, &m.Red, &m.Rating, &m.Stats, &m.ValueSources)
	return m, err
}

func validateIDs(ids ...string) error {
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("%q: %w", id, ErrInvalidID)
		}
	}
	return nil
}
