package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type SourcedEvent struct {
	ID           string
	Source       string
	SourceID     string
	LeagueCode   string
	Kickoff      time.Time
	HomeTeamID   string
	AwayTeamID   string
	HomeTeamName string
	AwayTeamName string
}

func (s *Store) EventsAwaitingSportStats(ctx context.Context, w StatsWindow, leagueCodes, eventSources []string) ([]SourcedEvent, error) {
	return s.querySourcedEvents(ctx, "events awaiting sport stats", sourcedEventsFrom+`
		WHERE e.kickoff BETWEEN $2 AND $3 AND e."leagueCode" = ANY($4) AND e.status NOT IN ('postponed', 'cancelled')
		  AND e.id::text <> ALL($6::text[])
		  AND NOT EXISTS (SELECT 1 FROM player_match_stats s WHERE s."eventId" = e.id)
		ORDER BY e.kickoff LIMIT $5`, eventSources, w.From, w.To, leagueCodes, w.Limit, skipOrEmpty(w.Skip))
}

type SourcedTeam struct {
	TeamID     string
	SourceID   string
	LeagueCode string
}

func (s *Store) PropTeams(ctx context.Context, source string, from, to time.Time) ([]SourcedTeam, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ts."teamId"::text, ts."sourceId", e."leagueCode"
		FROM events e
		JOIN team_sources ts ON ts.source = $1 AND ts."teamId" IN (e."homeTeamId", e."awayTeamId")
		WHERE e.kickoff BETWEEN $2 AND $3 AND e.status IN ('scheduled', 'live')
		  AND EXISTS (SELECT 1 FROM prices_current pc WHERE pc."eventId" = e.id AND pc."playerId" IS NOT NULL)
		GROUP BY ts."teamId", ts."sourceId", e."leagueCode"
		ORDER BY min(e.kickoff)`, source, from, to)
	if err != nil {
		return nil, fmt.Errorf("query %s prop teams: %w", source, err)
	}
	teams, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SourcedTeam])
	if err != nil {
		return nil, fmt.Errorf("scan %s prop teams: %w", source, err)
	}
	return teams, nil
}

const sourcedEventsFrom = `
	SELECT e.id::text, src.source, src."sourceId", e."leagueCode", e.kickoff, e."homeTeamId"::text, e."awayTeamId"::text, home.name, away.name
	FROM events e
	JOIN event_sources src ON src."eventId" = e.id AND src.source = ANY($1)
	JOIN teams home ON home.id = e."homeTeamId" JOIN teams away ON away.id = e."awayTeamId"`

func (s *Store) querySourcedEvents(ctx context.Context, what, sql string, args ...any) ([]SourcedEvent, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", what, err)
	}
	events, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SourcedEvent])
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", what, err)
	}
	for i := range events {
		events[i].Kickoff = events[i].Kickoff.UTC()
	}
	return events, nil
}

func (s *Store) PlayerIDsBySourceID(ctx context.Context, source string, sourceIDs []string) (map[string]string, error) {
	return s.idsBySourceID(ctx, `SELECT "sourceId", "playerId"::text FROM player_aliases WHERE source = $1 AND "sourceId" = ANY($2)`, source, sourceIDs)
}

func (s *Store) TeamIDsBySourceID(ctx context.Context, source string, sourceIDs []string) (map[string]string, error) {
	return s.idsBySourceID(ctx, `SELECT "sourceId", "teamId"::text FROM team_sources WHERE source = $1 AND "sourceId" = ANY($2)`, source, sourceIDs)
}

func (s *Store) idsBySourceID(ctx context.Context, sql, source string, sourceIDs []string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, sql, source, sourceIDs)
	if err != nil {
		return nil, fmt.Errorf("query %s ids: %w", source, err)
	}
	ids := make(map[string]string, len(sourceIDs))
	var sourceID, id string
	_, err = pgx.ForEachRow(rows, []any{&sourceID, &id}, func() error {
		ids[sourceID] = id
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s ids: %w", source, err)
	}
	return ids, nil
}
