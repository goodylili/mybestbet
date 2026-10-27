package seed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const inProgressLookback = 3 * time.Hour

var otherSports = []canon.Sport{canon.SportBasketball, canon.SportTennis, canon.SportAmericanFootball, canon.SportEsports}

func init() {
	for _, sport := range otherSports {
		RegisterSportSeeder(sport, NewOtherSportsSeeder)
	}
}

type OtherSportsSeeder struct {
	store      *Store
	log        *slog.Logger
	now        func() time.Time
	espn       *espn.Client
	euroleague *euroleague.Client
	liquipedia *esports.Liquipedia
	opendota   *esports.OpenDota
	flashscore *flashscore.Client
	bo3        *bo3.Client
	acb        *acb.Client

	tennisByLeague   map[string][]espn.TennisMatch
	flashscoreTennis *flashscoreTennisWindow
	dotaTeams        []esports.DotaTeam
}

func NewOtherSportsSeeder(d SeederDeps) (LeagueSeeder, error) {
	deps := map[string]sources.Deps{}
	for _, source := range []string{espn.SourceName, euroleague.SourceName, esports.LiquipediaSource, esports.OpenDotaSource, flashscore.SourceName, bo3.SourceName, acb.SourceName} {
		sourceDeps, err := d.SourceDeps(source)
		if err != nil {
			return nil, fmt.Errorf("%s deps: %w", source, err)
		}
		deps[source] = sourceDeps
	}
	return &OtherSportsSeeder{
		store: d.Store, log: d.Log, now: d.Now,
		espn:       espn.New(deps[espn.SourceName]),
		euroleague: euroleague.New(deps[euroleague.SourceName]),
		liquipedia: esports.NewLiquipedia(deps[esports.LiquipediaSource]),
		opendota:   esports.NewOpenDota(deps[esports.OpenDotaSource]),
		flashscore: flashscore.New(deps[flashscore.SourceName]),
		bo3:        bo3.New(deps[bo3.SourceName]),
		acb:        acb.New(deps[acb.SourceName]),
	}, nil
}

func (o *OtherSportsSeeder) SeedLeague(ctx context.Context, league League) error {
	switch {
	case hasKey(espn.Leagues, league.Code):
		return o.seedESPNLeague(ctx, league)
	case hasKey(euroleague.Competitions, league.Code):
		return o.seedEuroLeague(ctx, league)
	case hasKey(esports.Wikis, league.Code):
		return errors.Join(o.seedEsportsLeague(ctx, league), o.seedBo3League(ctx, league))
	case league.Code == acb.LeagueCode:
		return o.seedACB(ctx, league)
	case league.Sport == canon.SportTennis:
		return o.seedFlashscoreTennisLeague(ctx, league)
	}
	o.log.Warn("no public source for league, league row only", "league", league.Code, "sport", league.Sport)
	return nil
}

func (o *OtherSportsSeeder) isInSeedWindow(kickoff time.Time) bool {
	now := o.now()
	return kickoff.After(now.Add(-inProgressLookback)) && kickoff.Before(now.Add(UpcomingHorizon))
}

func (o *OtherSportsSeeder) seedWindowDays() []time.Time {
	start := o.now().UTC().Add(-inProgressLookback).Truncate(24 * time.Hour)
	end := o.now().UTC().Add(UpcomingHorizon)
	var days []time.Time
	for day := start; !day.After(end); day = day.Add(24 * time.Hour) {
		days = append(days, day)
	}
	return days
}

func (o *OtherSportsSeeder) logSeeded(source string, league League, teams, events int, err error) error {
	o.log.Info("league seeded", "source", source, "league", league.Code, "teams", teams, "events", events, "failed", err != nil)
	return err
}

func hasKey[V any](m map[string]V, key string) bool {
	_, ok := m[key]
	return ok
}
