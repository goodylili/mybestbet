package resolve

import (
	"cmp"
	"slices"

	"mybestbet/internal/review"
)

type matchOutcome int

const (
	matchNeedsReview matchOutcome = iota
	matchExactAlias
	matchAutoLinked
	matchFixtureOpponent
)

type match struct {
	id         string
	score      float64
	outcome    matchOutcome
	candidates []review.Candidate
}

func exactAliasMatch(id string) match {
	return match{id: id, score: 1, outcome: matchExactAlias}
}

func (m match) isSettled() bool { return m.outcome == matchExactAlias || m.outcome == matchAutoLinked }

func decideByScore(ranked []scoredCandidate, autoLinkScore float64) match {
	if isClearWinner(ranked, autoLinkScore) {
		return match{id: ranked[0].id, score: ranked[0].score, outcome: matchAutoLinked}
	}
	return match{outcome: matchNeedsReview, candidates: topCandidates(ranked)}
}

func sortByScoreDesc(ranked []scoredCandidate) {
	slices.SortStableFunc(ranked, func(a, b scoredCandidate) int { return cmp.Compare(b.score, a.score) })
}

func topCandidates(ranked []scoredCandidate) []review.Candidate {
	top := ranked[:min(len(ranked), reviewCandidateCount)]
	candidates := make([]review.Candidate, len(top))
	for i, c := range top {
		candidates[i] = review.Candidate{ID: c.id, Name: c.name, Score: c.score}
	}
	return candidates
}
