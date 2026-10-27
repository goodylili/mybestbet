package resolve

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const dedupJobLockKey int64 = 0x6465647570

func (s *store) withJobLock(ctx context.Context, fn func() error) (bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire dedup connection: %w", err)
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, dedupJobLockKey).Scan(&locked); err != nil {
		return false, fmt.Errorf("take dedup lock: %w", err)
	}
	if !locked {
		return false, nil
	}
	defer func() {
		if _, err := conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, dedupJobLockKey); err != nil {
			_ = conn.Conn().Close(context.WithoutCancel(ctx))
		}
	}()
	return true, fn()
}

const stampLegacyMerges = `UPDATE events d SET "mergedAt" = now(),
	"mergedFlipped" = (d."homeTeamId" = s."awayTeamId" OR d."awayTeamId" = s."homeTeamId") AND NOT (d."homeTeamId" = s."homeTeamId" OR d."awayTeamId" = s."awayTeamId")
FROM events s WHERE s.id = d."mergedInto" AND d."mergedAt" IS NULL`

const selectPurgeBatch = `WITH RECURSIVE chain AS (
	SELECT id AS duplicate, "mergedInto" AS target, "mergedFlipped" AS flipped, 1 AS depth
	FROM events WHERE "mergedInto" IS NOT NULL AND "mergedAt" <= $1 AND ($4::uuid[] IS NULL OR id = ANY($4::uuid[]))
	UNION ALL
	SELECT c.duplicate, e."mergedInto", c.flipped <> e."mergedFlipped", c.depth + 1
	FROM chain c JOIN events e ON e.id = c.target
	WHERE e."mergedInto" IS NOT NULL AND c.depth < $3)
SELECT c.duplicate::text, c.target::text, c.flipped
FROM chain c JOIN events s ON s.id = c.target AND s."mergedInto" IS NULL
ORDER BY c.duplicate LIMIT $2`

const purgeBatchRows = `unnest($1::uuid[], $2::uuid[], $3::boolean[]) AS m(duplicate, target, flipped)`

var purgeSteps = []struct {
	name  string
	sql   string
	byID  bool
	tally func(*PurgeReport, int)
}{
	{"flatten merge chains", `UPDATE events e SET "mergedInto" = m.target, "mergedFlipped" = e."mergedFlipped" <> m.flipped
		FROM ` + purgeBatchRows + ` WHERE e."mergedInto" = m.duplicate`, false, nil},
	{"move event sources", `UPDATE event_sources r SET "eventId" = m.target
		FROM ` + purgeBatchRows + ` WHERE r."eventId" = m.duplicate`, false, func(r *PurgeReport, n int) { r.MovedSources += n }},
	{"copy lineups", `INSERT INTO lineups ("eventId", "playerId", status, "updatedAt")
		SELECT m.target, l."playerId", l.status, l."updatedAt" FROM lineups l JOIN ` + purgeBatchRows + ` ON l."eventId" = m.duplicate
		ON CONFLICT DO NOTHING`, false, func(r *PurgeReport, n int) { r.MovedLineups += n }},
	{"delete lineups", `DELETE FROM lineups WHERE "eventId" = ANY($1::uuid[])`, true, nil},
	{"copy player match stats", `INSERT INTO player_match_stats ("playerId", "eventId", "teamId", "opponentTeamId", "isHome", "playedAt", minutes, started,
			goals, assists, shots, "shotsOnTarget", fouls, tackles, yellow, red, rating, "valueSources", "updatedAt", stats)
		SELECT p."playerId", m.target, p."teamId", p."opponentTeamId", p."isHome" <> m.flipped, p."playedAt", p.minutes, p.started,
			p.goals, p.assists, p.shots, p."shotsOnTarget", p.fouls, p.tackles, p.yellow, p.red, p.rating, p."valueSources", p."updatedAt", p.stats
		FROM player_match_stats p JOIN ` + purgeBatchRows + ` ON p."eventId" = m.duplicate
		ON CONFLICT DO NOTHING`, false, func(r *PurgeReport, n int) { r.MovedStats += n }},
	{"delete player match stats", `DELETE FROM player_match_stats WHERE "eventId" = ANY($1::uuid[])`, true, nil},
	{"copy closing prices", `INSERT INTO closing_prices ("marketKey", source, side, label, "eventId", raw, prob, "observedAt")
		SELECT replace(c."marketKey", m.duplicate::text, m.target::text), c.source, c.side, c.label, m.target, c.raw, c.prob, c."observedAt"
		FROM closing_prices c JOIN ` + purgeBatchRows + ` ON c."eventId" = m.duplicate
		WHERE NOT m.flipped
		ON CONFLICT DO NOTHING`, false, func(r *PurgeReport, n int) { r.MovedClosingPrices += n }},
	{"delete closing prices", `DELETE FROM closing_prices WHERE "eventId" = ANY($1::uuid[])`, true, nil},
	{"delete current prices", `DELETE FROM prices_current WHERE "eventId" = ANY($1::uuid[])`, true,
		func(r *PurgeReport, n int) { r.DroppedPrices += n }},
	{"delete price history", `DELETE FROM prices_history WHERE "eventId" = ANY($1::uuid[])`, true,
		func(r *PurgeReport, n int) { r.DroppedHistory += n }},
	{"delete signals", `DELETE FROM signals WHERE "eventId" = ANY($1::uuid[])`, true,
		func(r *PurgeReport, n int) { r.DroppedSignals += n }},
}

