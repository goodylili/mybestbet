package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestAliasOfTeamOutsideLeagueIsNotTrusted(t *testing.T) {
	c := newTestCache(t)
	c.addLeague("PRIMEIRA", canon.SportFootball)
	c.addLeague("LIBERTADORES", canon.SportFootball)
	c.addTeam(Team{ID: "nacional-pt", League: "PRIMEIRA", Name: "Nacional"})
	c.addTeam(Team{ID: "nacional-uy", League: "LIBERTADORES", Name: "Nacional"})
	c.addTeam(Team{ID: "flamengo", League: "LIBERTADORES", Name: "Flamengo"})
	c.addTeamAlias(testSource, "Nacional", "nacional-pt")
	m := c.matchTeam(testSource, "LIBERTADORES", "Nacional")
	assert.Equal(t, matchAutoLinked, m.outcome)
	assert.Equal(t, "nacional-uy", m.id)
}

func TestClubJoiningCupIsMatchedThroughMembership(t *testing.T) {
	c := newTestCache(t)
	c.addTeam(Team{ID: "arsenal", League: "EPL", Name: "Arsenal"})
	c.addTeamAlias(seedSource, "Arsenal FC", "arsenal")
	c.joinLeague("arsenal", "UCL")
	m := c.matchTeam(testSource, "UCL", "Arsenal FC")
	assert.Equal(t, matchAutoLinked, m.outcome)
	assert.Equal(t, "arsenal", m.id)
}

func TestTennisCompetitorPoolSpansTournaments(t *testing.T) {
	c := newTestCache(t)
	c.addLeague("ATP_AO", canon.SportTennis)
	c.addLeague("ATP_MIAMI", canon.SportTennis)
	c.addTeam(Team{ID: "sinner", League: "ATP_AO", Name: "Jannik Sinner"})
	m := c.matchTeam(testSource, "ATP_MIAMI", "Sinner J.")
	assert.Equal(t, matchAutoLinked, m.outcome)
	assert.Equal(t, "sinner", m.id)
}

func TestNewTennisCompetitorOnlyForUnmatchedFullNames(t *testing.T) {
	c := newTestCache(t)
	c.addLeague("ATP_CH_ZADAR", canon.SportTennis)
	c.addTeam(Team{ID: "sinner", League: "ATP_CH_ZADAR", Name: "Jannik Sinner"})
	tests := []struct {
		name     string
		raw      string
		expected bool
	}{
		{name: "unknown full name", raw: "Luka Mikrut", expected: true},
		{name: "initial only", raw: "Mikrut L.", expected: false},
		{name: "mononym", raw: "Mikrut", expected: false},
		{name: "doubles pair", raw: "Mikrut L. / Pavic M.", expected: false},
	}
	assert.Equal(t, matchAutoLinked, c.matchTeam(testSource, "ATP_CH_ZADAR", "Janik Siner").outcome)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := c.matchTeam(testSource, "ATP_CH_ZADAR", tt.raw)
			require.Equal(t, matchNeedsReview, m.outcome)
			assert.Equal(t, tt.expected, isNewCompetitorCandidate(m, tt.raw))
		})
	}
}

func TestKickoffToleranceFollowsSport(t *testing.T) {
	c := newTestCache(t)
	c.addLeague("ATP_AO", canon.SportTennis)
	c.addLeague("NBA", canon.SportBasketball)
	scheduled := time.Date(2027, 1, 20, 1, 0, 0, 0, time.UTC)
	c.addEvent(Event{ID: "tennis", League: "ATP_AO", HomeTeamID: "a", AwayTeamID: "b", Kickoff: scheduled})
	c.addEvent(Event{ID: "nba", League: "NBA", HomeTeamID: "c", AwayTeamID: "d", Kickoff: scheduled})
	followsPreviousMatch := scheduled.Add(5 * time.Hour)
	assert.Len(t, c.eventCandidates("ATP_AO", "a", "b", followsPreviousMatch), 1)
	assert.Empty(t, c.eventCandidates("NBA", "c", "d", followsPreviousMatch))
	assert.Empty(t, c.eventCandidates("NBA", "c", "d", scheduled.Add(16*time.Minute)))
}

func TestMatchingConfigFallsBackToDefaults(t *testing.T) {
	m := MatchingConfig{SportKickoffTolerance: map[canon.Sport]time.Duration{canon.SportTennis: 6 * time.Hour}}
	assert.Equal(t, 6*time.Hour, m.KickoffToleranceFor(canon.SportTennis))
	assert.Equal(t, 15*time.Minute, m.KickoffToleranceFor(canon.SportEsports))
}
