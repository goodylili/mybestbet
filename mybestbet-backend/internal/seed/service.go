package seed

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/canon"
)

const UpcomingHorizon = 7 * 24 * time.Hour

type Service struct {
	client       *sofascore.Client
	fallback     *fotmob.Client
	store        *Store
	sportSeeders map[canon.Sport]LeagueSeeder
	log          *slog.Logger
	now          func() time.Time
}

func NewService(client *sofascore.Client, fallback *fotmob.Client, store *Store, log *slog.Logger, now func() time.Time) *Service {
	return &Service{client: client, fallback: fallback, store: store, log: log, now: now}
}

func (s *Service) WithSportSeeders(seeders map[canon.Sport]LeagueSeeder) *Service {
	s.sportSeeders = seeders
	return s
}

func (s *Service) SeedAll(ctx context.Context, leagues []League) error {
	var errs []error
	for _, league := range inSeedingOrder(leagues) {
		if err := s.seedAnySport(ctx, league); err != nil {
			s.log.Error("seed league failed", "league", league.Code, "sport", league.Sport, "err", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func inSeedingOrder(leagues []League) []League {
	ordered := slices.Clone(leagues)
	slices.SortStableFunc(ordered, func(a, b League) int { return cmp.Compare(a.Format(), b.Format()) })
	return ordered
}

func (s *Service) seedAnySport(ctx context.Context, league League) error {
	if league.Sport == "" || league.Sport == canon.SportFootball {
		return s.SeedLeague(ctx, league)
	}
	if err := s.store.UpsertLeague(ctx, league); err != nil {
		return err
	}
	seeder, ok := s.sportSeeders[league.Sport]
	if !ok {
		s.log.Warn("no seeder registered for sport, league row only", "league", league.Code, "sport", league.Sport)
		return nil
	}
	return seeder.SeedLeague(ctx, league)
}

func (s *Service) SeedLeague(ctx context.Context, league League) error {
	if err := s.store.UpsertLeague(ctx, league); err != nil {
		return err
	}
	if s.fallback != nil {
		if err := s.store.UpsertLeagueLogo(ctx, league.Code, s.fallback.LeagueLogoURL(league.Code)); err != nil {
			return err
		}
	}
	if league.UniqueTournamentID == 0 {
		return s.seedFromFotMobIfConfigured(ctx, league, errNoSofascoreTournament)
	}
	err := s.seedFromSofascore(ctx, league)
	if err == nil {
		return nil
	}
	s.log.Warn("sofascore seed failed, falling back to fotmob", "source", sofascore.SourceName, "league", league.Code, "err", err)
	return s.seedFromFotMobIfConfigured(ctx, league, err)
}

var errNoSofascoreTournament = errors.New("seed: league has no sofascore unique tournament id")

func (s *Service) seedFromFotMobIfConfigured(ctx context.Context, league League, sofascoreErr error) error {
	if s.fallback == nil {
		return fmt.Errorf("league %s: %w", league.Code, sofascoreErr)
	}
	return s.seedFromFotMob(ctx, league)
}

func (s *Service) seedFromSofascore(ctx context.Context, league League) error {
	season, err := s.client.CurrentSeason(ctx, league.UniqueTournamentID)
	if err != nil {
		return fmt.Errorf("league %s: %w", league.Code, err)
	}
	teamIDs, teamErr := s.seedTeamsAndSquads(ctx, league, season.ID)
	if teamIDs == nil {
		return teamErr
	}
	events, err := s.client.UpcomingEvents(ctx, league.UniqueTournamentID, season.ID, s.now().Add(UpcomingHorizon))
	if err != nil {
		return errors.Join(teamErr, fmt.Errorf("league %s upcoming events: %w", league.Code, err))
	}
	errs := []error{teamErr}
	for _, e := range events {
		errs = append(errs, s.seedEvent(ctx, league.Code, e, teamIDs))
	}
	s.log.Info("league seeded", "source", sofascore.SourceName, "league", league.Code, "season", season.Year, "teams", len(teamIDs), "events", len(events))
	return errors.Join(errs...)
}

func (s *Service) seedTeamsAndSquads(ctx context.Context, league League, seasonID int64) (map[int64]string, error) {
	teams, err := s.client.SeasonTeams(ctx, league.UniqueTournamentID, seasonID)
	if err != nil {
		return nil, fmt.Errorf("league %s teams: %w", league.Code, err)
	}
	teamIDs := make(map[int64]string, len(teams))
	var errs []error
	for _, t := range teams {
		teamID, err := s.store.UpsertTeam(ctx, teamFromSofascore(league.Code, t))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		teamIDs[t.ID] = teamID
		errs = append(errs, s.seedSquadUnlessKnown(ctx, league, t.ID, teamID))
	}
	return teamIDs, errors.Join(errs...)
}

func (s *Service) seedSquadUnlessKnown(ctx context.Context, league League, sofascoreTeamID int64, teamID string) error {
	if league.Format() != DomesticLeague {
		hasSquad, err := s.store.HasPlayers(ctx, teamID)
		if err != nil || hasSquad {
			return err
		}
	}
	return s.seedSquad(ctx, sofascoreTeamID, teamID)
}

func (s *Service) seedSquad(ctx context.Context, sofascoreTeamID int64, teamID string) error {
	players, err := s.client.TeamPlayers(ctx, sofascoreTeamID)
	if err != nil {
		return fmt.Errorf("squad %d: %w", sofascoreTeamID, err)
	}
	var errs []error
	for _, p := range players {
		_, err := s.store.UpsertPlayer(ctx, Player{SofascoreID: p.ID, TeamID: teamID, Name: p.Name, Position: NormalizePosition(canon.SportFootball, p.Position)})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Service) seedEvent(ctx context.Context, leagueCode string, e sofascore.Event, teamIDs map[int64]string) error {
	homeID, err := s.teamIDFor(ctx, leagueCode, e.HomeTeam, teamIDs)
	if err != nil {
		return err
	}
	awayID, err := s.teamIDFor(ctx, leagueCode, e.AwayTeam, teamIDs)
	if err != nil {
		return err
	}
	_, err = s.store.UpsertEvent(ctx, Event{
		SofascoreID: e.ID,
		LeagueCode:  leagueCode,
		HomeTeamID:  homeID,
		AwayTeamID:  awayID,
		Kickoff:     e.Kickoff(),
		Status:      StatusFromSofascore(e.Status.Type),
	})
	return err
}

func (s *Service) teamIDFor(ctx context.Context, leagueCode string, t sofascore.Team, teamIDs map[int64]string) (string, error) {
	if id, ok := teamIDs[t.ID]; ok {
		return id, nil
	}
	id, err := s.store.UpsertTeam(ctx, teamFromSofascore(leagueCode, t))
	if err != nil {
		return "", err
	}
	teamIDs[t.ID] = id
	return id, nil
}

func teamFromSofascore(leagueCode string, t sofascore.Team) Team {
	return Team{SofascoreID: t.ID, LeagueCode: leagueCode, Name: t.Name, ShortName: t.ShortName}
}
