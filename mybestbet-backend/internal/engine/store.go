package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/internal/signal"
	"mybestbet/pkg/canon"
)

type store struct {
	pool *pgxpool.Pool
}

const createPriceBatchTable = `CREATE TEMP TABLE IF NOT EXISTS engine_price_batch (
	"marketKey" text, source text, side text, label text, "eventId" text, code text, line double precision,
	"playerId" text, period text, raw double precision, form text, prob double precision, bid double precision,
	ask double precision, liquidity double precision, suspended boolean, "deepLink" text, "regulationOnly" boolean, "voidIfNoStart" boolean,
	"observedAt" timestamptz, "rawChanged" boolean, "sourceMarketKey" text
) ON COMMIT DELETE ROWS`

var priceBatchColumns = []string{"marketKey", "source", "side", "label", "eventId", "code", "line", "playerId", "period", "raw", "form", "prob", "bid", "ask", "liquidity", "suspended", "deepLink", "regulationOnly", "voidIfNoStart", "observedAt", "rawChanged", "sourceMarketKey"}

const unchangedRefreshAfter = `interval '10 minutes'`

const upsertCurrentPrices = `INSERT INTO prices_current ("marketKey", source, side, label, "eventId", code, line, "playerId", period, raw, form, prob, bid, ask, liquidity, suspended, "deepLink", "regulationOnly", "voidIfNoStart", "observedAt", "sourceMarketKey")
SELECT DISTINCT ON (b."marketKey", b.source, b.side, b.label)
	b."marketKey", b.source, b.side, b.label, e.id, b.code, b.line, NULLIF(b."playerId", '')::uuid, b.period, b.raw, b.form, b.prob, b.bid, b.ask, b.liquidity, b.suspended, b."deepLink", b."regulationOnly", b."voidIfNoStart", b."observedAt", b."sourceMarketKey"
FROM engine_price_batch b
JOIN events e ON e.id = b."eventId"::uuid
WHERE NULLIF(b."playerId", '') IS NULL OR EXISTS (SELECT 1 FROM players p WHERE p.id = NULLIF(b."playerId", '')::uuid)
ORDER BY b."marketKey", b.source, b.side, b.label, b."observedAt" DESC
ON CONFLICT ("marketKey", source, side, label) DO UPDATE SET
	raw = EXCLUDED.raw, form = EXCLUDED.form, prob = EXCLUDED.prob, bid = EXCLUDED.bid, ask = EXCLUDED.ask,
	liquidity = EXCLUDED.liquidity, suspended = EXCLUDED.suspended, "deepLink" = EXCLUDED."deepLink",
	"regulationOnly" = EXCLUDED."regulationOnly", "voidIfNoStart" = EXCLUDED."voidIfNoStart", "observedAt" = EXCLUDED."observedAt",
	"sourceMarketKey" = EXCLUDED."sourceMarketKey"
WHERE prices_current."observedAt" <= EXCLUDED."observedAt" AND (
	(prices_current.raw, prices_current.form, prices_current.prob, prices_current.bid, prices_current.ask, prices_current.liquidity, prices_current.suspended, prices_current."deepLink", prices_current."regulationOnly", prices_current."voidIfNoStart", prices_current."sourceMarketKey")
	IS DISTINCT FROM (EXCLUDED.raw, EXCLUDED.form, EXCLUDED.prob, EXCLUDED.bid, EXCLUDED.ask, EXCLUDED.liquidity, EXCLUDED.suspended, EXCLUDED."deepLink", EXCLUDED."regulationOnly", EXCLUDED."voidIfNoStart", EXCLUDED."sourceMarketKey")
	OR prices_current."observedAt" < EXCLUDED."observedAt" - ` + unchangedRefreshAfter + `)`

