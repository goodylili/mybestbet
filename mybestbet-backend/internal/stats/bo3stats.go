package stats

import (
	"context"
	"fmt"
	"strings"

	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/statkey"
)

var errSeriesNotFound = fmt.Errorf("%w: no bo3 series between the event teams", errStatsNotFinal)

type sideByTeamID map[string]eventSide

func (s *Service) bo3Lines(ctx context.Context, e SourcedEvent) (sportLines, error) {
	match, sides, err := s.bo3SeriesFor(ctx, e)
	if err != nil {
		return sportLines{}, err
	}
	if !match.IsFinished() {
		return sportLines{}, errStatsNotFinal
	}
	lines, err := s.bo3.PlayerLines(ctx, e.LeagueCode, match)
	if err != nil {
		return sportLines{}, err
	}
	sl := sportLines{playerSource: bo3.SourceName}
	for _, l := range lines {
		side := sides[l.TeamID]
		if side == sideUnknown {
			continue
		}
		sl.lines = append(sl.lines, sourceLine{
			sourcePlayerID: l.Player.ID, name: l.Player.Name, isHome: side == sideHome,
			values: statkey.Values{statkey.Kills: float64(l.Kills), statkey.Maps: float64(l.Maps)},
		})
	}
	if len(sl.lines) == 0 {
		return sportLines{}, errStatsNotFinal
	}
	return sl, nil
}

func (s *Service) bo3SeriesFor(ctx context.Context, e SourcedEvent) (bo3.Match, sideByTeamID, error) {
	if e.Source == bo3.SourceName {
		_, slug, found := strings.Cut(e.SourceID, ":")
		if !found {
			return bo3.Match{}, nil, fmt.Errorf("%s: %w", e.SourceID, errMalformedSourceID)
		}
		match, err := s.bo3.Match(ctx, slug)
		if err != nil {
			return bo3.Match{}, nil, err
		}
		return seriesWithResolvedSides(ctx, s.store, e, []bo3.Match{match})
	}
	candidates, err := s.bo3.Matches(ctx, e.LeagueCode, e.Kickoff.Add(-seriesGameLeadWindow), e.Kickoff.Add(esportsStatsDelay))
	if err != nil {
		return bo3.Match{}, nil, err
	}
	return seriesWithResolvedSides(ctx, s.store, e, candidates)
}

func seriesWithResolvedSides(ctx context.Context, store *Store, e SourcedEvent, candidates []bo3.Match) (bo3.Match, sideByTeamID, error) {
	teamIDs := make([]string, 0, 2*len(candidates))
	for _, m := range candidates {
		teamIDs = append(teamIDs, m.Team1.ID, m.Team2.ID)
	}
	linkedTeams, err := store.TeamIDsBySourceID(ctx, bo3.SourceName, teamIDs)
	if err != nil {
		return bo3.Match{}, nil, err
	}
	for _, m := range candidates {
		team1, team2 := sideOf(e, m.Team1.ID, m.Team1.Name, linkedTeams), sideOf(e, m.Team2.ID, m.Team2.Name, linkedTeams)
		if team1 != sideUnknown && team2 != sideUnknown && team1 != team2 {
			return m, sideByTeamID{m.Team1.ID: team1, m.Team2.ID: team2}, nil
		}
	}
	return bo3.Match{}, nil, fmt.Errorf("%s %s: %w", e.LeagueCode, e.SourceID, errSeriesNotFound)
}
