package resolve

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const mergeLockKey int64 = 0x6d65726765

type mergeTx struct {
	tx pgx.Tx
}

func (s *store) inMergeLock(ctx context.Context, fn func(mergeTx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, mergeLockKey); err != nil {
			return fmt.Errorf("take merge lock: %w", err)
		}
		return fn(mergeTx{tx: tx})
	})
}

const selectMergeCandidates = `
WITH team_refs AS (SELECT "teamId", array_agg(DISTINCT source) AS sources FROM team_sources GROUP BY 1),
event_refs AS (
	SELECT "eventId", array_agg(DISTINCT source) AS sources, count(*) FILTER (WHERE link) AS links FROM (
		SELECT "eventId", source, true AS link FROM event_links
		UNION ALL SELECT "eventId", source, false FROM event_sources) refs
	GROUP BY 1)
SELECT e.id::text, e."leagueCode", e.kickoff, e."createdAt",
	h.id::text, h.name, COALESCE(hr.sources, '{}'), a.id::text, a.name, COALESCE(ar.sources, '{}'),
	COALESCE(r.links, 0), COALESCE(r.sources, '{}')
FROM events e
JOIN teams h ON h.id = e."homeTeamId" JOIN teams a ON a.id = e."awayTeamId"
LEFT JOIN team_refs hr ON hr."teamId" = h.id LEFT JOIN team_refs ar ON ar."teamId" = a.id
LEFT JOIN event_refs r ON r."eventId" = e.id
WHERE e.kickoff >= $1 AND e."mergedInto" IS NULL`

func (m mergeTx) unmergedEventsSince(ctx context.Context, since time.Time) ([]mergeEvent, error) {
	rows, err := m.tx.Query(ctx, selectMergeCandidates, since)
	if err != nil {
		return nil, fmt.Errorf("query merge candidates: %w", err)
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (mergeEvent, error) {
		var e mergeEvent
		err := row.Scan(&e.id, &e.league, &e.kickoff, &e.createdAt,
			&e.home.id, &e.home.name, &e.home.sources, &e.away.id, &e.away.name, &e.away.sources,
			&e.links, &e.sources)
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan merge candidates: %w", err)
	}
	return events, nil
}

func (m mergeTx) merge(ctx context.Context, em eventMerge) (int, int, error) {
	if _, err := m.tx.Exec(ctx, `UPDATE events SET "mergedInto" = $2, "mergedAt" = now(),
		"mergedFlipped" = CASE WHEN id = $1 THEN $3::boolean ELSE "mergedFlipped" <> $3::boolean END
		WHERE (id = $1 OR "mergedInto" = $1) AND "mergedInto" IS DISTINCT FROM $2`,
		em.duplicate, em.canonical, em.flipped); err != nil {
		return 0, 0, fmt.Errorf("mark event merged: %w", err)
	}
	moved, err := m.tx.Exec(ctx, `UPDATE event_links SET "eventId" = $2, flipped = flipped <> $3 WHERE "eventId" = $1`,
		em.duplicate, em.canonical, em.flipped)
	if err != nil {
		return 0, 0, fmt.Errorf("move event links: %w", err)
	}
	closed, err := m.tx.Exec(ctx, `UPDATE signals SET status = 'closed', "closedAt" = now() WHERE "eventId" = $1 AND status = 'open'`, em.duplicate)
	if err != nil {
		return 0, 0, fmt.Errorf("close merged signals: %w", err)
	}
	return int(moved.RowsAffected()), int(closed.RowsAffected()), nil
}
