package wire

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sync/errgroup"

	"mybestbet/internal/logos"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/seed"
	"mybestbet/internal/stats"
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/fbref"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/sources"
)

const (
	defaultStatsMetricsAddr = ":9107"
	statsBackfillEnv        = "STATS_BACKFILL"
	statsLiveOnceEnv        = "STATS_LIVE_ONCE"
	statsHistoryOnceEnv     = "STATS_HISTORY_ONCE"
)

func RunStats(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	sofascoreDeps, err := statSourceDeps(cfg, sofascore.SourceName, log)
	if err != nil {
		return err
	}
	fotmobDeps, err := statSourceDeps(cfg, fotmob.SourceName, log)
	if err != nil {
		return err
	}
	fbrefDeps, err := statSourceDeps(cfg, fbref.SourceName, log)
	if err != nil {
		return err
	}
	sportDeps := map[string]sources.Deps{}
	for _, source := range []string{espn.SourceName, euroleague.SourceName, esports.OpenDotaSource, flashscore.SourceName, bo3.SourceName, acb.SourceName} {
		if sportDeps[source], err = statSourceDeps(cfg, source, log); err != nil {
			return err
		}
	}
	store, entities, now := stats.NewStore(pool), seed.NewStore(pool), func() time.Time { return time.Now().UTC() }
	fotmobClient, espnClient, bo3Client := fotmob.New(fotmobDeps), espn.New(sportDeps[espn.SourceName]), bo3.New(sportDeps[bo3.SourceName])
	flashscoreClient := flashscore.New(sportDeps[flashscore.SourceName])
	openDotaClient, euroLeagueClient, acbClient := esports.NewOpenDota(sportDeps[esports.OpenDotaSource]), euroleague.New(sportDeps[euroleague.SourceName]), acb.New(sportDeps[acb.SourceName])
	svc := stats.NewService(stats.ServiceDeps{
		Sofascore:  sofascore.New(sofascoreDeps),
		FotMob:     fotmobClient,
		FBref:      fbref.New(fbrefDeps),
		ESPN:       espnClient,
		EuroLeague: euroLeagueClient,
		OpenDota:   openDotaClient,
		Bo3:        bo3Client,
		ACB:        acbClient,
		Store:      store,
		Entities:   entities,
		Leagues:    cfg.LeagueCodes(),
		Enabled:    statSourceEnabled(cfg),
		Log:        log,
		Now:        now,
	})
	if os.Getenv(statsHistoryOnceEnv) != "" {
		svc.CollectHistory(ctx)
		return nil
	}
	if backfill := os.Getenv(statsBackfillEnv); backfill != "" {
		return backfillSportStats(ctx, svc, backfill, log)
	}
	rdb, err := redis.Open(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	liveScores := stats.NewLiveScoreCollector(stats.LiveScoreCollectorDeps{
		FotMob:     fotmobClient,
		ESPN:       espnClient,
		Flashscore: flashscoreClient,
		Bo3:        bo3Client,
		OpenDota:   openDotaClient,
		EuroLeague: euroLeagueClient,
		ACB:        acbClient,
		Store:      store,
		Redis:      rdb,
		SportOf:    cfg.LeagueSport,
		Log:        log.With("job", "live_scores"),
		Now:        now,
	})
	if os.Getenv(statsLiveOnceEnv) != "" {
		liveScores.CollectRound(ctx)
		return nil
	}
	tennis := stats.NewTennisCollector(stats.TennisCollectorDeps{
		Flashscore: flashscoreClient,
		Store:      store,
		Entities:   entities,
		Leagues:    cfg.LeagueCodes(),
		Log:        log,
		Now:        now,
	})
	sourceLogos, err := logos.LoadSourceLogoURLs(configPath())
	if err != nil {
		return err
	}
	logoJob := logos.NewJob(logos.JobDeps{Store: logos.NewStore(pool), Fetcher: logos.NewFetcher(), SourceLogos: sourceLogos, Log: log.With("job", "logos"), Now: now})
	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = defaultStatsMetricsAddr
	}
	playerMerger := stats.NewPlayerMerger(stats.PlayerMergerDeps{
		Store:    store,
		FotMob:   fotmobClient,
		Redis:    rdb,
		Enabled:  statSourceEnabled(cfg),
		Interval: cfg.PlayerMerge.Interval,
		Log:      log.With("job", "player_merge"),
	})
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return svc.Run(groupCtx) })
	group.Go(func() error { return playerMerger.Run(groupCtx) })
	group.Go(func() error { return svc.RunHistory(groupCtx) })
	group.Go(func() error { return tennis.Run(groupCtx) })
	group.Go(func() error { return liveScores.Run(groupCtx) })
	group.Go(func() error { return logoJob.Run(groupCtx) })
	group.Go(func() error { return observability.Serve(groupCtx, metricsAddr, observability.Mux(), log) })
	return group.Wait()
}

func backfillSportStats(ctx context.Context, svc *stats.Service, lookbackText string, log *slog.Logger) error {
	lookback, err := time.ParseDuration(lookbackText)
	if err != nil {
		return fmt.Errorf("%s: %w", statsBackfillEnv, err)
	}
	log.Info("sport stats backfill started", "lookback", lookback)
	svc.BackfillSportStats(ctx, lookback)
	log.Info("sport stats backfill finished")
	return nil
}
