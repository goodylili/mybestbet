package stats

import (
	"context"
	"log/slog"
	"time"

	"mybestbet/internal/platform/observability"
	"mybestbet/internal/seed"
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/fbref"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
)

const (
	tickInterval           = time.Minute
	jobItemBudget          = 2 * time.Minute
	lineupLeadTime         = 60 * time.Minute
	lineupTrailTime        = 10 * time.Minute
	lineupPollInterval     = 5 * time.Minute
	sofascoreLineupGrace   = 30 * time.Minute
	statsDelayAfterKickoff = 4 * time.Hour
	statsRetryWindow       = 72 * time.Hour
	statsRetryInterval     = 30 * time.Minute
	statsEventsPerTick     = 20
	seasonStatsHourUTC     = 4
	attemptMemory          = 96 * time.Hour
)

type Service struct {
	sofascore  *sofascore.Client
	fotmob     *fotmob.Client
	fbref      *fbref.Client
	espn       *espn.Client
	euroleague *euroleague.Client
	opendota   *esports.OpenDota
	bo3        *bo3.Client
	acb        *acb.Client
	store      *Store
	entities   *seed.Store
	leagues    []string
	enabled    func(source string) bool
	log        *slog.Logger
	now        func() time.Time

	lastLineupPoll    map[string]time.Time
	lastStatsAttempt  map[string]time.Time
	nextSeasonStatsAt time.Time

	historyAttempts map[string]time.Time
	historyDays     map[string]bool
}

type ServiceDeps struct {
	Sofascore  *sofascore.Client
	FotMob     *fotmob.Client
	FBref      *fbref.Client
	ESPN       *espn.Client
	EuroLeague *euroleague.Client
	OpenDota   *esports.OpenDota
	Bo3        *bo3.Client
	ACB        *acb.Client
	Store      *Store
	Entities   *seed.Store
	Leagues    []string
	Enabled    func(source string) bool
	Log        *slog.Logger
	Now        func() time.Time
}

func NewService(d ServiceDeps) *Service {
	s := &Service{
		sofascore: d.Sofascore, fotmob: d.FotMob, fbref: d.FBref, store: d.Store, entities: d.Entities,
		espn: d.ESPN, euroleague: d.EuroLeague, opendota: d.OpenDota, bo3: d.Bo3, acb: d.ACB,
		leagues: d.Leagues, enabled: d.Enabled, log: d.Log, now: d.Now,
		lastLineupPoll:   map[string]time.Time{},
		lastStatsAttempt: map[string]time.Time{},
		historyAttempts:  map[string]time.Time{},
		historyDays:      map[string]bool{},
	}
	s.nextSeasonStatsAt = nextDailyRun(s.now(), seasonStatsHourUTC)
	return s
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		s.runDueJobs(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Service) runDueJobs(ctx context.Context) {
	now := s.now()
	s.pollLineups(ctx, now)
	s.collectMatchStats(ctx, now)
	s.collectSportStats(ctx, now)
	if !now.Before(s.nextSeasonStatsAt) {
		s.refreshSeasonStatsIfEnabled(ctx)
		s.nextSeasonStatsAt = nextDailyRun(now, seasonStatsHourUTC)
	}
	forgetOlderThan(s.lastLineupPoll, now.Add(-attemptMemory))
	forgetOlderThan(s.lastStatsAttempt, now.Add(-attemptMemory))
}

func (s *Service) isEnabled(source string) bool {
	return s.enabled == nil || s.enabled(source)
}

func (s *Service) refreshSeasonStatsIfEnabled(ctx context.Context) {
	if s.isEnabled(fbref.SourceName) {
		s.refreshSeasonStats(ctx)
	}
}

func recentlyAttempted(last map[string]time.Time, now time.Time, interval time.Duration) []string {
	ids := []string{}
	for id, at := range last {
		if now.Sub(at) < interval {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *Service) recordFailure(source, op string, attrs ...any) {
	recordJobFailure(s.log, source, op, attrs...)
}

func recordJobFailure(log *slog.Logger, source, op string, attrs ...any) {
	observability.FetchErrors.WithLabelValues(source, op).Inc()
	log.Warn("stats job failed", append([]any{"source", source, "op", op}, attrs...)...)
}

func nextDailyRun(after time.Time, hourUTC int) time.Time {
	after = after.UTC()
	run := time.Date(after.Year(), after.Month(), after.Day(), hourUTC, 0, 0, 0, time.UTC)
	if !run.After(after) {
		run = run.Add(24 * time.Hour)
	}
	return run
}

func isDue(last map[string]time.Time, key string, now time.Time, interval time.Duration) bool {
	at, seen := last[key]
	return !seen || now.Sub(at) >= interval
}

func forgetOlderThan(last map[string]time.Time, cutoff time.Time) {
	for key, at := range last {
		if at.Before(cutoff) {
			delete(last, key)
		}
	}
}
