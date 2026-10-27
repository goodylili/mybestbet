package stats

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const playerMergeLockKey int64 = 0x706c6d65726765

type playerTwin struct {
	duplicateID         string
	survivorID          string
	name                string
	playedForClub       bool
	duplicateFotmobID   string
	nationalFotmobTeam  string
	clubFotmobTeams     []string
	hasConflictingIdent bool
}

type playerMergeResult struct {
	merged       bool
	movedAliases []movedAlias
}

type movedAlias struct {
	source   string
	sourceID string
}

const selectPlayerTwins = `
WITH national AS (SELECT id FROM teams WHERE "leagueCode" = ANY($1::text[])),
membership AS (
	SELECT id AS "playerId", "teamId" FROM players WHERE "teamId" IS NOT NULL
	UNION SELECT "playerId", "teamId" FROM player_teams),
national_copy AS (
	SELECT p.id, p."normalizedName", p."teamId", p."sofascoreId" FROM players p WHERE p."teamId" IN (SELECT id FROM national)),
club_copy AS (
	SELECT p.id, p."normalizedName", p."sofascoreId", array_agg(m."teamId") AS clubs
	FROM players p JOIN membership m ON m."playerId" = p.id
	WHERE p."teamId" NOT IN (SELECT id FROM national)
	  AND NOT EXISTS (SELECT 1 FROM membership x WHERE x."playerId" = p.id AND x."teamId" IN (SELECT id FROM national))
	GROUP BY p.id)
SELECT n.id::text, c.id::text, n."normalizedName",
	EXISTS (SELECT 1 FROM player_match_stats s WHERE s."playerId" = n.id AND s."teamId" = ANY(c.clubs)),
	COALESCE((SELECT min(a."sourceId") FROM player_aliases a WHERE a."playerId" = n.id AND a.source = $3), ''),
	COALESCE((SELECT min(ts."sourceId") FROM team_sources ts WHERE ts."teamId" = n."teamId" AND ts.source = $3), ''),
	COALESCE((SELECT array_agg(ts."sourceId") FROM team_sources ts WHERE ts."teamId" = ANY(c.clubs) AND ts.source = $3), '{}'),
	(n."sofascoreId" IS NOT NULL AND c."sofascoreId" IS NOT NULL) OR EXISTS (
		SELECT 1 FROM player_aliases a JOIN player_aliases b ON b.source = a.source AND b."sourceId" <> a."sourceId"
		WHERE a."playerId" = n.id AND b."playerId" = c.id AND a.source = ANY($2::text[]))
FROM national_copy n JOIN club_copy c ON c."normalizedName" = n."normalizedName" AND c.id <> n.id
ORDER BY n."normalizedName", n.id, c.id`

func (s *Store) withPlayerMergeLock(ctx context.Context, fn func(*pgxpool.Conn) error) (locked bool, err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire player merge connection: %w", err)
	}
	defer conn.Release()
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, playerMergeLockKey).Scan(&locked); err != nil {
		return false, fmt.Errorf("take player merge lock: %w", err)
	}
	if !locked {
		return false, nil
	}
	defer func() {
		cleanup := context.WithoutCancel(ctx)
		if _, unlockErr := conn.Exec(cleanup, `SELECT pg_advisory_unlock($1)`, playerMergeLockKey); unlockErr != nil {
			err = errors.Join(err, fmt.Errorf("release player merge lock: %w", unlockErr), conn.Conn().Close(cleanup))
		}
	}()
	return true, fn(conn)
}

func playerTwins(ctx context.Context, conn *pgxpool.Conn, nationalLeagues, identitySources []string, fotmobSource string) ([]playerTwin, error) {
	rows, err := conn.Query(ctx, selectPlayerTwins, nationalLeagues, identitySources, fotmobSource)
	if err != nil {
		return nil, fmt.Errorf("query player twins: %w", err)
	}
	twins, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (playerTwin, error) {
		var t playerTwin
		err := row.Scan(&t.duplicateID, &t.survivorID, &t.name, &t.playedForClub,
			&t.duplicateFotmobID, &t.nationalFotmobTeam, &t.clubFotmobTeams, &t.hasConflictingIdent)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan player twins: %w", err)
	}
	return twins, nil
}

type playerKeyedRows struct {
	table   string
	owned   string
	assign  string
	sameRow string
}

const survivorMarketKey = `replace(moved."marketKey", '|' || @duplicate::uuid::text || '|', '|' || @survivor::uuid::text || '|')`

const duplicateMarketOfEvents = `moved."eventId" = ANY(@events::uuid[]) AND moved."marketKey" LIKE '%|' || @duplicate::uuid::text || '|%'`

const samePriceSelection = `kept."marketKey" = ` + survivorMarketKey + ` AND (kept.source, kept.side, kept.label) = (moved.source, moved.side, moved.label)`

