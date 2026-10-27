package stats

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"mybestbet/internal/names"
	"mybestbet/internal/stats/flashscore"
)

const unlinkedKickoffTolerance = 15 * time.Minute

func (c *LiveScoreCollector) unlinkedFootballScores(ctx context.Context, events []SourcedEvent, now time.Time) ([]liveScore, error) {
	byID := map[string]flashscore.FootballMatch{}
	var errs []error
	for _, offset := range flashscoreLiveDayOffsets {
		matches, err := c.flashscore.StartedFootballMatches(ctx, offset, now)
		errs = append(errs, err)
		for _, m := range matches {
			byID[m.ID] = m
		}
	}
	var scores []liveScore
	for _, e := range events {
		if m, found := footballMatchFor(e, byID); found {
			scores = append(scores, liveScore{event: e, score: m.Score, finished: m.Finished})
		}
	}
	return scores, errors.Join(errs...)
}

func footballMatchFor(e SourcedEvent, matches map[string]flashscore.FootballMatch) (flashscore.FootballMatch, bool) {
	var candidates []flashscore.FootballMatch
	for _, m := range matches {
		if kickoffGap(m, e) <= unlinkedKickoffTolerance && sidesAgree(m, e) {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return flashscore.FootballMatch{}, false
	}
	return slices.MinFunc(candidates, func(a, b flashscore.FootballMatch) int {
		return cmp.Or(cmp.Compare(kickoffGap(a, e), kickoffGap(b, e)), cmp.Compare(a.ID, b.ID))
	}), true
}

func kickoffGap(m flashscore.FootballMatch, e SourcedEvent) time.Duration {
	return m.Kickoff.Sub(e.Kickoff).Abs()
}

func sidesAgree(m flashscore.FootballMatch, e SourcedEvent) bool {
	home, away := teamNameRank(m.Home, e.HomeTeamName), teamNameRank(m.Away, e.AwayTeamName)
	flipped := teamNameRank(m.Home, e.AwayTeamName) + teamNameRank(m.Away, e.HomeTeamName)
	return home > 0 && away > 0 && home+away > flipped
}

func teamNameRank(a, b string) int {
	switch {
	case !names.TeamsLikelySame(a, b) || !names.HaveSameSquadQualifiers(names.Team(a), names.Team(b)):
		return 0
	case names.Team(a) == names.Team(b):
		return 2
	}
	return 1
}
