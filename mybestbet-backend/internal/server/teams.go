package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/logos"
)

func (s *Server) getTeam(w http.ResponseWriter, r *http.Request) {
	id, err := uuidPathValue(r, "id")
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	team, err := s.teamByID(r.Context(), id)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (s *Server) teamByID(ctx context.Context, id string) (TeamDetail, error) {
	var team TeamDetail
	var homeLeague string
	var logoURLs map[string]string
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		err := s.pool.QueryRow(groupCtx, `SELECT t.id::text, t.name, t."leagueCode",
				ARRAY(SELECT "leagueCode" FROM team_leagues WHERE "teamId" = t.id UNION SELECT t."leagueCode" ORDER BY 1)
			FROM teams t WHERE t.id = $1`, id).Scan(&team.ID, &team.Name, &homeLeague, &team.LeagueCodes)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load team: %w", err)
		}
		return nil
	})
	group.Go(func() (err error) {
		logoURLs, err = s.logoURLs(groupCtx, logos.KindTeam, []string{id})
		return err
	})
	if err := group.Wait(); err != nil {
		return TeamDetail{}, err
	}
	team.Sport = s.sportOf(homeLeague)
	team.LogoURL = logoURLs[team.ID]
	return team, nil
}

func (s *Server) teamsByID(ctx context.Context, ids []string) (map[string]Team, error) {
	if len(ids) == 0 {
		return map[string]Team{}, nil
	}
	var teams []Team
	var logoURLs map[string]string
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		rows, err := s.pool.Query(groupCtx, `SELECT id::text, name FROM teams WHERE id = ANY($1::uuid[])`, ids)
		if err != nil {
			return fmt.Errorf("list teams: %w", err)
		}
		teams, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Team, error) {
			var t Team
			err := row.Scan(&t.ID, &t.Name)
			return t, err
		})
		if err != nil {
			return fmt.Errorf("scan teams: %w", err)
		}
		return nil
	})
	group.Go(func() (err error) {
		logoURLs, err = s.logoURLs(groupCtx, logos.KindTeam, ids)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}
	byID := make(map[string]Team, len(teams))
	for _, t := range teams {
		t.LogoURL = logoURLs[t.ID]
		byID[t.ID] = t
	}
	return byID, nil
}

func (s *Server) withOpponents(ctx context.Context, matches []PlayerMatchStat) ([]PlayerMatchStat, error) {
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		if m.OpponentTeamID != "" {
			ids = append(ids, m.OpponentTeamID)
		}
	}
	teams, err := s.teamsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	return namedOpponents(matches, teams), nil
}

func namedOpponents(matches []PlayerMatchStat, teams map[string]Team) []PlayerMatchStat {
	for i := range matches {
		opponent := teams[matches[i].OpponentTeamID]
		matches[i].OpponentTeamName, matches[i].OpponentLogoURL = opponent.Name, opponent.LogoURL
	}
	return matches
}
