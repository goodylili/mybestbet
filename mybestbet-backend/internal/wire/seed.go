package wire

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/resolve"
	"mybestbet/internal/seed"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/browser"
)

const (
	statSourceRequestTimeout = 30 * time.Second
	statSourceBrowserTimeout = 90 * time.Second
	defaultStatSourceRPS     = 1.0
)

func RunSeed(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	pool, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	matching, err := resolve.LoadMatchingConfig(configPath())
	if err != nil {
		return err
	}
	deps, err := statSourceDeps(cfg, sofascore.SourceName, log)
	if err != nil {
		return err
	}
	fallbackDeps, err := statSourceDeps(cfg, fotmob.SourceName, log)
	if err != nil {
		return err
	}
	fallbackDeps.Config.LeagueIDs = withDefaults(seed.DefaultFotMobLeagues, fallbackDeps.Config.LeagueIDs)
	leagues, err := seedLeagues(cfg)
	if err != nil {
		return err
	}
	store := seed.NewStore(pool).WithKickoffTolerance(matching.KickoffToleranceFor)
	now := func() time.Time { return time.Now().UTC() }
	sportSeeders, err := seed.BuildSportSeeders(seed.SeederDeps{
		Store: store, Config: cfg, Log: log, Now: now,
		SourceDeps: func(source string) (sources.Deps, error) { return statSourceDeps(cfg, source, log) },
	})
	if err != nil {
		return err
	}
	svc := seed.NewService(sofascore.New(deps), fotmob.New(fallbackDeps), store, log, now).WithSportSeeders(sportSeeders)
	return svc.SeedAll(ctx, leagues)
}

func selectedSports() []canon.Sport {
	var sports []canon.Sport
	for raw := range strings.SplitSeq(os.Getenv("SEED_SPORTS"), ",") {
		if sport := strings.TrimSpace(raw); sport != "" {
			sports = append(sports, canon.Sport(sport))
		}
	}
	return sports
}

func withDefaults(defaults, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(defaults)+len(overrides))
	maps.Copy(merged, defaults)
	maps.Copy(merged, overrides)
	return merged
}

func seedLeagues(cfg config.Config) ([]seed.League, error) {
	tournamentIDs := map[string]int64{}
	maps.Copy(tournamentIDs, seed.DefaultUniqueTournaments)
	for code, raw := range cfg.Sources[sofascore.SourceName].LeagueIDs {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("sofascore leagueIds.%s: %w", code, err)
		}
		tournamentIDs[code] = parsed
	}
	selected := selectedSports()
	leagues := make([]seed.League, 0, len(cfg.Leagues))
	for _, l := range cfg.Leagues {
		if len(selected) > 0 && !slices.Contains(selected, l.Sport) {
			continue
		}
		leagues = append(leagues, seed.League{Code: l.Code, Name: l.Name, Country: l.Country, Sport: l.Sport, UniqueTournamentID: tournamentIDs[l.Code]})
	}
	return leagues, nil
}

var errStatSourceDisabled = errors.New("stat source disabled in config")

type disabledTransport struct{ source string }

func (t disabledTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("%s: %w", t.source, errStatSourceDisabled)
}

func statSourceEnabled(cfg config.Config) func(string) bool {
	return func(source string) bool { return cfg.Sources[source].Enabled }
}

func statSourceDeps(cfg config.Config, source string, log *slog.Logger) (sources.Deps, error) {
	sc := cfg.Sources[source]
	if !sc.Enabled {
		return sources.Deps{
			HTTP:    &http.Client{Transport: disabledTransport{source: source}},
			Limiter: rate.NewLimiter(rate.Inf, 1),
			Config:  sc,
			Logger:  log.With("source", source),
		}, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if sc.Proxy != "" {
		proxyURL, err := url.Parse(sc.Proxy)
		if err != nil {
			return sources.Deps{}, fmt.Errorf("%s proxy: %w", source, err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	rps := sc.MaxRPS
	if rps <= 0 {
		rps = defaultStatSourceRPS
	}
	deps := sources.Deps{
		HTTP:    &http.Client{Timeout: statSourceRequestTimeout, Transport: transport},
		Limiter: rate.NewLimiter(rate.Limit(rps), 1),
		Config:  sc,
		Logger:  log.With("source", source),
	}
	if cfg.BrowserdURL != "" {
		deps.Browser = browser.New(cfg.BrowserdURL, &http.Client{Timeout: statSourceBrowserTimeout})
	}
	return deps, nil
}
