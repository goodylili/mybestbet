package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/resolve"
	"mybestbet/pkg/sources"
)

type BrowserFactory func() sources.Browser

type Service struct {
	runners []*runner
	focus   *focusSet
}

func New(cfg config.Config, resolver *resolve.Resolver, rdb *goredis.Client, newBrowser BrowserFactory, log *slog.Logger) (*Service, error) {
	if len(cfg.Tiers) == 0 {
		return nil, fmt.Errorf("ingest: config has no freshness tiers")
	}
	s := &Service{focus: newFocusSet(rdb, log)}
	for _, name := range enabledSourceNames(cfg) {
		factory, registered := sources.Lookup(name)
		if !registered {
			log.Warn("enabled source has no registered adapter", "source", name)
			continue
		}
		r, err := newRunner(cfg, name, factory, resolver, rdb, newBrowser, log)
		if err != nil {
			return nil, err
		}
		r.focus = s.focus
		s.runners = append(s.runners, r)
	}
	return s, nil
}

func enabledSourceNames(cfg config.Config) []string {
	var names []string
	for name, sc := range cfg.Sources {
		if sc.Enabled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func newRunner(cfg config.Config, name string, factory sources.Factory, resolver *resolve.Resolver, rdb *goredis.Client, newBrowser BrowserFactory, log *slog.Logger) (*runner, error) {
	sc := cfg.Sources[name]
	adapter, err := buildAdapter(name, sc, factory, newBrowser, log)
	if err != nil {
		return nil, err
	}
	streamer, _ := adapter.(sources.Streamer)
	discoveryEvery := cfg.Discovery
	if discoveryEvery <= 0 {
		discoveryEvery = defaultDiscovery
	}
	return &runner{
		name:           name,
		adapter:        adapter,
		streamer:       streamer,
		leagues:        sharedLeagues(cfg.LeagueCodes(), adapter.Capabilities().Leagues),
		tiers:          sortedTiers(cfg.Tiers),
		discoveryEvery: discoveryEvery,
		ceilingRPS:     sc.MaxRPS,
		resolver:       resolver,
		rdb:            rdb,
		log:            log,
		now:            func() time.Time { return time.Now().UTC() },
		unitRandom:     unitRandom,
		schedule:       newSchedule(),
		hashes:         newMarketHashes(),
		health:         &health{},
		streamRestart:  make(chan struct{}, 1),
	}, nil
}

func newLimiter(maxRPS float64) *rate.Limiter {
	if maxRPS <= 0 {
		return rate.NewLimiter(rate.Inf, 1)
	}
	return rate.NewLimiter(rate.Limit(maxRPS), max(1, int(maxRPS)))
}

func sharedLeagues(configured, supported []string) []string {
	if len(supported) == 0 {
		return configured
	}
	return slices.DeleteFunc(slices.Clone(configured), func(code string) bool { return !slices.Contains(supported, code) })
}

func (s *Service) Sources() []string {
	names := make([]string, len(s.runners))
	for i, r := range s.runners {
		names[i] = r.name
	}
	return names
}

func (s *Service) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return s.focus.refreshUntilDone(ctx) })
	for _, r := range s.runners {
		g.Go(func() error { return r.run(ctx) })
	}
	return g.Wait()
}
