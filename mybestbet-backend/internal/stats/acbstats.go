package stats

import (
	"context"
	"fmt"
	"strconv"

	"mybestbet/internal/stats/acb"
)

func (s *Service) acbLines(ctx context.Context, e SourcedEvent) (sportLines, error) {
	matchID, err := strconv.ParseInt(e.SourceID, 10, 64)
	if err != nil {
		return sportLines{}, fmt.Errorf("%s: %w", e.SourceID, errMalformedSourceID)
	}
	box, err := s.acb.Boxscore(ctx, matchID)
	if err != nil {
		return sportLines{}, err
	}
	if !box.Finished {
		return sportLines{}, errStatsNotFinal
	}
	clubIDs := make([]string, 0, len(box.Lines))
	for _, l := range box.Lines {
		clubIDs = append(clubIDs, l.Club.ClubID)
	}
	linkedTeams, err := s.store.TeamIDsBySourceID(ctx, acb.SourceName, clubIDs)
	if err != nil {
		return sportLines{}, err
	}
	sl := sportLines{playerSource: acb.SourceName}
	for _, l := range box.Lines {
		side := sideOf(e, l.Club.ClubID, l.Club.Name, linkedTeams)
		if !l.Played || side == sideUnknown {
			continue
		}
		sl.lines = append(sl.lines, sourceLine{
			sourcePlayerID: l.Player.ID, name: l.Player.Name, isHome: side == sideHome,
			minutes: l.Minutes, started: l.Started, values: l.Values,
		})
	}
	return sl, nil
}
