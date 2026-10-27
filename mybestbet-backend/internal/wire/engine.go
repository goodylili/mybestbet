package wire

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"mybestbet/internal/engine"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
)

const defaultEngineMetricsAddr = ":9104"

func RunEngine(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	rdb, err := redis.Open(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	consumer, err := engineConsumerName()
	if err != nil {
		return err
	}
	eng, err := engine.New(ctx, engine.Deps{Config: cfg, Pool: pool, Redis: rdb, Log: log, Group: engine.DefaultGroup, Consumer: consumer, SpoolDir: engineEnvOr("ENGINE_SPOOL_DIR", filepath.Join(os.TempDir(), "mybestbet-spool-"+consumer))})
	if err != nil {
		return err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return eng.Run(groupCtx) })
	group.Go(func() error {
		return observability.Serve(groupCtx, engineEnvOr("METRICS_ADDR", defaultEngineMetricsAddr), observability.Mux(), log)
	})
	return group.Wait()
}

func engineConsumerName() (string, error) {
	if name := os.Getenv("ENGINE_CONSUMER"); name != "" {
		return name, nil
	}
	host, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("engine consumer name: %w", err)
	}
	return "engine-" + host, nil
}

func engineEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
