package stats

import (
	"context"
	"strconv"

	"mybestbet/internal/stats/fotmob"
)

const exactFotMobNameConfidence = 0.95

type fotmobPlayerRef struct {
	id     int64
	name   string
	isHome bool
}

func (s *Service) fotmobPlayerIDs(ctx context.Context, e ScheduledEvent, refs []fotmobPlayerRef) (map[int64]string, error) {
	sourceIDs := make([]string, len(refs))
	for i, r := range refs {
		sourceIDs[i] = strconv.FormatInt(r.id, 10)
	}
	known, err := s.store.PlayerIDsBySourceID(ctx, fotmob.SourceName, sourceIDs)
	if err != nil {
		return nil, err
	}
	home, away, err := s.squads(ctx, e)
	if err != nil {
		return nil, err
	}
	ids := make(map[int64]string, len(refs))
	for i, r := range refs {
		if playerID, ok := known[sourceIDs[i]]; ok {
			ids[r.id] = playerID
			continue
		}
		squad, teamName := sideSquad(r.isHome, home, away, e)
		match, ok := matchPlayerByName(squad, r.name, teamName)
		if !ok {
			s.log.Debug("fotmob player unmatched", "source", fotmob.SourceName, "event_id", e.ID, "player", r.name)
			continue
		}
		ids[r.id] = match.playerID
		if !match.isExact {
			continue
		}
		if err := s.store.LinkPlayerAlias(ctx, fotmob.SourceName, sourceIDs[i], match.playerID, exactFotMobNameConfidence); err != nil {
			return nil, err
		}
	}
	return ids, nil
}
