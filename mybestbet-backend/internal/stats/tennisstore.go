package stats

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"mybestbet/internal/stats/flashscore"
	"mybestbet/pkg/canon"
)

func (s *Store) SourceEventsWithStats(ctx context.Context, source string, sourceIDs []string) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT src."sourceId" FROM event_sources src
		WHERE src.source = $1 AND src."sourceId" = ANY($2)
		  AND EXISTS (SELECT 1 FROM player_match_stats s WHERE s."eventId" = src."eventId")`, source, sourceIDs)
	if err != nil {
		return nil, fmt.Errorf("query %s events with stats: %w", source, err)
	}
	withStats := make(map[string]bool, len(sourceIDs))
	var sourceID string
	_, err = pgx.ForEachRow(rows, []any{&sourceID}, func() error {
		withStats[sourceID] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s events with stats: %w", source, err)
	}
	return withStats, nil
}

func (s *Store) EnsureTennisLeague(ctx context.Context, t flashscore.Tournament) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO leagues (code, name, country, sport) VALUES ($1, $2, $3, $4) ON CONFLICT (code) DO NOTHING`,
		t.Code, t.Name, t.Country, string(canon.SportTennis))
	if err != nil {
		return fmt.Errorf("ensure tennis league %s: %w", t.Code, err)
	}
	return nil
}
