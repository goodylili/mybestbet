package wire

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"mybestbet/internal/ingest"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/sources"
	_ "mybestbet/pkg/sources/all"
	"mybestbet/pkg/sources/browser"
)

const (
	defaultIngestorMetricsAddr = ":9101"
	browserdRequestTimeout     = 90 * time.Second
)

func RunIngestor(ctx context.Context, cfg config.Config, log *slog.Logger) error {
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
	ingestor, err := ingest.New(cfg, resolver, rdb, browserFactory(cfg), log)
	if err != nil {
		return err
	}
	log.Info("ingestor sources", "sources", ingestor.Sources())
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return ingestor.Run(groupCtx) })
	group.Go(func() error { return resolver.FollowAliasUpdates(groupCtx, aliasUpdates, nil) })
	group.Go(func() error {
		return observability.Serve(groupCtx, metricsAddrOr(defaultIngestorMetricsAddr), observability.Mux(), log)
	})
	return group.Wait()
}

func browserFactory(cfg config.Config) ingest.BrowserFactory {
	return func() sources.Browser {
		return browser.New(cfg.BrowserdURL, &http.Client{Timeout: browserdRequestTimeout})
	}
}