const moveEventLinks = `UPDATE event_links l SET "eventId" = m.target, flipped = l.flipped <> m.flipped
FROM ` + purgeBatchRows + ` WHERE l."eventId" = m.duplicate
RETURNING l.source, l."sourceEventId", l."eventId"::text, l.flipped`

const stashInheritedFacts = `CREATE TEMP TABLE purge_inherited ON COMMIT DROP AS
SELECT DISTINCT ON (m.target) m.target, d."sofascoreId", d.score, d."liveUpdatedAt"
FROM ` + purgeBatchRows + ` JOIN events d ON d.id = m.duplicate
WHERE NOT m.flipped AND (d."sofascoreId" IS NOT NULL OR d.score IS NOT NULL)
ORDER BY m.target, d."sofascoreId" NULLS LAST, d."liveUpdatedAt" DESC NULLS LAST`

const deleteMergedEvents = `DELETE FROM events WHERE id = ANY($1::uuid[]) AND "mergedInto" IS NOT NULL`

const inheritFacts = `UPDATE events s SET
	"sofascoreId" = CASE WHEN s."sofascoreId" IS NULL AND NOT EXISTS (SELECT 1 FROM events o WHERE o."sofascoreId" = f."sofascoreId") THEN f."sofascoreId" ELSE s."sofascoreId" END,
	score = COALESCE(s.score, f.score),
	"liveUpdatedAt" = CASE WHEN s.score IS NULL AND f.score IS NOT NULL THEN f."liveUpdatedAt" ELSE s."liveUpdatedAt" END
FROM purge_inherited f WHERE s.id = f.target`

type purgeBatch struct {
	duplicates []string
	targets    []string
	flipped    []bool
}

func (b purgeBatch) args() []any { return []any{b.duplicates, b.targets, b.flipped} }

func (s *store) stampLegacyMerges(ctx context.Context) (int, error) {
	tag, err := s.pool.Exec(ctx, stampLegacyMerges)
	if err != nil {
		return 0, fmt.Errorf("stamp legacy merges: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (m mergeTx) purgeBatch(ctx context.Context, mergedBefore time.Time, limit int, scope []string) (PurgeReport, []EventLinkMove, error) {
	batch, err := m.selectPurgeBatch(ctx, mergedBefore, limit, scope)
	if err != nil || len(batch.duplicates) == 0 {
		return PurgeReport{}, nil, err
	}
	report := PurgeReport{Selected: len(batch.duplicates)}
	args := batch.args()
	for _, step := range purgeSteps {
		stepArgs := args
		if step.byID {
			stepArgs = args[:1]
		}
		tag, err := m.tx.Exec(ctx, step.sql, stepArgs...)
		if err != nil {
			return PurgeReport{}, nil, fmt.Errorf("%s: %w", step.name, err)
		}
		if step.tally != nil {
			step.tally(&report, int(tag.RowsAffected()))
		}
	}
	moves, err := m.moveEventLinks(ctx, args)
	if err != nil {
		return PurgeReport{}, nil, err
	}
	report.MovedLinks = len(moves)
	if _, err := m.tx.Exec(ctx, stashInheritedFacts, args...); err != nil {
		return PurgeReport{}, nil, fmt.Errorf("stash inherited facts: %w", err)
	}
	tag, err := m.tx.Exec(ctx, deleteMergedEvents, batch.duplicates)
	if err != nil {
		return PurgeReport{}, nil, fmt.Errorf("delete merged events: %w", err)
	}
	report.PurgedEvents = int(tag.RowsAffected())
	if _, err := m.tx.Exec(ctx, inheritFacts); err != nil {
		return PurgeReport{}, nil, fmt.Errorf("inherit merged event facts: %w", err)
	}
	return report, moves, nil
}

func (m mergeTx) selectPurgeBatch(ctx context.Context, mergedBefore time.Time, limit int, scope []string) (purgeBatch, error) {
	rows, err := m.tx.Query(ctx, selectPurgeBatch, mergedBefore, limit, maxMergeChainDepth, scope)
	if err != nil {
		return purgeBatch{}, fmt.Errorf("select purge batch: %w", err)
	}
	var (
		b                 purgeBatch
		duplicate, target string
		flipped           bool
	)
	_, err = pgx.ForEachRow(rows, []any{&duplicate, &target, &flipped}, func() error {
		b.duplicates = append(b.duplicates, duplicate)
		b.targets = append(b.targets, target)
		b.flipped = append(b.flipped, flipped)
		return nil
	})
	if err != nil {
		return purgeBatch{}, fmt.Errorf("scan purge batch: %w", err)
	}
	return b, nil
}

func (m mergeTx) moveEventLinks(ctx context.Context, args []any) ([]EventLinkMove, error) {
	rows, err := m.tx.Query(ctx, moveEventLinks, args...)
	if err != nil {
		return nil, fmt.Errorf("move event links: %w", err)
	}
	moves, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (EventLinkMove, error) {
		var mv EventLinkMove
		err := row.Scan(&mv.Source, &mv.SourceEventID, &mv.Link.EventID, &mv.Link.Flipped)
		return mv, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan moved event links: %w", err)
	}
	return moves, nil
}
