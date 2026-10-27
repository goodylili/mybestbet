package wire

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/resolve"
	"mybestbet/internal/review"
)

const defaultResolverMetricsAddr = ":9103"

func RunResolver(ctx context.Context, cfg config.Config, log *slog.Logger) error {
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
	resolver, aliasUpdates, err := openResolver(ctx, pool, rdb, log)
	if err != nil {
		return err
	}
	defer func() { _ = aliasUpdates.Close() }()
	dedup, err := resolve.LoadDedupConfig(configPath())
	if err != nil {
		return err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return resolve.NewReprocessor(resolver).Run(groupCtx, aliasUpdates) })
	group.Go(func() error { return resolve.NewDedupJob(resolver, dedup).Run(groupCtx) })
	group.Go(func() error {
		return observability.Serve(groupCtx, metricsAddrOr(defaultResolverMetricsAddr), observability.Mux(), log)
	})
	return group.Wait()
}

func openResolver(ctx context.Context, pool *pgxpool.Pool, rdb *goredis.Client, log *slog.Logger) (*resolve.Resolver, *goredis.PubSub, error) {
	matching, err := resolve.LoadMatchingConfig(configPath())
	if err != nil {
		return nil, nil, err
	}
	resolver, err := resolve.New(pool, rdb, review.NewService(pool, rdb), log)
	if err != nil {
		return nil, nil, err
	}
	resolver.WithMatching(matching)
	aliasUpdates, err := resolve.SubscribeAliasUpdates(ctx, rdb)
	if err != nil {
		return nil, nil, err
	}
	mergeDuplicateEvents(ctx, resolver, log)
	if err := resolver.Load(ctx); err != nil {
		_ = aliasUpdates.Close()
		return nil, nil, err
	}
	return resolver, aliasUpdates, nil
}

func mergeDuplicateEvents(ctx context.Context, resolver *resolve.Resolver, log *slog.Logger) {
	report, err := resolver.MergeDuplicateEvents(ctx)
	if err != nil {
		log.Warn("duplicate event merge failed", "error", err)
		return
	}
	log.Info("duplicate events merged", "groups", report.Groups, "merged_events", report.MergedEvents,
		"moved_links", report.MovedLinks, "closed_signals", report.ClosedSignals)
}

const defaultConfigPath = "config.yaml"

func configPath() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	return defaultConfigPath
}

func metricsAddrOr(fallback string) string {
	if addr := os.Getenv("METRICS_ADDR"); addr != "" {
		return addr
	}
	return fallback
}
