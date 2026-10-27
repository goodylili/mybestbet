package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/internal/stats/statkey"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type ScheduledEvent struct {
	ID              string
	SofascoreID     *int64
	LeagueCode      string
	Kickoff         time.Time
	HomeTeamID      string
	AwayTeamID      string
	HomeTeamName    string
	AwayTeamName    string
	HasAnyStatsRows bool
	FotmobID        *string
}

type StatsWindow struct {
	From  time.Time
	To    time.Time
	Limit int
	Skip  []string
}

type NamedPlayer struct {
	ID       string
	Name     string
	TeamName string
}

type LineupRow struct {
	PlayerID string
	Status   LineupStatus
}

const scheduledEventColumns = `e.id, e."sofascoreId", e."leagueCode", e.kickoff, e."homeTeamId", e."awayTeamId", home.name, away.name,
	EXISTS (SELECT 1 FROM player_match_stats s WHERE s."eventId" = e.id),
	(SELECT src."sourceId" FROM event_sources src WHERE src."eventId" = e.id AND src.source = 'fotmob' ORDER BY src."sourceId" LIMIT 1)`

const footballEventsWithTeams = `events e
	JOIN leagues league ON league.code = e."leagueCode" AND league.sport = 'football'
	JOIN teams home ON home.id = e."homeTeamId" JOIN teams away ON away.id = e."awayTeamId"`

func (s *Store) EventsAwaitingLineups(ctx context.Context, from, to time.Time) ([]ScheduledEvent, error) {
	return s.queryScheduledEvents(ctx, `SELECT `+scheduledEventColumns+`
		FROM `+footballEventsWithTeams+`
		WHERE e.kickoff BETWEEN $1 AND $2 AND e.status IN ('scheduled', 'live')
		ORDER BY e.kickoff`, from, to)
}

func (s *Store) EventsAwaitingStats(ctx context.Context, w StatsWindow, untilSofascore bool) ([]ScheduledEvent, error) {
	return s.queryScheduledEvents(ctx, `SELECT `+scheduledEventColumns+`
		FROM `+footballEventsWithTeams+`
		WHERE e.kickoff BETWEEN $1 AND $2 AND e.status NOT IN ('postponed', 'cancelled')
		  AND e.id::text <> ALL($4::text[])
		  AND NOT EXISTS (SELECT 1 FROM player_match_stats s WHERE s."eventId" = e.id AND (NOT $5 OR s."valueSources"->>'minutes' = 'sofascore'))
		ORDER BY e.kickoff LIMIT $3`, w.From, w.To, w.Limit, skipOrEmpty(w.Skip), untilSofascore)
}

func skipOrEmpty(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func (s *Store) HasMatchStats(ctx context.Context, eventID string) (bool, error) {
	var has bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM player_match_stats WHERE "eventId" = $1)`, eventID).Scan(&has); err != nil {
		return false, fmt.Errorf("check event %s stats: %w", eventID, err)
	}
	return has, nil
}

func (s *Store) queryScheduledEvents(ctx context.Context, sql string, args ...any) ([]ScheduledEvent, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query scheduled events: %w", err)
	}
	events, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ScheduledEvent])
	if err != nil {
		return nil, fmt.Errorf("scan scheduled events: %w", err)
	}
	return events, nil
}

func (s *Store) SetEventStatus(ctx context.Context, eventID, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE events SET status = $2 WHERE id = $1 AND status IS DISTINCT FROM $2`, eventID, status)
	if err != nil {
		return fmt.Errorf("set event %s status: %w", eventID, err)
	}
	return nil
}

func (s *Store) PlayerIDsBySofascore(ctx context.Context, sofascoreIDs []int64) (map[int64]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT "sofascoreId", id FROM players WHERE "sofascoreId" = ANY($1)`, sofascoreIDs)
	if err != nil {
		return nil, fmt.Errorf("query players by sofascore id: %w", err)
	}
	ids := make(map[int64]string, len(sofascoreIDs))
	var sofascoreID int64
	var playerID string
	_, err = pgx.ForEachRow(rows, []any{&sofascoreID, &playerID}, func() error {
		ids[sofascoreID] = playerID
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan players by sofascore id: %w", err)
	}
	return ids, nil
}

func (s *Store) TeamPlayers(ctx context.Context, teamID string) ([]NamedPlayer, error) {
	return s.queryNamedPlayers(ctx, `SELECT p.id, p.name, t.name FROM players p JOIN teams t ON t.id = p."teamId" WHERE p."teamId" = $1`, teamID)
}

func (s *Store) LeaguePlayers(ctx context.Context, leagueCode string) ([]NamedPlayer, error) {
	return s.queryNamedPlayers(ctx, `SELECT p.id, p.name, t.name FROM players p JOIN teams t ON t.id = p."teamId" WHERE t."leagueCode" = $1`, leagueCode)
}

func (s *Store) queryNamedPlayers(ctx context.Context, sql string, args ...any) ([]NamedPlayer, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query players: %w", err)
	}
	players, err := pgx.CollectRows(rows, pgx.RowToStructByPos[NamedPlayer])
	if err != nil {
		return nil, fmt.Errorf("scan players: %w", err)
	}
	return players, nil
}

func (s *Store) PlayerAliases(ctx context.Context, source string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT "sourceId", "playerId" FROM player_aliases WHERE source = $1`, source)
	if err != nil {
		return nil, fmt.Errorf("query %s player aliases: %w", source, err)
	}
	aliases := map[string]string{}
	var sourceID, playerID string
	_, err = pgx.ForEachRow(rows, []any{&sourceID, &playerID}, func() error {
		aliases[sourceID] = playerID
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s player aliases: %w", source, err)
	}
	return aliases, nil
}