const insertPriceHistory = `INSERT INTO prices_history ("marketKey", source, side, label, "eventId", raw, form, prob, suspended, "observedAt")
SELECT "marketKey", source, side, label, "eventId"::uuid, raw, form, prob, suspended, "observedAt"
FROM engine_price_batch WHERE "rawChanged"
ON CONFLICT DO NOTHING`

const upsertClosingPrices = `INSERT INTO closing_prices ("marketKey", source, side, label, "eventId", raw, prob, "observedAt")
SELECT DISTINCT ON (b."marketKey", b.source, b.side, b.label) b."marketKey", b.source, b.side, b.label, e.id, b.raw, b.prob, b."observedAt"
FROM engine_price_batch b
JOIN events e ON e.id = b."eventId"::uuid
WHERE b."observedAt" < e.kickoff AND NOT b.suspended AND b.form <> 'line_only'
ORDER BY b."marketKey", b.source, b.side, b.label, b."observedAt" DESC
ON CONFLICT ("marketKey", source, side, label) DO UPDATE SET raw = EXCLUDED.raw, prob = EXCLUDED.prob, "observedAt" = EXCLUDED."observedAt"
WHERE closing_prices."observedAt" <= EXCLUDED."observedAt" AND (
	(closing_prices.raw, closing_prices.prob) IS DISTINCT FROM (EXCLUDED.raw, EXCLUDED.prob)
	OR closing_prices."observedAt" < EXCLUDED."observedAt" - ` + unchangedRefreshAfter + `)`

func (s *store) writePrices(ctx context.Context, rows []priceRow) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, createPriceBatchTable); err != nil {
			return fmt.Errorf("create price batch table: %w", err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"engine_price_batch"}, priceBatchColumns, pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			return rows[i].copyValues(), nil
		})); err != nil {
			return fmt.Errorf("copy price batch: %w", err)
		}
		for _, stmt := range []struct{ name, sql string }{{"prices_current", upsertCurrentPrices}, {"prices_history", insertPriceHistory}, {"closing_prices", upsertClosingPrices}} {
			if _, err := tx.Exec(ctx, stmt.sql); err != nil {
				return fmt.Errorf("write %s: %w", stmt.name, err)
			}
		}
		return nil
	})
}

func (r priceRow) copyValues() []any {
	p := r.cell.Price
	return []any{
		r.marketKey, r.cell.Source, string(r.cell.Side), r.cell.Label, r.eventID, string(r.code), r.line, r.playerID, r.period,
		p.Raw, string(p.Form), p.Prob, p.Bid, p.Ask, p.Liquidity, p.Suspended, r.cell.DeepLink, r.rules.RegulationOnly, r.rules.VoidIfNoStart, p.ObservedAt, r.isRawChanged, r.sourceMarketKey,
	}
}

func (r priceRow) isPersistable() bool {
	if _, err := uuid.Parse(r.eventID); err != nil {
		return false
	}
	if r.playerID == "" {
		return true
	}
	_, err := uuid.Parse(r.playerID)
	return err == nil
}

const ensurePartition = `SELECT ensure_prices_history_partition($1::date)`

func (s *store) ensureHistoryPartitions(ctx context.Context, now time.Time) error {
	today := now.UTC().Truncate(24 * time.Hour)
	for _, day := range []time.Time{today, today.Add(24 * time.Hour)} {
		if _, err := s.pool.Exec(ctx, ensurePartition, day); err != nil {
			return fmt.Errorf("ensure history partition %s: %w", day.Format(time.DateOnly), err)
		}
	}
	return nil
}

const selectCurrentPrices = `SELECT pc."marketKey", pc.source, pc.side, pc.label, pc.raw, pc.form, pc.prob, pc.bid, pc.ask, pc.liquidity,
	pc.suspended, pc."deepLink", pc."regulationOnly", pc."voidIfNoStart", pc."observedAt", pc."sourceMarketKey"
FROM prices_current pc
JOIN events e ON e.id = pc."eventId"
WHERE e.kickoff > $1 AND e.status IN ('scheduled', 'live') AND e."mergedInto" IS NULL
ORDER BY pc."marketKey", pc.source`

