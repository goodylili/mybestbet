package stats

import (
	"context"

	"mybestbet/internal/stats/fbref"
)

const exactNameAliasConfidence = 1.0

func (s *Service) refreshSeasonStats(ctx context.Context) {
	aliases, err := s.store.PlayerAliases(ctx, fbref.SourceName)
	if err != nil {
		s.recordFailure("postgres", "season_stats", "err", err)
		return
	}
	for _, league := range s.leagues {
		itemCtx, cancel := context.WithTimeout(ctx, jobItemBudget)
		if err := s.refreshLeagueSeason(itemCtx, league, aliases); err != nil {
			s.recordFailure(fbref.SourceName, "season_stats", "league", league, "err", err)
		}
		cancel()
	}
}

func (s *Service) refreshLeagueSeason(ctx context.Context, league string, aliases map[string]string) error {
	seasons, err := s.fbref.LeagueSeason(ctx, league)
	if err != nil {
		return err
	}
	candidates, err := s.store.LeaguePlayers(ctx, league)
	if err != nil {
		return err
	}
	stored := 0
	for _, p := range seasons {
		playerID, err := s.fbrefPlayerID(ctx, p, candidates, aliases)
		if err != nil {
			return err
		}
		if playerID == "" {
			continue
		}
		if err := s.store.UpsertSeasonStat(ctx, playerID, p.FBrefID, seasonStatFrom(league, p)); err != nil {
			return err
		}
		stored++
	}
	s.log.Info("season stats stored", "source", fbref.SourceName, "league", league, "players", stored, "unmatched", len(seasons)-stored)
	return nil
}

func (s *Service) fbrefPlayerID(ctx context.Context, p fbref.PlayerSeason, candidates []NamedPlayer, aliases map[string]string) (string, error) {
	if id, ok := aliases[p.FBrefID]; ok {
		return id, nil
	}
	match, ok := matchPlayerByName(candidates, p.Name, p.Team)
	if !ok || !match.isExact {
		return "", nil
	}
	if err := s.store.LinkPlayerAlias(ctx, fbref.SourceName, p.FBrefID, match.playerID, exactNameAliasConfidence); err != nil {
		return "", err
	}
	aliases[p.FBrefID] = match.playerID
	return match.playerID, nil
}

func seasonStatFrom(league string, p fbref.PlayerSeason) SeasonStat {
	return SeasonStat{
		LeagueCode: league, Season: p.Season, Source: fbref.SourceName, Team: p.Team,
		Matches: p.Matches, Starts: p.Starts, Minutes: p.Minutes, Goals: p.Goals, Assists: p.Assists,
		Shots: p.Shots, ShotsOnTarget: p.ShotsOnTarget, Fouls: p.Fouls, Tackles: p.Tackles, Yellow: p.Yellow, Red: p.Red,
		GoalsPer90: p.Per90(p.Goals), AssistsPer90: p.Per90(p.Assists),
		ShotsPer90: p.Per90(p.Shots), ShotsOnTargetPer90: p.Per90(p.ShotsOnTarget),
	}
}
