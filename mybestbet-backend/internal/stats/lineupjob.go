package stats

import (
	"context"
	"time"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/internal/stats/sofascore"
	"mybestbet/pkg/canon"
)

type sidedPlayer struct {
	player sofascore.Player
	isHome bool
}

func (s *Service) pollLineups(ctx context.Context, now time.Time) {
	events, err := s.store.EventsAwaitingLineups(ctx, now.Add(-lineupTrailTime), now.Add(lineupLeadTime))
	if err != nil {
		s.recordFailure("postgres", "lineups", "err", err)
		return
	}
	for _, e := range events {
		if !isDue(s.lastLineupPoll, e.ID, now, lineupPollInterval) {
			continue
		}
		s.lastLineupPoll[e.ID] = now
		itemCtx, cancel := context.WithTimeout(ctx, jobItemBudget)
		s.refreshLineup(itemCtx, e, now)
		cancel()
	}
}

func (s *Service) refreshLineup(ctx context.Context, e ScheduledEvent, now time.Time) {
	if s.isEnabled(sofascore.SourceName) {
		isWritten, err := s.lineupFromSofascore(ctx, e)
		if err != nil {
			s.recordFailure(sofascore.SourceName, "lineups", "event_id", e.ID, "err", err)
		}
		if isWritten || e.Kickoff.Sub(now) > sofascoreLineupGrace {
			return
		}
	}
	if !s.isEnabled(fotmob.SourceName) {
		return
	}
	if err := s.lineupFromFotMob(ctx, e); err != nil {
		s.recordFailure(fotmob.SourceName, "lineups", "event_id", e.ID, "err", err)
	}
}

func (s *Service) lineupFromSofascore(ctx context.Context, e ScheduledEvent) (bool, error) {
	if e.SofascoreID == nil {
		return false, nil
	}
	lineups, err := s.sofascore.Lineups(ctx, *e.SofascoreID)
	if err != nil || !lineups.Confirmed {
		return false, err
	}
	entries := lineups.Entries()
	sided := make([]sidedPlayer, len(entries))
	for i, entry := range entries {
		sided[i] = sidedPlayer{player: entry.Player, isHome: entry.IsHome}
	}
	playerIDs, err := s.ensureSofascorePlayers(ctx, e, sided)
	if err != nil {
		return false, err
	}
	rows := make([]LineupRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, LineupRow{PlayerID: playerIDs[entry.Player.ID], Status: lineupStatusOf(entry.Substitute, entry.Missing)})
	}
	if err := s.store.UpsertLineup(ctx, e.ID, rows); err != nil {
		return false, err
	}
	s.log.Info("lineup stored", "source", sofascore.SourceName, "event_id", e.ID, "players", len(rows))
	return true, nil
}

func (s *Service) lineupFromFotMob(ctx context.Context, e ScheduledEvent) error {
	details, isFound, err := s.fotmobDetails(ctx, e)
	if err != nil || !isFound || !details.IsLineupConfirmed() {
		return err
	}
	entries := details.LineupEntries()
	refs := make([]fotmobPlayerRef, len(entries))
	for i, entry := range entries {
		refs[i] = fotmobPlayerRef{id: entry.FotmobID, name: entry.Name, isHome: entry.IsHome}
	}
	playerIDs, err := s.fotmobPlayerIDs(ctx, e, refs)
	if err != nil {
		return err
	}
	rows := make([]LineupRow, 0, len(entries))
	for _, entry := range entries {
		if playerID, ok := playerIDs[entry.FotmobID]; ok {
			rows = append(rows, LineupRow{PlayerID: playerID, Status: lineupStatusOf(entry.Substitute, entry.Missing)})
		}
	}
	if err := s.store.UpsertLineup(ctx, e.ID, rows); err != nil {
		return err
	}
	s.log.Info("lineup stored", "source", fotmob.SourceName, "event_id", e.ID, "players", len(rows))
	return nil
}

func lineupStatusOf(isSubstitute, isMissing bool) LineupStatus {
	switch {
	case isMissing:
		return LineupOut
	case isSubstitute:
		return LineupBench
	}
	return LineupStarter
}

func (s *Service) ensureSofascorePlayers(ctx context.Context, e ScheduledEvent, players []sidedPlayer) (map[int64]string, error) {
	sofascoreIDs := make([]int64, len(players))
	for i, p := range players {
		sofascoreIDs[i] = p.player.ID
	}
	known, err := s.store.PlayerIDsBySofascore(ctx, sofascoreIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range players {
		if _, ok := known[p.player.ID]; ok {
			continue
		}
		teamID := e.AwayTeamID
		if p.isHome {
			teamID = e.HomeTeamID
		}
		playerID, err := s.entities.UpsertPlayer(ctx, seed.Player{SofascoreID: p.player.ID, TeamID: teamID, Name: p.player.Name, Position: seed.NormalizePosition(canon.SportFootball, p.player.Position)})
		if err != nil {
			return nil, err
		}
		known[p.player.ID] = playerID
	}
	return known, nil
}

func (s *Service) fotmobDetails(ctx context.Context, e ScheduledEvent) (fotmob.Details, bool, error) {
	if e.FotmobID != nil {
		details, err := s.fotmob.MatchDetails(ctx, *e.FotmobID)
		return details, err == nil, err
	}
	matches, err := s.fotmob.LeagueMatches(ctx, e.LeagueCode)
	if err != nil {
		return fotmob.Details{}, false, err
	}
	match, ok := fotmob.FindMatch(matches, e.Kickoff, e.HomeTeamName, e.AwayTeamName)
	if !ok {
		return fotmob.Details{}, false, nil
	}
	details, err := s.fotmob.MatchDetails(ctx, match.ID)
	return details, err == nil, err
}

func (s *Service) squads(ctx context.Context, e ScheduledEvent) (home, away []NamedPlayer, err error) {
	if home, err = s.store.TeamPlayers(ctx, e.HomeTeamID); err != nil {
		return nil, nil, err
	}
	if away, err = s.store.TeamPlayers(ctx, e.AwayTeamID); err != nil {
		return nil, nil, err
	}
	return home, away, nil
}

func sideSquad(isHome bool, home, away []NamedPlayer, e ScheduledEvent) ([]NamedPlayer, string) {
	if isHome {
		return home, e.HomeTeamName
	}
	return away, e.AwayTeamName
}
