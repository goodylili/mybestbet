package seed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

type SeederDeps struct {
	Store      *Store
	Config     config.Config
	Log        *slog.Logger
	Now        func() time.Time
	SourceDeps func(source string) (sources.Deps, error)
}

type LeagueSeeder interface {
	SeedLeague(ctx context.Context, league League) error
}

type SeederBuilder func(SeederDeps) (LeagueSeeder, error)

var sportSeederBuilders = map[canon.Sport]SeederBuilder{}

func RegisterSportSeeder(sport canon.Sport, build SeederBuilder) {
	sportSeederBuilders[sport] = build
}

func BuildSportSeeders(deps SeederDeps) (map[canon.Sport]LeagueSeeder, error) {
	seeders := make(map[canon.Sport]LeagueSeeder, len(sportSeederBuilders))
	for sport, build := range sportSeederBuilders {
		seeder, err := build(deps)
		if err != nil {
			return nil, fmt.Errorf("build %s seeder: %w", sport, err)
		}
		seeders[sport] = seeder
	}
	return seeders, nil
}