func (s *store) loadCurrentPrices(ctx context.Context, kickoffAfter time.Time, restore func(canon.Delta) error) error {
	rows, err := s.pool.Query(ctx, selectCurrentPrices, kickoffAfter)
	if err != nil {
		return fmt.Errorf("query current prices: %w", err)
	}
	defer rows.Close()
	var pending canon.Delta
	var pendingKey string
	flush := func() error {
		if pendingKey == "" {
			return nil
		}
		return restore(pending)
	}
	for rows.Next() {
		var marketKey, source, side, label, form, deepLink, sourceMarketKey string
		var p canon.Price
		var rules canon.SettleRule
		if err := rows.Scan(&marketKey, &source, &side, &label, &p.Raw, &form, &p.Prob, &p.Bid, &p.Ask, &p.Liquidity, &p.Suspended, &deepLink, &rules.RegulationOnly, &rules.VoidIfNoStart, &p.ObservedAt, &sourceMarketKey); err != nil {
			return fmt.Errorf("scan current price: %w", err)
		}
		p.Form = canon.PriceForm(form)
		if marketKey+"\x00"+source != pendingKey {
			if err := flush(); err != nil {
				return err
			}
			key, err := canon.ParseMarketKey(marketKey)
			if err != nil {
				return fmt.Errorf("restore current price: %w", err)
			}
			pendingKey = marketKey + "\x00" + source
			pending = canon.Delta{Source: source, Market: canon.Market{Key: key, SourceKey: sourceMarketKey, Rules: rules}, FetchedAt: p.ObservedAt}
		}
		pending.Market.Selections = append(pending.Market.Selections, canon.Selection{Side: canon.Side(side), Label: label, Price: p, DeepLink: deepLink})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate current prices: %w", err)
	}
	return flush()
}

const selectKickoffs = `SELECT id::text, kickoff FROM events WHERE kickoff > $1 AND status IN ('scheduled', 'live') AND "mergedInto" IS NULL`

func (s *store) loadKickoffs(ctx context.Context, kickoffAfter time.Time) (map[string]time.Time, error) {
	rows, err := s.pool.Query(ctx, selectKickoffs, kickoffAfter)
	if err != nil {
		return nil, fmt.Errorf("query kickoffs: %w", err)
	}
	kickoffs := map[string]time.Time{}
	var eventID string
	var kickoff time.Time
	if _, err := pgx.ForEachRow(rows, []any{&eventID, &kickoff}, func() error {
		kickoffs[eventID] = kickoff
		return nil
	}); err != nil {
		return nil, fmt.Errorf("scan kickoffs: %w", err)
	}
	return kickoffs, nil
}

const selectOpenSignals = `SELECT s.id::text, s."dedupeKey", s.type, s."eventId"::text, s."leagueCode", s."marketKey", s.margin, s.ev, s."fairProb", s.legs, s."oldestLegAgeMs", s."detectedAt"
FROM signals s WHERE s.status = 'open'`

const selectOpenSignalsOfStartedEvents = selectOpenSignals + ` AND EXISTS (SELECT 1 FROM events e WHERE e.id = s."eventId" AND e.kickoff <= $1)`

func (s *store) loadOpenSignals(ctx context.Context) ([]canon.Signal, error) {
	return s.queryOpenSignals(ctx, selectOpenSignals)
}

func (s *store) loadOpenSignalsOfStartedEvents(ctx context.Context, now time.Time) ([]canon.Signal, error) {
	return s.queryOpenSignals(ctx, selectOpenSignalsOfStartedEvents, now)
}

