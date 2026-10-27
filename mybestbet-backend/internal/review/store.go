package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const itemColumns = `id::text, kind, source, "sourceKey", payload, candidates, status, "createdAt", COALESCE("decidedId"::text, '')`

const foreignKeyViolation = "23503"

type storedItem struct {
	Item
	DecidedID string
}

func scanItem(row pgx.Row) (storedItem, error) {
	var (
		it         storedItem
		payload    []byte
		candidates []byte
	)
	if err := row.Scan(&it.ID, &it.Kind, &it.Source, &it.SourceKey, &payload, &candidates, &it.Status, &it.CreatedAt, &it.DecidedID); err != nil {
		return storedItem{}, err
	}
	it.Payload = payload
	if err := json.Unmarshal(candidates, &it.Candidates); err != nil {
		return storedItem{}, fmt.Errorf("decode candidates of review %s: %w", it.ID, err)
	}
	return it, nil
}

func upsertPendingItem(ctx context.Context, pool *pgxpool.Pool, it NewItem) error {
	payload, err := json.Marshal(it.Payload)
	if err != nil {
		return fmt.Errorf("encode review payload: %w", err)
	}
	candidates, err := json.Marshal(nonNilCandidates(it.Candidates))
	if err != nil {
		return fmt.Errorf("encode review candidates: %w", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO review_queue (kind, source, "sourceKey", payload, candidates)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (kind, source, "sourceKey") DO UPDATE
		SET payload = EXCLUDED.payload, candidates = EXCLUDED.candidates
		WHERE review_queue.status = 'pending'`,
		it.Kind, it.Source, it.SourceKey, payload, candidates)
	if err != nil {
		return fmt.Errorf("upsert review item: %w", err)
	}
	return nil
}

func nonNilCandidates(c []Candidate) []Candidate {
	if c == nil {
		return []Candidate{}
	}
	return c
}

func listItems(ctx context.Context, pool *pgxpool.Pool, status string, limit int) ([]Item, error) {
	rows, err := pool.Query(ctx, `SELECT `+itemColumns+` FROM review_queue
		WHERE ($1 = '' OR status = $1) ORDER BY "createdAt" LIMIT $2`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list review items: %w", err)
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan review item: %w", err)
		}
		items = append(items, it.Item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate review items: %w", err)
	}
	return items, nil
}

func lockItem(ctx context.Context, tx pgx.Tx, id string) (storedItem, error) {
	it, err := scanItem(tx.QueryRow(ctx, `SELECT `+itemColumns+` FROM review_queue WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return storedItem{}, ErrNotFound
	}
	if err != nil {
		return storedItem{}, fmt.Errorf("lock review item: %w", err)
	}
	return it, nil
}

func markDecided(ctx context.Context, tx pgx.Tx, id, status, decidedID string) (Item, error) {
	it, err := scanItem(tx.QueryRow(ctx, `UPDATE review_queue
		SET status = $2, "decidedId" = NULLIF($3, '')::uuid, "decidedAt" = now()
		WHERE id = $1 RETURNING `+itemColumns, id, status, decidedID))
	if err != nil {
		return Item{}, fmt.Errorf("mark review decided: %w", err)
	}
	return it.Item, nil
}

func upsertTeamAlias(ctx context.Context, tx pgx.Tx, source, sourceName, teamID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO team_aliases (source, "sourceName", "teamId", confidence) VALUES ($1, $2, $3, 1)
		ON CONFLICT (source, "sourceName") DO UPDATE SET "teamId" = EXCLUDED."teamId", confidence = 1`, source, sourceName, teamID)
	return wrapWrite("upsert team alias", err)
}

func upsertPlayerAlias(ctx context.Context, tx pgx.Tx, source, sourceID, playerID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO player_aliases (source, "sourceId", "playerId", confidence) VALUES ($1, $2, $3, 1)
		ON CONFLICT (source, "sourceId") DO UPDATE SET "playerId" = EXCLUDED."playerId", confidence = 1`, source, sourceID, playerID)
	return wrapWrite("upsert player alias", err)
}

func upsertEventLink(ctx context.Context, tx pgx.Tx, source, sourceEventID, eventID string, flipped bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO event_links (source, "sourceEventId", "eventId", flipped) VALUES ($1, $2, $3, $4)
		ON CONFLICT (source, "sourceEventId") DO UPDATE SET "eventId" = EXCLUDED."eventId", flipped = EXCLUDED.flipped`,
		source, sourceEventID, eventID, flipped)
	return wrapWrite("upsert event link", err)
}

func insertTeam(ctx context.Context, tx pgx.Tx, league, name string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ($1, $2, $3) RETURNING id::text`,
		league, name, plainLower(name)).Scan(&id)
	return id, wrapWrite("insert team", err)
}

func insertPlayer(ctx context.Context, tx pgx.Tx, name, teamID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO players (name, "normalizedName", "teamId") VALUES ($1, $2, NULLIF($3, '')::uuid) RETURNING id::text`,
		name, plainLower(name), teamID).Scan(&id)
	return id, wrapWrite("insert player", err)
}

func upsertEvent(ctx context.Context, tx pgx.Tx, league, homeTeamID, awayTeamID string, kickoff time.Time) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ($1, $2, $3, $4)
		ON CONFLICT ("homeTeamId", "awayTeamId", kickoff) DO UPDATE SET kickoff = EXCLUDED.kickoff RETURNING id::text`,
		league, homeTeamID, awayTeamID, kickoff).Scan(&id)
	return id, wrapWrite("upsert event", err)
}

func eventHomeTeam(ctx context.Context, tx pgx.Tx, eventID string) (string, error) {
	var home string
	err := tx.QueryRow(ctx, `SELECT "homeTeamId"::text FROM events WHERE id = $1`, eventID).Scan(&home)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: event %s does not exist", ErrInvalidDecision, eventID)
	}
	return home, wrapWrite("read event teams", err)
}

func plainLower(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func wrapWrite(op string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return fmt.Errorf("%w: %s references a missing row", ErrInvalidDecision, op)
	}
	return fmt.Errorf("%s: %w", op, err)
}
