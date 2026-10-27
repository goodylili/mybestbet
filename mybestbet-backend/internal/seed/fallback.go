package seed

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"mybestbet/internal/stats/fotmob"
	"mybestbet/pkg/canon"
)

var undecidedSlotName = regexp.MustCompile(`(?i)\b(ranked|winners?|losers?|tbd|tba|runners?-up)\b`)

type fotmobTeam struct {
	id   string
	name string
}

func (s *Service) seedFromFotMob(ctx context.Context, league League) error {
	matches, err := s.fallback.LeagueMatches(ctx, league.Code)
	if err != nil {
		return fmt.Errorf("league %s fotmob fallback: %w", league.Code, err)
	}
	matches = decidedMatches(matches)
	teams := distinctTeams(matches)
	adoption, creation, err := s.fotmobTeamPolicy(ctx, league, len(teams))
	if err != nil {
		return err
	}
	teamIDs := map[string]string{}
	var errs []error
	for _, t := range teams {
		id, err := s.store.upsertSourceTeam(ctx, SourceTeam{Source: fotmob.SourceName, SourceID: t.id, LeagueCode: league.Code, Name: t.name, LogoURL: fotmob.TeamLogoURL(t.id)}, adoption, creation)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if id == "" {
			continue
		}
		teamIDs[t.id] = id
		errs = append(errs, s.seedFotMobSquadIfEmpty(ctx, id, t.id))
	}
	upcoming := upcomingMatches(matches, s.now(), s.now().Add(UpcomingHorizon))
	for _, m := range upcoming {
		home, away := teamIDs[m.HomeID], teamIDs[m.AwayID]
		if home == "" || away == "" {
			continue
		}
		_, err := s.store.UpsertSourceEvent(ctx, SourceEvent{
			Source: fotmob.SourceName, SourceID: m.ID, LeagueCode: league.Code,
			HomeTeamID: home, AwayTeamID: away, Kickoff: m.Kickoff, Status: EventScheduled,
		})
		errs = append(errs, err)
	}
	s.log.Info("league seeded", "source", fotmob.SourceName, "league", league.Code, "teams", len(teamIDs), "events", len(upcoming))
	return errors.Join(errs...)
}

func (s *Service) fotmobTeamPolicy(ctx context.Context, league League, listedTeams int) (teamAdoption, teamCreation, error) {
	if league.Format() != DomesticLeague {
		return adoptByIDOrAlias, createMissingTeam, nil
	}
	seeded, err := s.store.CountSofascoreTeams(ctx, league.Code)
	if err != nil {
		return 0, false, err
	}
	return adoptByLeagueMemberName, teamCreation(seeded < listedTeams), nil
}

func (s *Service) seedFotMobSquadIfEmpty(ctx context.Context, teamID, fotmobTeamID string) error {
	hasSquad, err := s.store.HasPlayers(ctx, teamID)
	if err != nil || hasSquad {
		return err
	}
	squad, err := s.fallback.TeamSquad(ctx, fotmobTeamID)
	if err != nil {
		return fmt.Errorf("fotmob squad %s: %w", fotmobTeamID, err)
	}
	var errs []error
	for _, p := range squad {
		_, err := s.store.UpsertSourcePlayer(ctx, SourcePlayer{
			Source: fotmob.SourceName, SourceID: p.ID, TeamID: teamID, Name: p.Name,
			Position: NormalizePosition(canon.SportFootball, p.Position), ClubSourceID: p.ClubID,
		})
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func decidedMatches(matches []fotmob.Match) []fotmob.Match {
	decided := make([]fotmob.Match, 0, len(matches))
	for _, m := range matches {
		if isDecidedTeam(m.HomeID, m.Home) && isDecidedTeam(m.AwayID, m.Away) {
			decided = append(decided, m)
		}
	}
	return decided
}

func isDecidedTeam(id, name string) bool {
	return id != "" && name != "" && !undecidedSlotName.MatchString(name)
}

func distinctTeams(matches []fotmob.Match) []fotmobTeam {
	seen := map[string]bool{}
	var teams []fotmobTeam
	for _, m := range matches {
		for _, t := range []fotmobTeam{{id: m.HomeID, name: m.Home}, {id: m.AwayID, name: m.Away}} {
			if !seen[t.id] {
				seen[t.id] = true
				teams = append(teams, t)
			}
		}
	}
	return teams
}

func upcomingMatches(matches []fotmob.Match, from, until time.Time) []fotmob.Match {
	var upcoming []fotmob.Match
	for _, m := range matches {
		if !m.Finished && m.Kickoff.After(from) && m.Kickoff.Before(until) {
			upcoming = append(upcoming, m)
		}
	}
	return upcoming
}

func (s *Store) CountSofascoreTeams(ctx context.Context, leagueCode string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM teams t
		WHERE t."sofascoreId" IS NOT NULL AND (t."leagueCode" = $1 OR EXISTS (SELECT 1 FROM team_leagues m WHERE m."teamId" = t.id AND m."leagueCode" = $1))`,
		leagueCode).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count sofascore teams %s: %w", leagueCode, err)
	}
	return n, nil
}

func (s *Store) HasPlayers(ctx context.Context, teamID string) (bool, error) {
	var has bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM players WHERE "teamId" = $1) OR EXISTS (SELECT 1 FROM player_teams WHERE "teamId" = $1)`, teamID).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("team %s has players: %w", teamID, err)
	}
	return has, nil
}
