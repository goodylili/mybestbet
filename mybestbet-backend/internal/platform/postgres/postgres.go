package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mybestbet/migrations"
)

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

const migrationsTable = `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, "appliedAt" timestamptz NOT NULL DEFAULT now())`

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, migrationsTable); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := applyOnce(ctx, pool, name); err != nil {
			return err
		}
	}
	return nil
}

func applyOnce(ctx context.Context, pool *pgxpool.Pool, name string) error {
	version := strings.TrimSuffix(name, ".sql")
	sql, err := fs.ReadFile(migrations.FS, name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING`, version)
		if err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		return nil
	})
}
