package resolve

import (
	"slices"
	"strings"

	"mybestbet/internal/names"
	"mybestbet/internal/review"
	"mybestbet/pkg/canon"
)

const (
	playerAutoLinkScore       = 0.90
	surnameInitialScore       = 0.95
	exactPlayerNameScore      = 1.0
	namesContainedScore       = 0.93
	unanchoredNameScoreCap    = 0.85
	similarNameTokenScore     = 0.88
	minPrefixNameTokenLength  = 3
	minContainedNameTokenSize = 2
)

func playerSourceKey(p canon.SourcePlayer) string {
	if p.SourceID != "" {
		return p.SourceID
	}
	return p.Name
}

func (c *Cache) matchPlayer(source string, p canon.SourcePlayer, ev Event) match {
	sourceKey := playerSourceKey(p)
	if id, ok := c.playerAlias(source, sourceKey); ok {
		return exactAliasMatch(id)
	}
	squad := c.squadsOf(ev.HomeTeamID, ev.AwayTeamID)
	var m match
	if c.sportOf(ev.League) == canon.SportEsports {
		m = matchPlayerByHandle(squad, p.Name)
	} else {
		m = matchPlayerByName(squad, names.Person(p.Name))
	}
	if m.outcome == matchAutoLinked && c.isClaimedByAnotherSourcePlayer(source, m.id, sourceKey) {
		return c.contestedMatch(m)
	}
	return m
}

func matchPlayerByHandle(squad []squadPlayer, rawName string) match {
	handle := names.Handle(rawName)
	if found, ok := uniquePlayer(squad, func(sp squadPlayer) bool { return handle != "" && names.Handle(sp.Name) == handle }); ok {
		return match{id: found.ID, score: exactPlayerNameScore, outcome: matchAutoLinked}
	}
	return match{outcome: matchNeedsReview, candidates: topCandidates(rankSquad(squad, names.Person(rawName)))}
}

func matchPlayerByName(squad []squadPlayer, name names.PersonName) match {
	if found, ok := uniquePlayer(squad, func(sp squadPlayer) bool { return hasSameFullName(sp.name, name) }); ok {
		return match{id: found.ID, score: exactPlayerNameScore, outcome: matchAutoLinked}
	}
	if found, ok := uniquePlayer(squad, func(sp squadPlayer) bool { return hasSameSurnameAndInitial(sp.name, name) }); ok {
		return match{id: found.ID, score: surnameInitialScore, outcome: matchAutoLinked}
	}
	if found, ok := uniquePlayer(squad, func(sp squadPlayer) bool { return isShorterNameContained(sp.name, name) }); ok {
		return match{id: found.ID, score: namesContainedScore, outcome: matchAutoLinked}
	}
	return decideByScore(rankSquad(squad, name), playerAutoLinkScore)
}

func (c *Cache) contestedMatch(m match) match {
	name := ""
	if p, ok := c.player(m.id); ok {
		name = p.Name
	}
	return match{outcome: matchNeedsReview, candidates: []review.Candidate{{ID: m.id, Name: name, Score: m.score}}}
}

func (c *Cache) isClaimedByAnotherSourcePlayer(source, playerID, sourceKey string) bool {
	claimant, claimed := c.playerClaimant(source, playerID)
	if !claimed || claimant == sourceKey {
		return false
	}
	return !isShortFormOf(names.Person(claimant), names.Person(sourceKey))
}

func isShortFormOf(a, b names.PersonName) bool {
	shorter, longer := a, b
	if len(shorter.Tokens()) > len(longer.Tokens()) {
		shorter, longer = longer, shorter
	}
	return len(shorter.Tokens()) < len(longer.Tokens()) && isEveryTokenIn(shorter.Tokens(), longer.Tokens())
}

func uniquePlayer(squad []squadPlayer, matches func(squadPlayer) bool) (squadPlayer, bool) {
	var found []squadPlayer
	for _, sp := range squad {
		if matches(sp) && !slices.ContainsFunc(found, func(f squadPlayer) bool { return f.ID == sp.ID }) {
			found = append(found, sp)
		}
	}
	if len(found) != 1 {
		return squadPlayer{}, false
	}
	return found[0], true
}

func hasSameFullName(candidate, source names.PersonName) bool {
	return candidate.Full() == source.Full() || candidate.SortedFull() == source.SortedFull()
}

func hasSameSurnameAndInitial(candidate, source names.PersonName) bool {
	if source.IsMononym() {
		return candidate.Surname() == source.Full()
	}
	key := candidate.SurnameInitialKey()
	switch {
	case key == "":
		return false
	case key == source.SurnameInitialKey():
		return areCompatibleGivenNames(candidate.Given(), source.Given())
	case key == source.ReversedSurnameInitialKey():
		return areCompatibleGivenNames(candidate.Given(), source.Surname())
	}
	return false
}

func areCompatibleGivenNames(a, b string) bool {
	return names.IsInitial(a) || names.IsInitial(b) || areSimilarNameTokens(a, b)
}

func isShorterNameContained(candidate, source names.PersonName) bool {
	shorter, longer := candidate, source
	if len(shorter.Tokens()) > len(longer.Tokens()) {
		shorter, longer = longer, shorter
	}
	return len(shorter.Tokens()) >= minContainedNameTokenSize && isEveryTokenIn(shorter.Tokens(), longer.Tokens())
}

func isEveryTokenIn(tokens, pool []string) bool {
	for _, t := range tokens {
		if !hasSimilarToken(pool, t) {
			return false
		}
	}
	return true
}

func hasSimilarToken(pool []string, token string) bool {
	for _, candidate := range pool {
		if areSimilarNameTokens(candidate, token) {
			return true
		}
	}
	return false
}

func areSimilarNameTokens(a, b string) bool {
	shorter, longer := a, b
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) >= minPrefixNameTokenLength && strings.HasPrefix(longer, shorter) {
		return true
	}
	return jaroWinkler(a, b) >= similarNameTokenScore
}

func sharesSurnameAndGivenName(a, b names.PersonName) bool {
	sharesSurname := hasSimilarToken(b.Tokens(), a.Surname()) || hasSimilarToken(a.Tokens(), b.Surname())
	sharesGivenName := areCompatibleGivenNames(a.Given(), b.Given()) || hasSimilarToken(b.Tokens(), a.Given()) || hasSimilarToken(a.Tokens(), b.Given())
	return sharesSurname && sharesGivenName
}

func rankSquad(squad []squadPlayer, name names.PersonName) []scoredCandidate {
	bestByPlayer := map[string]scoredCandidate{}
	for _, sp := range squad {
		score := personSimilarity(sp.name, name)
		if !sharesSurnameAndGivenName(sp.name, name) {
			score = min(score, unanchoredNameScoreCap)
		}
		if best, seen := bestByPlayer[sp.ID]; !seen || score > best.score {
			bestByPlayer[sp.ID] = scoredCandidate{id: sp.ID, name: sp.Name, score: score}
		}
	}
	ranked := make([]scoredCandidate, 0, len(bestByPlayer))
	for _, c := range bestByPlayer {
		ranked = append(ranked, c)
	}
	slices.SortFunc(ranked, func(a, b scoredCandidate) int { return strings.Compare(a.id, b.id) })
	sortByScoreDesc(ranked)
	return ranked
}