var duplicatePlayerRows = []playerKeyedRows{
	{table: "player_teams", owned: `moved."playerId" = @duplicate::uuid`, assign: `"playerId" = @survivor::uuid`, sameRow: `kept."playerId" = @survivor::uuid AND kept."teamId" = moved."teamId"`},
	{table: "player_match_stats", owned: `moved."playerId" = @duplicate::uuid`, assign: `"playerId" = @survivor::uuid`, sameRow: `kept."playerId" = @survivor::uuid AND kept."eventId" = moved."eventId"`},
	{table: "lineups", owned: `moved."playerId" = @duplicate::uuid`, assign: `"playerId" = @survivor::uuid`, sameRow: `kept."playerId" = @survivor::uuid AND kept."eventId" = moved."eventId"`},
	{table: "player_season_stats", owned: `moved."playerId" = @duplicate::uuid`, assign: `"playerId" = @survivor::uuid`,
		sameRow: `kept."playerId" = @survivor::uuid AND (kept."leagueCode", kept.season, kept.source) = (moved."leagueCode", moved.season, moved.source)`},
	{table: "prices_current", owned: `moved."playerId" = @duplicate::uuid`, assign: `"playerId" = @survivor::uuid, "marketKey" = ` + survivorMarketKey, sameRow: samePriceSelection},
	{table: "closing_prices", owned: duplicateMarketOfEvents, assign: `"marketKey" = ` + survivorMarketKey, sameRow: samePriceSelection},
	{table: "prices_history", owned: duplicateMarketOfEvents, assign: `"marketKey" = ` + survivorMarketKey,
		sameRow: samePriceSelection + ` AND kept."eventId" = moved."eventId" AND kept."observedAt" = moved."observedAt"`},
}

func (r playerKeyedRows) moveSQL() string {
	return `UPDATE ` + r.table + ` moved SET ` + r.assign + ` WHERE ` + r.owned +
		` AND NOT EXISTS (SELECT 1 FROM ` + r.table + ` kept WHERE ` + r.sameRow + `)`
}

func (r playerKeyedRows) dropSQL() string {
	return `DELETE FROM ` + r.table + ` moved WHERE ` + r.owned
}

func mergePlayerInto(ctx context.Context, conn *pgxpool.Conn, duplicateID, survivorID string) (playerMergeResult, error) {
	var result playerMergeResult
	err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var present int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM players WHERE id IN ($1, $2) ORDER BY id FOR UPDATE) locked`,
			duplicateID, survivorID).Scan(&present); err != nil {
			return fmt.Errorf("lock player pair: %w", err)
		}
		if present != 2 {
			return nil
		}
		var events []string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(array_agg(DISTINCT "eventId"::text), '{}') FROM prices_current WHERE "playerId" = $1`,
			duplicateID).Scan(&events); err != nil {
			return fmt.Errorf("read duplicate priced events: %w", err)
		}
		aliases, err := moveAliases(ctx, tx, duplicateID, survivorID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO player_teams ("playerId", "teamId") SELECT $2, "teamId" FROM players WHERE id = $1 AND "teamId" IS NOT NULL
			ON CONFLICT DO NOTHING`, duplicateID, survivorID); err != nil {
			return fmt.Errorf("keep duplicate team membership: %w", err)
		}
		args := pgx.NamedArgs{"duplicate": duplicateID, "survivor": survivorID, "events": events}
		for _, rows := range duplicatePlayerRows {
			if _, err := tx.Exec(ctx, rows.moveSQL(), args); err != nil {
				return fmt.Errorf("move %s rows: %w", rows.table, err)
			}
			if _, err := tx.Exec(ctx, rows.dropSQL(), args); err != nil {
				return fmt.Errorf("drop leftover %s rows: %w", rows.table, err)
			}
		}
		if err := absorbDuplicate(ctx, tx, duplicateID, survivorID); err != nil {
			return err
		}
		result = playerMergeResult{merged: true, movedAliases: aliases}
		return nil
	})
	if err != nil {
		return playerMergeResult{}, fmt.Errorf("merge player %s into %s: %w", duplicateID, survivorID, err)
	}
	return result, nil
}

func moveAliases(ctx context.Context, tx pgx.Tx, duplicateID, survivorID string) ([]movedAlias, error) {
	rows, err := tx.Query(ctx, `UPDATE player_aliases SET "playerId" = $2 WHERE "playerId" = $1 RETURNING source, "sourceId"`, duplicateID, survivorID)
	if err != nil {
		return nil, fmt.Errorf("move player aliases: %w", err)
	}
	aliases, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (movedAlias, error) {
		var a movedAlias
		err := row.Scan(&a.source, &a.sourceID)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan moved player aliases: %w", err)
	}
	return aliases, nil
}

func absorbDuplicate(ctx context.Context, tx pgx.Tx, duplicateID, survivorID string) error {
	var (
		position    *string
		sofascoreID *int64
	)
	if err := tx.QueryRow(ctx, `DELETE FROM players WHERE id = $1 RETURNING position, "sofascoreId"`, duplicateID).Scan(&position, &sofascoreID); err != nil {
		return fmt.Errorf("delete duplicate player: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE players SET position = COALESCE(position, $2), "sofascoreId" = COALESCE("sofascoreId", $3), "updatedAt" = now()
		WHERE id = $1 AND (position IS NULL AND $2::text IS NOT NULL OR "sofascoreId" IS NULL AND $3::bigint IS NOT NULL)`,
		survivorID, position, sofascoreID); err != nil {
		return fmt.Errorf("fill survivor player: %w", err)
	}
	return nil
}