func (s *store) queryOpenSignals(ctx context.Context, sql string, args ...any) ([]canon.Signal, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query open signals: %w", err)
	}
	defer rows.Close()
	var open []canon.Signal
	for rows.Next() {
		var sig canon.Signal
		var kind, marketKey string
		var legs []byte
		var oldestLegAgeMs int64
		if err := rows.Scan(&sig.ID, &sig.DedupeKey, &kind, &sig.EventID, &sig.LeagueCode, &marketKey, &sig.Margin, &sig.EV, &sig.FairProb, &legs, &oldestLegAgeMs, &sig.DetectedAt); err != nil {
			return nil, fmt.Errorf("scan open signal: %w", err)
		}
		if sig.Market, err = canon.ParseMarketKey(marketKey); err != nil {
			return nil, fmt.Errorf("open signal %s: %w", sig.DedupeKey, err)
		}
		if err := json.Unmarshal(legs, &sig.Legs); err != nil {
			return nil, fmt.Errorf("decode legs of signal %s: %w", sig.DedupeKey, err)
		}
		sig.Type, sig.Status = canon.SignalType(kind), canon.SignalOpen
		sig.OldestLegAge = time.Duration(oldestLegAgeMs) * time.Millisecond
		signal.DescribeMove(&sig)
		open = append(open, sig)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate open signals: %w", err)
	}
	return open, nil
}

const closeOpenSignals = `UPDATE signals s SET status = 'closed', "closedAt" = c."closedAt"
FROM unnest($1::text[], $2::timestamptz[]) AS c("dedupeKey", "closedAt")
WHERE s."dedupeKey" = c."dedupeKey" AND s.status = 'open' AND s."detectedAt" <= c."closedAt"
RETURNING s."dedupeKey", s.id::text, s."leagueCode", s."detectedAt", false`

const upsertOpenSignals = `INSERT INTO signals (id, "dedupeKey", type, status, "eventId", "leagueCode", "marketKey", margin, ev, "fairProb", "peakMargin", legs, "oldestLegAgeMs", "detectedAt")
SELECT v.id::uuid, v."dedupeKey", v.type, 'open', e.id, e."leagueCode", v."marketKey", v.margin, v.ev, v."fairProb", v.margin, v.legs::jsonb, v."oldestLegAgeMs", v."detectedAt"
FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::float8[], $7::float8[], $8::float8[], $9::text[], $10::int8[], $11::timestamptz[])
	AS v(id, "dedupeKey", type, "eventId", "marketKey", margin, ev, "fairProb", legs, "oldestLegAgeMs", "detectedAt")
JOIN events e ON e.id = v."eventId"::uuid AND e."mergedInto" IS NULL
WHERE NOT EXISTS (SELECT 1 FROM signals closed WHERE closed.id = v.id::uuid AND closed.status = 'closed')
ON CONFLICT ("dedupeKey") WHERE status = 'open' DO UPDATE SET
	margin = EXCLUDED.margin, ev = EXCLUDED.ev, "fairProb" = EXCLUDED."fairProb", legs = EXCLUDED.legs,
	"oldestLegAgeMs" = EXCLUDED."oldestLegAgeMs", "peakMargin" = GREATEST(signals."peakMargin", EXCLUDED.margin)
RETURNING "dedupeKey", id::text, "leagueCode", "detectedAt", xmax = 0`

type signalColumns struct {
	ids, dedupeKeys, types, eventIDs, marketKeys, legs []string
	margins, evs, fairProbs                            []float64
	oldestLegAgeMs                                     []int64
	detectedAt                                         []time.Time
}

