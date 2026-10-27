package stats

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/fotmob"
)

const (
	historyInterval         = 6 * time.Hour
	historyHorizon          = 7 * 24 * time.Hour
	historyFixturesPerTeam  = 20
	historyTeamBudget       = 15 * time.Minute
	historyESPNLookback     = 200 * 24 * time.Hour
	historyESPNSettledAfter = 3 * 24 * time.Hour
	historyESPNEventLimit   = 3000
	historyRetryInterval    = 24 * time.Hour
	dayLayout               = "2006-01-02"
)

type historyPass struct {
	teams    int
	fixtures int
	events   int
	rows     int
	failures int
}

func (s *Service) RunHistory(ctx context.Context) error {
	ticker := time.NewTicker(historyInterval)
	defer ticker.Stop()
	for {
		s.CollectHistory(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Service) CollectHistory(ctx context.Context) {
	now := s.now()
	if s.isEnabled(fotmob.SourceName) {
		s.footballHistory(ctx, now)
	}
	if s.isEnabled(espn.SourceName) {
		s.espnHistory(ctx, now)
	}
	forgetOlderThan(s.historyAttempts, now.Add(-attemptMemory))
}

func (s *Service) footballHistory(ctx context.Context, now time.Time) {
	teams, err := s.store.PropTeams(ctx, fotmob.SourceName, now, now.Add(historyHorizon))
	if err != nil {
		s.recordFailure("postgres", "history", "err", err)
		return
	}
	var pass historyPass
	seenTeams, seenFixtures := map[string]bool{}, map[string]bool{}
	for _, t := range teams {
		if ctx.Err() != nil {
			return
		}
		if seenTeams[t.SourceID] {
			continue
		}
		seenTeams[t.SourceID] = true
		pass.teams++
		teamCtx, cancel := context.WithTimeout(ctx, historyTeamBudget)
		if err := s.teamFootballHistory(teamCtx, t.SourceID, now, seenFixtures, &pass); err != nil {
			pass.failures++
			s.recordFailure(fotmob.SourceName, "history", "team_id", t.TeamID, "err", err)
		}
		cancel()
	}
	s.log.Info("football history pass", "source", fotmob.SourceName, "teams", pass.teams, "fixtures", pass.fixtures,
		"events_collected", pass.events, "rows", pass.rows, "failures", pass.failures)
}

func (s *Service) teamFootballHistory(ctx context.Context, fotmobTeamID string, now time.Time, seen map[string]bool, pass *historyPass) error {
	fixtures, err := s.fotmob.FinishedFixtures(ctx, fotmobTeamID)
	if err != nil {
		return err
	}
	var errs []error
	for _, f := range s.recentLeagueFixtures(fixtures, now) {
		if seen[f.fixture.ID] {
			continue
		}
		seen[f.fixture.ID] = true
		pass.fixtures++
		rows, err := s.fixtureHistory(ctx, f, now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if rows > 0 {
			pass.events++
			pass.rows += rows
		}
	}
	return errors.Join(errs...)
}

type leagueFixture struct {
	fixture    fotmob.Fixture
	leagueCode string
}

func (s *Service) recentLeagueFixtures(fixtures []fotmob.Fixture, now time.Time) []leagueFixture {
	var recent []leagueFixture
	for _, f := range fixtures {
		code, ok := s.fotmob.LeagueCodeOf(f.LeagueID)
		if !ok || !slices.Contains(s.leagues, code) || now.Sub(f.Kickoff) < statsDelayAfterKickoff {
			continue
		}
		recent = append(recent, leagueFixture{fixture: f, leagueCode: code})
	}
	slices.SortFunc(recent, func(a, b leagueFixture) int { return b.fixture.Kickoff.Compare(a.fixture.Kickoff) })
	return recent[:min(len(recent), historyFixturesPerTeam)]
}

func (s *Service) fixtureHistory(ctx context.Context, f leagueFixture, now time.Time) (int, error) {
	e, err := s.ensureFotMobEvent(ctx, f)
	if err != nil {
		return 0, err
	}
	hasStats, err := s.store.HasMatchStats(ctx, e.ID)
	if err != nil || hasStats || !isDue(s.historyAttempts, e.ID, now, historyRetryInterval) {
		return 0, err
	}
	s.historyAttempts[e.ID] = now
	details, err := s.fotmob.MatchDetails(ctx, f.fixture.ID)
	if err != nil || !details.IsFinished() {
		return 0, err
	}
	return s.storeFotMobStats(ctx, e, details)
}

func (s *Service) ensureFotMobEvent(ctx context.Context, f leagueFixture) (ScheduledEvent, error) {
	homeID, err := s.fotmobTeam(ctx, f.leagueCode, f.fixture.HomeID, f.fixture.Home)
	if err != nil {
		return ScheduledEvent{}, err
	}
	awayID, err := s.fotmobTeam(ctx, f.leagueCode, f.fixture.AwayID, f.fixture.Away)
	if err != nil {
		return ScheduledEvent{}, err
	}
	eventID, err := s.entities.UpsertSourceEvent(ctx, seed.SourceEvent{
		Source: fotmob.SourceName, SourceID: f.fixture.ID, LeagueCode: f.leagueCode,
		HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: f.fixture.Kickoff, Status: seed.EventFinished,
	})
	if err != nil {
		return ScheduledEvent{}, err
	}
	fotmobID := f.fixture.ID
	return ScheduledEvent{
		ID: eventID, LeagueCode: f.leagueCode, Kickoff: f.fixture.Kickoff, FotmobID: &fotmobID,
		HomeTeamID: homeID, AwayTeamID: awayID, HomeTeamName: f.fixture.Home, AwayTeamName: f.fixture.Away,
	}, nil
}

func (s *Service) fotmobTeam(ctx context.Context, leagueCode, fotmobTeamID, name string) (string, error) {
	return s.entities.UpsertSourceTeam(ctx, seed.SourceTeam{
		Source: fotmob.SourceName, SourceID: fotmobTeamID, LeagueCode: leagueCode, Name: name, LogoURL: fotmob.TeamLogoURL(fotmobTeamID),
	})
}

func (s *Service) espnHistory(ctx context.Context, now time.Time) {
	teams, err := s.store.PropTeams(ctx, espn.SourceName, now, now.Add(historyHorizon))
	if err != nil {
		s.recordFailure("postgres", "history", "err", err)
		return
	}
	propTeams := map[string]map[string]bool{}
	for _, t := range teams {
		if _, ok := espn.Leagues[t.LeagueCode]; !ok {
			continue
		}
		if propTeams[t.LeagueCode] == nil {
			propTeams[t.LeagueCode] = map[string]bool{}
		}
		propTeams[t.LeagueCode][t.SourceID] = true
	}
	var pass historyPass
	leagues := slices.Sorted(maps.Keys(propTeams))
	for _, league := range leagues {
		pass.teams += len(propTeams[league])
		pass.fixtures += s.espnLeagueHistory(ctx, league, propTeams[league], now, &pass)
	}
	window := StatsWindow{
		From: now.Add(-historyESPNLookback), To: now.Add(-statsDelayAfterKickoff), Limit: historyESPNEventLimit,
		Skip: recentlyAttempted(s.historyAttempts, now, historyRetryInterval),
	}
	s.log.Info("espn history games", "source", espn.SourceName, "leagues", len(leagues), "teams", pass.teams, "games", pass.fixtures, "failures", pass.failures)
	s.collectSportStatsIn(ctx, now, window, leagues, s.historyAttempts)
}

func (s *Service) espnLeagueHistory(ctx context.Context, league string, propTeams map[string]bool, now time.Time, pass *historyPass) int {
	games := 0
	for day := now.Add(-historyESPNLookback).Truncate(24 * time.Hour); day.Before(now); day = day.Add(24 * time.Hour) {
		if ctx.Err() != nil {
			return games
		}
		key := league + ":" + day.Format(dayLayout)
		if s.historyDays[key] {
			continue
		}
		stored, err := s.espnDayHistory(ctx, league, day, propTeams)
		if err != nil {
			pass.failures++
			s.recordFailure(espn.SourceName, "history", "league", league, "day", day.Format(dayLayout), "err", err)
			continue
		}
		games += stored
		s.historyDays[key] = now.Sub(day) > historyESPNSettledAfter
	}
	return games
}

func (s *Service) espnDayHistory(ctx context.Context, league string, day time.Time, propTeams map[string]bool) (int, error) {
	board, err := s.espn.Games(ctx, league, day)
	if err != nil {
		return 0, err
	}
	stored := 0
	var errs []error
	for _, g := range board.Games {
		isFinal := seed.StatusFromESPN(g.Status) == seed.EventFinished
		involvesPropTeam := propTeams[seed.ESPNSourceID(league, g.Home.ID)] || propTeams[seed.ESPNSourceID(league, g.Away.ID)]
		if !isFinal || !involvesPropTeam {
			continue
		}
		if err := s.ensureESPNEvent(ctx, league, g); err != nil {
			errs = append(errs, err)
			continue
		}
		stored++
	}
	return stored, errors.Join(errs...)
}

func (s *Service) ensureESPNEvent(ctx context.Context, league string, g espn.Game) error {
	homeID, err := s.espnTeam(ctx, league, g.Home)
	if err != nil {
		return err
	}
	awayID, err := s.espnTeam(ctx, league, g.Away)
	if err != nil {
		return err
	}
	_, err = s.entities.UpsertSourceEvent(ctx, seed.SourceEvent{
		Source: espn.SourceName, SourceID: seed.ESPNSourceID(league, g.ID), LeagueCode: league,
		HomeTeamID: homeID, AwayTeamID: awayID, Kickoff: g.Kickoff, Status: seed.EventFinished,
	})
	return err
}

func (s *Service) espnTeam(ctx context.Context, league string, t espn.Team) (string, error) {
	return s.entities.UpsertSourceTeam(ctx, seed.SourceTeam{
		Source: espn.SourceName, SourceID: seed.ESPNSourceID(league, t.ID), LeagueCode: league, Name: t.Name, LogoURL: t.LogoURL,
	})
}