func (s *Store) UpsertMatchStats(ctx context.Context, stats []MatchStat) error {
	batch := &pgx.Batch{}
	for _, m := range stats {
		batch.Queue(`
			INSERT INTO player_match_stats ("playerId", "eventId", "teamId", "opponentTeamId", "isHome", "playedAt", minutes, started,
				goals, assists, shots, "shotsOnTarget", fouls, tackles, yellow, red, rating, stats, "valueSources", "updatedAt")
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, now())
			ON CONFLICT ("playerId", "eventId") DO UPDATE SET
				"teamId" = EXCLUDED."teamId", "opponentTeamId" = EXCLUDED."opponentTeamId", "isHome" = EXCLUDED."isHome",
				"playedAt" = EXCLUDED."playedAt", minutes = EXCLUDED.minutes, started = EXCLUDED.started, goals = EXCLUDED.goals,
				assists = EXCLUDED.assists, shots = EXCLUDED.shots, "shotsOnTarget" = EXCLUDED."shotsOnTarget", fouls = EXCLUDED.fouls,
				tackles = EXCLUDED.tackles, yellow = EXCLUDED.yellow, red = EXCLUDED.red, rating = EXCLUDED.rating,
				stats = EXCLUDED.stats, "valueSources" = EXCLUDED."valueSources", "updatedAt" = now()
			WHERE player_match_stats."valueSources"->>'minutes' IS DISTINCT FROM 'sofascore'
			   OR EXCLUDED."valueSources"->>'minutes' = 'sofascore'`,
			m.PlayerID, m.EventID, nullableID(m.TeamID), nullableID(m.OpponentTeamID), m.IsHome, m.PlayedAt, m.Minutes, m.Started,
			m.Goals, m.Assists, m.Shots, m.ShotsOnTarget, m.Fouls, m.Tackles, m.Yellow, m.Red, m.Rating, statsOrEmpty(m.Stats), m.ValueSources)
	}
	return s.sendBatch(ctx, batch, "upsert player match stats")
}

func (s *Store) UpsertLineup(ctx context.Context, eventID string, rows []LineupRow) error {
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO lineups ("eventId", "playerId", status, "updatedAt") VALUES ($1, $2, $3, now())
			ON CONFLICT ("eventId", "playerId") DO UPDATE SET status = EXCLUDED.status, "updatedAt" = now()
			WHERE lineups.status IS DISTINCT FROM EXCLUDED.status`,
			eventID, r.PlayerID, string(r.Status))
	}
	return s.sendBatch(ctx, batch, "upsert lineup")
}

func (s *Store) UpsertSeasonStat(ctx context.Context, playerID, sourcePlayerID string, st SeasonStat) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO player_season_stats ("playerId", "leagueCode", season, source, "sourcePlayerId", team, matches, starts, minutes,
			goals, assists, shots, "shotsOnTarget", fouls, tackles, yellow, red, "goalsPer90", "assistsPer90", "shotsPer90",
			"shotsOnTargetPer90", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, now())
		ON CONFLICT ("playerId", "leagueCode", season, source) DO UPDATE SET
			"sourcePlayerId" = EXCLUDED."sourcePlayerId", team = EXCLUDED.team, matches = EXCLUDED.matches, starts = EXCLUDED.starts,
			minutes = EXCLUDED.minutes, goals = EXCLUDED.goals, assists = EXCLUDED.assists, shots = EXCLUDED.shots,
			"shotsOnTarget" = EXCLUDED."shotsOnTarget", fouls = EXCLUDED.fouls, tackles = EXCLUDED.tackles, yellow = EXCLUDED.yellow,
			red = EXCLUDED.red, "goalsPer90" = EXCLUDED."goalsPer90", "assistsPer90" = EXCLUDED."assistsPer90",
			"shotsPer90" = EXCLUDED."shotsPer90", "shotsOnTargetPer90" = EXCLUDED."shotsOnTargetPer90", "updatedAt" = now()`,
		playerID, st.LeagueCode, st.Season, st.Source, sourcePlayerID, st.Team, st.Matches, st.Starts, st.Minutes,
		st.Goals, st.Assists, st.Shots, st.ShotsOnTarget, st.Fouls, st.Tackles, st.Yellow, st.Red,
		st.GoalsPer90, st.AssistsPer90, st.ShotsPer90, st.ShotsOnTargetPer90)
	if err != nil {
		return fmt.Errorf("upsert season stat %s %s: %w", st.LeagueCode, st.Season, err)
	}
	return nil
}

func (s *Store) LinkPlayerAlias(ctx context.Context, source, sourceID, playerID string, confidence float64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO player_aliases (source, "sourceId", "playerId", confidence) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source, "sourceId") DO NOTHING`, source, sourceID, playerID, confidence)
	if err != nil {
		return fmt.Errorf("link %s player alias: %w", source, err)
	}
	return nil
}

func (s *Store) sendBatch(ctx context.Context, batch *pgx.Batch, op string) error {
	if batch.Len() == 0 {
		return nil
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return tx.SendBatch(ctx, batch).Close()
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func statsOrEmpty(values statkey.Values) statkey.Values {
	if values == nil {
		return statkey.Values{}
	}
	return values
}

func nullableID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

func (s *Store) EventMatchStats(ctx context.Context, eventID string) ([]MatchStat, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+matchStatColumns+` FROM player_match_stats WHERE "eventId" = $1`, eventID)
	if err != nil {
		return nil, fmt.Errorf("query event match stats: %w", err)
	}
	stats, err := pgx.CollectRows(rows, scanMatchStat)
	if err != nil {
		return nil, fmt.Errorf("scan event match stats: %w", err)
	}
	return stats, nil
}