func (c *signalColumns) add(sig canon.Signal) error {
	legs, err := json.Marshal(sig.Legs)
	if err != nil {
		return fmt.Errorf("encode legs of signal %s: %w", sig.DedupeKey, err)
	}
	c.ids, c.dedupeKeys, c.types = append(c.ids, sig.ID), append(c.dedupeKeys, sig.DedupeKey), append(c.types, string(sig.Type))
	c.eventIDs, c.marketKeys, c.legs = append(c.eventIDs, sig.EventID), append(c.marketKeys, sig.Market.String()), append(c.legs, string(legs))
	c.margins, c.evs, c.fairProbs = append(c.margins, sig.Margin), append(c.evs, sig.EV), append(c.fairProbs, sig.FairProb)
	c.oldestLegAgeMs, c.detectedAt = append(c.oldestLegAgeMs, sig.OldestLegAge.Milliseconds()), append(c.detectedAt, sig.DetectedAt)
	return nil
}

func (s *store) persistSignals(ctx context.Context, rounds [][]canon.Signal) ([]canon.Signal, error) {
	var changed []canon.Signal
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		changed = nil
		for _, round := range rounds {
			opened, err := upsertSignals(ctx, tx, round)
			if err != nil {
				return err
			}
			closed, err := closeSignals(ctx, tx, round)
			if err != nil {
				return err
			}
			changed = append(append(changed, opened...), closed...)
		}
		return nil
	})
	return changed, err
}

func upsertSignals(ctx context.Context, tx pgx.Tx, round []canon.Signal) ([]canon.Signal, error) {
	var cols signalColumns
	byDedupeKey := make(map[string]canon.Signal, len(round))
	for _, sig := range round {
		if _, err := uuid.Parse(sig.EventID); err != nil {
			continue
		}
		if err := cols.add(sig); err != nil {
			return nil, err
		}
		byDedupeKey[sig.DedupeKey] = sig
	}
	if len(byDedupeKey) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, upsertOpenSignals, cols.ids, cols.dedupeKeys, cols.types, cols.eventIDs, cols.marketKeys,
		cols.margins, cols.evs, cols.fairProbs, cols.legs, cols.oldestLegAgeMs, cols.detectedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert signals: %w", err)
	}
	return collectChangedSignals(rows, byDedupeKey, openedForm)
}

func openedForm(sig canon.Signal, isInserted bool) (canon.Signal, bool) {
	if sig.Status != canon.SignalClosed {
		return sig, true
	}
	sig.Status, sig.ClosedAt = canon.SignalOpen, nil
	return sig, isInserted
}

func closeSignals(ctx context.Context, tx pgx.Tx, round []canon.Signal) ([]canon.Signal, error) {
	byDedupeKey := map[string]canon.Signal{}
	var dedupeKeys []string
	var closedAt []time.Time
	for _, sig := range round {
		if sig.Status == canon.SignalClosed {
			byDedupeKey[sig.DedupeKey] = sig
			dedupeKeys, closedAt = append(dedupeKeys, sig.DedupeKey), append(closedAt, *sig.ClosedAt)
		}
	}
	if len(dedupeKeys) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, closeOpenSignals, dedupeKeys, closedAt)
	if err != nil {
		return nil, fmt.Errorf("close signals: %w", err)
	}
	return collectChangedSignals(rows, byDedupeKey, func(sig canon.Signal, _ bool) (canon.Signal, bool) { return sig, true })
}

func collectChangedSignals(rows pgx.Rows, byDedupeKey map[string]canon.Signal, emitted func(canon.Signal, bool) (canon.Signal, bool)) ([]canon.Signal, error) {
	defer rows.Close()
	var changed []canon.Signal
	for rows.Next() {
		var dedupeKey string
		var stored canon.Signal
		var isInserted bool
		if err := rows.Scan(&dedupeKey, &stored.ID, &stored.LeagueCode, &stored.DetectedAt, &isInserted); err != nil {
			return nil, fmt.Errorf("scan stored signal: %w", err)
		}
		sig, ok := emitted(byDedupeKey[dedupeKey], isInserted)
		if !ok {
			continue
		}
		sig.ID, sig.LeagueCode, sig.DetectedAt = stored.ID, stored.LeagueCode, stored.DetectedAt
		changed = append(changed, sig)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stored signals: %w", err)
	}
	return changed, nil
}
