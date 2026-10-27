package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/logger"
	"mybestbet/internal/wire"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	log := logger.New("stats", cfg.Env)
	if err := wire.RunStats(ctx, cfg, log); err != nil {
		log.Error("stats stopped", "err", err)
		stop()
		os.Exit(1)
	}
}
