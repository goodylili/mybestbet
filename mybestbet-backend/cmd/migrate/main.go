package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/logger"
	"mybestbet/internal/platform/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	log := logger.New("migrate", cfg.Env)
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("open postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	log.Info("migrations applied")
}
