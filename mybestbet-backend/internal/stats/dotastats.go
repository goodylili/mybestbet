package stats

import (
	"context"

	"mybestbet/internal/names"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/statkey"
)

type eventSide int

const (
	sideUnknown eventSide = iota
	sideHome
	sideAway
)

type dotaSeriesGame struct {
	esports.ProGame
	radiantIsHome bool
}

func (s *Service) dotaLines(ctx context.Context, e SourcedEvent) (sportLines, error) {
	series, err := s.dotaSeriesGames(ctx, e)
	if err != nil {
		return sportLines{}, err
	}
	if len(series) == 0 {
		return sportLines{}, errStatsNotFinal
	}
	byAccount := map[string]*sourceLine{}
	var order []string
	for _, g := range series {
		detail, err := s.opendota.Game(ctx, g.MatchID)
		if err != nil {
			return sportLines{}, err
		}
		for _, p := range detail.Players {
			line, seen := byAccount[p.AccountID]
			if !seen {
				line = &sourceLine{sourcePlayerID: p.AccountID, name: p.Name, isHome: p.IsRadiant == g.radiantIsHome, values: statkey.Zeroed([]statkey.Key{statkey.Kills, statkey.Maps})}
				byAccount[p.AccountID] = line
				order = append(order, p.AccountID)
			}
			line.values[statkey.Kills] += float64(p.Kills)
			line.values[statkey.Maps]++
		}
	}
	sl := sportLines{playerSource: esports.OpenDotaSource}
	for _, account := range order {
		sl.lines = append(sl.lines, *byAccount[account])
	}
	return sl, nil
}

func (s *Service) dotaSeriesGames(ctx context.Context, e SourcedEvent) ([]dotaSeriesGame, error) {
	games, err := s.opendota.ProGames(ctx)
	if err != nil {
		return nil, err
	}
	return dotaSeriesGamesOf(ctx, s.store, e, games)
}

func dotaSeriesGamesOf(ctx context.Context, store *Store, e SourcedEvent, games []esports.ProGame) ([]dotaSeriesGame, error) {
	teamIDs := make([]string, 0, 2*len(games))
	for _, g := range games {
		teamIDs = append(teamIDs, g.Radiant.ID, g.Dire.ID)
	}
	linkedTeams, err := store.TeamIDsBySourceID(ctx, esports.OpenDotaSource, teamIDs)
	if err != nil {
		return nil, err
	}
	from, until := e.Kickoff.Add(-seriesGameLeadWindow), e.Kickoff.Add(esportsStatsDelay)
	var series []dotaSeriesGame
	for _, g := range games {
		if g.Start.Before(from) || g.Start.After(until) {
			continue
		}
		radiant, dire := sideOf(e, g.Radiant.ID, g.Radiant.Name, linkedTeams), sideOf(e, g.Dire.ID, g.Dire.Name, linkedTeams)
		if radiant == sideUnknown || dire == sideUnknown || radiant == dire {
			continue
		}
		series = append(series, dotaSeriesGame{ProGame: g, radiantIsHome: radiant == sideHome})
	}
	return series, nil
}

func sideOf(e SourcedEvent, sourceTeamID, teamName string, linkedTeams map[string]string) eventSide {
	switch teamID := linkedTeams[sourceTeamID]; {
	case teamID == e.HomeTeamID:
		return sideHome
	case teamID == e.AwayTeamID:
		return sideAway
	case names.TeamsLikelySame(teamName, e.HomeTeamName):
		return sideHome
	case names.TeamsLikelySame(teamName, e.AwayTeamName):
		return sideAway
	}
	return sideUnknown
}
