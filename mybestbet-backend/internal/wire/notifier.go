package wire

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"golang.org/x/sync/errgroup"

	"mybestbet/internal/notify"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
)

const (
	defaultNotifierMetricsAddr = ":9105"
	notifierHTTPTimeout        = 60 * time.Second
)

func RunNotifier(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	metricsAddr := notifierEnvOr("METRICS_ADDR", defaultNotifierMetricsAddr)
	if cfg.Telegram.Token == "" {
		log.Warn("TELEGRAM_BOT_TOKEN not set, notifier idle")
		return observability.Serve(ctx, metricsAddr, observability.Mux(), log)
	}
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
	consumer, err := notifierConsumerName()
	if err != nil {
		return err
	}
	notifier := notify.New(notify.Deps{
		Config:          cfg,
		Pool:            pool,
		Redis:           rdb,
		HTTPClient:      &http.Client{Timeout: notifierHTTPTimeout},
		TelegramBaseURL: os.Getenv("TELEGRAM_API_URL"),
		WebBaseURL:      os.Getenv("PUBLIC_WEB_URL"),
		Consumer:        consumer,
		Log:             log,
	})
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return notifier.Run(groupCtx) })
	group.Go(func() error { return observability.Serve(groupCtx, metricsAddr, observability.Mux(), log) })
	return group.Wait()
}

func notifierConsumerName() (string, error) {
	if name := os.Getenv("NOTIFIER_CONSUMER"); name != "" {
		return name, nil
	}
	host, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve notifier consumer name: %w", err)
	}
	return host, nil
}

func notifierEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
