package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func fixtureCache(t *testing.T) (*Cache, time.Time) {
	t.Helper()
	c := newTestCache(t)
	c.addLeague("SSL", canon.SportFootball)
	c.addTeam(Team{ID: "grasshopper", League: "SSL", Name: "Grasshopper"})
	c.addTeam(Team{ID: "zurich", League: "SSL", Name: "FC Zürich"})
	c.addTeam(Team{ID: "vaduz", League: "SSL", Name: "FC Vaduz"})
	c.addTeam(Team{ID: "basel", League: "SSL", Name: "FC Basel"})
	kickoff := time.Date(2026, 10, 31, 17, 0, 0, 0, time.UTC)
	c.addEvent(Event{ID: "gc-vaduz", League: "SSL", HomeTeamID: "grasshopper", AwayTeamID: "vaduz", Kickoff: kickoff})
	return c, kickoff
}

func TestFixtureOpponentIsInferredFromTheSettledSide(t *testing.T) {
	c, kickoff := fixtureCache(t)
	r := &Resolver{cache: c}
	home, away := r.matchFixtureTeams(testSource, canon.SourceEvent{LeagueHint: "SSL", Home: "Grasshopper Club Zurich", Away: "FC Vaduz", Kickoff: kickoff.Add(10 * time.Minute)})
	assert.Equal(t, matchFixtureOpponent, home.outcome)
	assert.Equal(t, "grasshopper", home.id)
	assert.Equal(t, "vaduz", away.id)
}

func TestFixtureOpponentNeedsAUniqueNearbyFixture(t *testing.T) {
	tests := []struct {
		name    string
		kickoff time.Duration
		extra   *Event
		raw     string
	}{
		{name: "kickoff outside tolerance", kickoff: time.Hour, raw: "Grasshopper Club Zurich"},
		{name: "two fixtures for the anchor", extra: &Event{ID: "basel-vaduz", League: "SSL", HomeTeamID: "basel", AwayTeamID: "vaduz"}, raw: "Grasshopper Club Zurich"},
		{name: "name unlike the opponent", raw: "Lugano"},
		{name: "name clearly another club", raw: "FC Basel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, kickoff := fixtureCache(t)
			if tt.extra != nil {
				tt.extra.Kickoff = kickoff
				c.addEvent(*tt.extra)
			}
			r := &Resolver{cache: c}
			home, _ := r.matchFixtureTeams(testSource, canon.SourceEvent{LeagueHint: "SSL", Home: tt.raw, Away: "FC Vaduz", Kickoff: kickoff.Add(tt.kickoff)})
			assert.NotEqual(t, matchFixtureOpponent, home.outcome)
			assert.NotEqual(t, "grasshopper", home.id)
		})
	}
}

func TestWomenOnlyLeagueImpliesTheQualifier(t *testing.T) {
	c := newTestCache(t)
	c.addTeam(Team{ID: "arsenal-w", League: "UWCL", Name: "Arsenal (W)"})
	c.addTeam(Team{ID: "benfica-w", League: "UWCL", Name: "Benfica (W)"})
	c.addTeam(Team{ID: "arsenal", League: "EPL", Name: "Arsenal"})
	c.joinLeague("arsenal", "FRIENDLY")
	c.addTeam(Team{ID: "arsenal-w2", League: "FRIENDLY", Name: "Arsenal (W)"})
	assert.Equal(t, "arsenal-w", c.matchTeam(testSource, "UWCL", "Arsenal WFC").id)
	assert.Equal(t, "benfica-w", c.matchTeam(testSource, "UWCL", "SL Benfica").id)
	assert.Equal(t, "arsenal", c.matchTeam(testSource, "FRIENDLY", "Arsenal").id)
}

func TestCollegeLeaguesSkipSingleTokenAndStateSubsets(t *testing.T) {
	c := newTestCache(t)
	c.setMatching(DefaultMatchingConfig)
	c.addLeague("NCAAF", canon.SportAmericanFootball)
	c.addTeam(Team{ID: "arkansas-state", League: "NCAAF", Name: "Arkansas State Red Wolves"})
	c.addTeam(Team{ID: "utah-state", League: "NCAAF", Name: "Utah State Aggies"})
	c.addTeamAlias(seedSource, "Utah State", "utah-state")
	assert.Equal(t, matchNeedsReview, c.matchTeam(testSource, "NCAAF", "Arkansas").outcome)
	assert.Equal(t, matchNeedsReview, c.matchTeam(testSource, "NCAAF", "Utah").outcome)
	assert.Equal(t, matchNeedsReview, c.matchTeam(testSource, "NCAAF", "Utah Utes").outcome)
	assert.Equal(t, "utah-state", c.matchTeam(testSource, "NCAAF", "Utah St.").id)
}
