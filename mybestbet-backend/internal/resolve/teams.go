package resolve

import (
	"slices"
	"strings"
	"time"

	"mybestbet/internal/names"
	"mybestbet/internal/review"
	"mybestbet/pkg/canon"
)

const (
	teamAutoLinkScore          = 0.93
	newCompetitorScoreCeiling  = 0.80
	minNewCompetitorNameTokens = 2
	doublesPairSeparators      = "/&+"
	fixtureOpponentFloor       = 0.6
	womenQualifier             = "women"
)

func (c *Cache) matchTeam(source, league, rawName string) match {
	sport := c.sportOf(league)
	if id, ok := c.teamAlias(source, rawName); ok && c.isEligibleTeam(id, league, sport) {
		return exactAliasMatch(id)
	}
	if sport == canon.SportTennis {
		return c.matchCompetitor(rawName)
	}
	return decideByScore(c.rankLeagueTeams(league, rawName), teamAutoLinkScore)
}

func (c *Cache) isEligibleTeam(teamID, league string, sport canon.Sport) bool {
	if sport == canon.SportTennis {
		return c.isSportMember(teamID, sport)
	}
	return c.isLeagueMember(teamID, league)
}

func (c *Cache) rankLeagueTeams(league, rawName string) []scoredCandidate {
	normalized, literal := c.leagueQuery(league, rawName)
	isCollege := c.isCollegeLeague(league)
	bestByTeam := map[string]float64{}
	for _, name := range c.leagueNames(league) {
		bestByTeam[name.teamID] = max(bestByTeam[name.teamID], nameScore(normalized, literal, name, isCollege))
	}
	ranked := make([]scoredCandidate, 0, len(bestByTeam))
	for id, score := range bestByTeam {
		team, _ := c.team(id)
		ranked = append(ranked, scoredCandidate{id: id, name: team.Name, score: score})
	}
	sortByScoreDesc(ranked)
	return ranked
}

func (c *Cache) leagueQuery(league, rawName string) (string, string) {
	if !hasToken(names.Team(rawName), womenQualifier) && c.isWomenOnlyLeague(league) {
		rawName += " " + womenQualifier
	}
	return names.Team(rawName), names.TeamLiteral(rawName)
}

func nameScore(normalized, literal string, name teamName, isCollege bool) float64 {
	return max(teamSimilarity(normalized, name.normalized, !isCollege), teamSimilarity(literal, name.literal, !isCollege))
}

func hasToken(normalized, token string) bool {
	return slices.Contains(strings.Fields(normalized), token)
}

func (c *Cache) isWomenOnlyLeague(league string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	teamIDs := c.leagueTeams[league]
	if len(teamIDs) == 0 {
		return false
	}
	for _, id := range teamIDs {
		isWomen := slices.ContainsFunc(c.teamNames[id], func(n teamName) bool { return hasToken(n.normalized, womenQualifier) })
		if !isWomen {
			return false
		}
	}
	return true
}

func (c *Cache) inferFixtureOpponent(league string, kickoff time.Time, anchorID, rawName string, unresolved match) match {
	if c.isCollegeLeague(league) {
		return unresolved
	}
	opponentID, found := c.uniqueFixtureOpponent(league, kickoff, anchorID)
	if !found {
		return unresolved
	}
	normalized, literal := c.leagueQuery(league, rawName)
	score := 0.0
	for _, name := range c.namesOf(opponentID) {
		score = max(score, nameScore(normalized, literal, name, false))
	}
	if score < fixtureOpponentFloor || c.hasStrongerRival(league, rawName, opponentID, score) {
		return unresolved
	}
	return match{id: opponentID, score: score, outcome: matchFixtureOpponent}
}

func (c *Cache) uniqueFixtureOpponent(league string, kickoff time.Time, anchorID string) (string, bool) {
	tolerance := c.kickoffToleranceIn(league)
	opponentID := ""
	for _, e := range c.eventsOf(anchorID) {
		if e.League != league || absDuration(e.Kickoff.Sub(kickoff)) > tolerance {
			continue
		}
		other := e.opponentOf(anchorID)
		if opponentID != "" && opponentID != other {
			return "", false
		}
		opponentID = other
	}
	return opponentID, opponentID != "" && opponentID != anchorID
}

func (c *Cache) hasStrongerRival(league, rawName, opponentID string, opponentScore float64) bool {
	for _, cand := range c.rankLeagueTeams(league, rawName) {
		if cand.id == opponentID {
			continue
		}
		return cand.score >= teamAutoLinkScore && cand.score > opponentScore+ambiguityMargin
	}
	return false
}

func (c *Cache) matchCompetitor(rawName string) match {
	if isDoublesPair(rawName) {
		return match{outcome: matchNeedsReview}
	}
	pool := c.sportNames(canon.SportTennis)
	competitors := make([]squadPlayer, 0, len(pool))
	for _, n := range pool {
		competitors = append(competitors, squadPlayer{Player: Player{ID: n.teamID, Name: n.raw, TeamID: n.teamID}, name: names.Person(n.raw)})
	}
	m := matchPlayerByName(competitors, names.Person(rawName))
	if m.outcome == matchAutoLinked || len(competitors) == 0 {
		return m
	}
	return match{outcome: matchNeedsReview, candidates: c.distinctTeamCandidates(m.candidates)}
}

func (c *Cache) distinctTeamCandidates(candidates []review.Candidate) []review.Candidate {
	seen := map[string]bool{}
	distinct := candidates[:0:0]
	for _, cand := range candidates {
		if seen[cand.ID] {
			continue
		}
		seen[cand.ID] = true
		if team, ok := c.team(cand.ID); ok {
			cand.Name = team.Name
		}
		distinct = append(distinct, cand)
	}
	return distinct
}

func isNewCompetitorCandidate(m match, rawName string) bool {
	if isDoublesPair(rawName) {
		return false
	}
	person := names.Person(rawName)
	if len(person.Tokens()) < minNewCompetitorNameTokens {
		return false
	}
	for _, token := range person.Tokens() {
		if names.IsInitial(token) {
			return false
		}
	}
	return len(m.candidates) == 0 || m.candidates[0].Score < newCompetitorScoreCeiling
}

func isDoublesPair(rawName string) bool { return strings.ContainsAny(rawName, doublesPairSeparators) }
