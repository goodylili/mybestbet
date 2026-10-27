package stats

import (
	"context"
	"fmt"
	"time"

	"mybestbet/internal/seed"
	"mybestbet/pkg/canon"
)

func (s *Store) EventsAwaitingLiveScores(ctx context.Context, from, to time.Time, eventSources []string) ([]SourcedEvent, error) {
	return s.querySourcedEvents(ctx, "events awaiting live scores", sourcedEventsFrom+`
		WHERE e.kickoff BETWEEN $2 AND $3 AND e.status IN ('scheduled', 'live')
		ORDER BY e.kickoff, e.id`, eventSources, from, to)
}

func (s *Store) EventsWithoutSourceAwaitingLiveScores(ctx context.Context, from, to time.Time, sport canon.Sport, missingSource string) ([]SourcedEvent, error) {
	return s.querySourcedEvents(ctx, "events without source awaiting live scores", `
		SELECT e.id::text, ''::text, ''::text, e."leagueCode", e.kickoff, e."homeTeamId"::text, e."awayTeamId"::text, home.name, away.name
		FROM events e
		JOIN leagues l ON l.code = e."leagueCode"
		JOIN teams home ON home.id = e."homeTeamId" JOIN teams away ON away.id = e."awayTeamId"
		WHERE l.sport = $1 AND e.kickoff BETWEEN $2 AND $3 AND e.status IN ('scheduled', 'live')
		  AND NOT EXISTS (SELECT 1 FROM event_sources src WHERE src."eventId" = e.id AND src.source = $4)
		ORDER BY e.kickoff, e.id`, string(sport), from, to, missingSource)
}

func (s *Store) SaveLiveScore(ctx context.Context, eventID string, score canon.EventScore, status seed.EventStatus, observedAt time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE events SET score = $2, status = $3, "liveUpdatedAt" = $4
		WHERE id = $1 AND status IN ('scheduled', 'live')
		  AND ("liveUpdatedAt" IS NULL OR "liveUpdatedAt" < $4)
		  AND (score IS DISTINCT FROM $2 OR status IS DISTINCT FROM $3)`, eventID, score, string(status), observedAt)
	if err != nil {
		return false, fmt.Errorf("save live score for event %s: %w", eventID, err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) FinishOverdueEvents(ctx context.Context, now time.Time, maxDurations map[canon.Sport]time.Duration, fallback, quietFor time.Duration) (int64, error) {
	sports := make([]string, 0, len(maxDurations))
	seconds := make([]int64, 0, len(maxDurations))
	for sport, d := range maxDurations {
		sports, seconds = append(sports, string(sport)), append(seconds, int64(d/time.Second))
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE events e SET status = 'finished'
		FROM leagues l LEFT JOIN unnest($2::text[], $3::bigint[]) d(sport, seconds) ON d.sport = l.sport
		WHERE l.code = e."leagueCode" AND e.status IN ('scheduled', 'live')
		  AND e.kickoff < $1::timestamptz - make_interval(secs => coalesce(d.seconds, $4))
		  AND (e."liveUpdatedAt" IS NULL OR e."liveUpdatedAt" < $1::timestamptz - make_interval(secs => $5))`,
		now, sports, seconds, int64(fallback/time.Second), int64(quietFor/time.Second))
	if err != nil {
		return 0, fmt.Errorf("finish overdue events: %w", err)
	}
	return tag.RowsAffected(), nil
}
