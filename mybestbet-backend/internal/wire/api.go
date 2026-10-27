package wire

import (
	"context"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"mybestbet/internal/auth"
	"mybestbet/internal/booking"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/review"
	"mybestbet/internal/server"
	"mybestbet/internal/stats"
	_ "mybestbet/pkg/sources/all"
)

func RunAPI(ctx context.Context, cfg config.Config, log *slog.Logger) error {
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
	authService, err := auth.New(auth.Deps{Config: cfg, Pool: pool, Redis: rdb, Log: log})
	if err != nil {
		return err
	}
	defer authService.Wait()
	bookings, err := booking.New(booking.Deps{Config: cfg, Pool: pool, Redis: rdb, Log: log, NewBrowser: browserFactory(cfg)})
	if err != nil {
		return err
	}
	log.Info("booking sources", "books", bookings.Bookable())
	api := server.New(server.Deps{
		Config: cfg,
		Pool:   pool,
		Redis:  rdb,
		Stats:  stats.NewReader(pool),
		Review: review.NewService(pool, rdb),
		Auth:   authService,
		Books:  bookings,
		Log:    log,
	})
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return api.RunStreamHub(groupCtx) })
	group.Go(func() error { return observability.Serve(groupCtx, cfg.HTTPAddr, api.Handler(), log) })
	return group.Wait()
}
