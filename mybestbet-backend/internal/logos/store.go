package logos

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) UpsertCandidate(ctx context.Context, q Querier, kind Kind, entityID, sourceURL string) error {
	if entityID == "" || sourceURL == "" {
		return nil
	}
	_, err := q.Exec(ctx, `
		INSERT INTO logos (kind, "entityId", "sourceUrl") VALUES ($1, $2, $3)
		ON CONFLICT (kind, "entityId") DO UPDATE SET
			"sourceUrl" = EXCLUDED."sourceUrl", sha256 = NULL, bytes = NULL, "contentType" = NULL,
			"fetchedAt" = NULL, "failedAt" = NULL, "lastError" = NULL, "updatedAt" = now()
		WHERE logos."sourceUrl" IS DISTINCT FROM EXCLUDED."sourceUrl"
		  AND (logos.bytes IS NULL OR split_part(logos."sourceUrl", '/', 3) = split_part(EXCLUDED."sourceUrl", '/', 3))`,
		string(kind), entityID, sourceURL)
	if err != nil {
		return fmt.Errorf("upsert %s logo candidate %s: %w", kind, entityID, err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, kind Kind, entityID string) (Logo, error) {
	logo := Logo{Kind: kind, EntityID: entityID}
	err := s.pool.QueryRow(ctx, `
		SELECT "contentType", sha256, bytes FROM logos
		WHERE kind = $1 AND "entityId" = $2 AND bytes IS NOT NULL AND sha256 IS NOT NULL AND "contentType" IS NOT NULL`,
		string(kind), entityID).Scan(&logo.ContentType, &logo.SHA256, &logo.Bytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Logo{}, ErrNotFound
	}
	if err != nil {
		return Logo{}, fmt.Errorf("load %s logo %s: %w", kind, entityID, err)
	}
	return logo, nil
}

func (s *Store) Versions(ctx context.Context, kind Kind, ids []string) (map[string]string, error) {
	versions := map[string]string{}
	if len(ids) == 0 {
		return versions, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT "entityId", left(sha256, $3) FROM logos
		WHERE kind = $1 AND "entityId" = ANY($2) AND bytes IS NOT NULL AND sha256 IS NOT NULL`,
		string(kind), ids, versionLength)
	if err != nil {
		return nil, fmt.Errorf("list %s logo versions: %w", kind, err)
	}
	var entityID, version string
	_, err = pgx.ForEachRow(rows, []any{&entityID, &version}, func() error {
		versions[entityID] = version
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s logo versions: %w", kind, err)
	}
	return versions, nil
}

type candidate struct {
	Kind      Kind
	EntityID  string
	SourceURL string
}

func (s *Store) due(ctx context.Context, staleBefore, retryBefore time.Time, limit int) ([]candidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT kind, "entityId", "sourceUrl" FROM logos
		WHERE (bytes IS NULL OR "fetchedAt" < $1) AND ("failedAt" IS NULL OR "failedAt" < $2)
		ORDER BY "fetchedAt" NULLS FIRST, kind, "entityId" LIMIT $3`, staleBefore, retryBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list due logos: %w", err)
	}
	due, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		return nil, fmt.Errorf("scan due logos: %w", err)
	}
	return due, nil
}

func (s *Store) saveFetched(ctx context.Context, c candidate, image fetchedImage, at time.Time) (changed bool, err error) {
	var previous *string
	err = s.pool.QueryRow(ctx, `
		WITH prior AS (SELECT sha256 FROM logos WHERE kind = $1 AND "entityId" = $2 AND "sourceUrl" = $3 FOR UPDATE)
		UPDATE logos SET
			"fetchedAt" = $4, "failedAt" = NULL, "lastError" = NULL,
			sha256 = $5, "contentType" = $6,
			bytes = CASE WHEN logos.sha256 IS DISTINCT FROM $5 THEN $7::bytea ELSE logos.bytes END,
			"updatedAt" = CASE WHEN logos.sha256 IS DISTINCT FROM $5 THEN now() ELSE logos."updatedAt" END
		FROM prior
		WHERE logos.kind = $1 AND logos."entityId" = $2 AND logos."sourceUrl" = $3
		RETURNING prior.sha256`,
		string(c.Kind), c.EntityID, c.SourceURL, at, image.sha256, image.contentType, image.bytes).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store %s logo %s: %w", c.Kind, c.EntityID, err)
	}
	return previous == nil || *previous != image.sha256, nil
}

func (s *Store) saveFailure(ctx context.Context, c candidate, cause error, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE logos SET "failedAt" = $4, "lastError" = $5, "updatedAt" = now()
		WHERE kind = $1 AND "entityId" = $2 AND "sourceUrl" = $3`,
		string(c.Kind), c.EntityID, c.SourceURL, at, cause.Error())
	if err != nil {
		return fmt.Errorf("record %s logo %s failure: %w", c.Kind, c.EntityID, err)
	}
	return nil
}
