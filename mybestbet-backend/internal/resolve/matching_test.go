package resolve

import (
	"cmp"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"mybestbet/pkg/canon"
)

const (
	teamPrecisionTarget   = 0.98
	playerPrecisionTarget = 0.95
	teamRecallFloor       = 0.85
	playerRecallFloor     = 0.80
	testSource            = "labelled"
	seedSource            = "seed"
)

type labelledLeague struct {
	Sport    canon.Sport         `yaml:"sport"`
	Teams    []string            `yaml:"teams"`
	Aliases  map[string][]string `yaml:"aliases"`
	Variants map[string]string   `yaml:"variants"`
}

func (l labelledLeague) sport() canon.Sport {
	if l.Sport == "" {
		return canon.SportFootball
	}
	return l.Sport
}

type labelledFixture struct {
	League   string            `yaml:"league"`
	Sport    canon.Sport       `yaml:"sport"`
	Home     []string          `yaml:"home"`
	Away     []string          `yaml:"away"`
	Variants map[string]string `yaml:"variants"`
}

type precisionTally struct {
	linked, correct, expectedLinks, correctLinks int
	wrong                                        []string
}

func (p *precisionTally) record(variant, expected, linkedName string, isLinked bool) {
	if expected != "" {
		p.expectedLinks++
	}
	if !isLinked {
		return
	}
	p.linked++
	if linkedName == expected {
		p.correct++
		p.correctLinks++
		return
	}
	p.wrong = append(p.wrong, variant+" -> "+linkedName+" (want "+strconv.Quote(expected)+")")
}

func (p *precisionTally) precision() float64 { return float64(p.correct) / float64(p.linked) }

func (p *precisionTally) recall() float64 { return float64(p.correctLinks) / float64(p.expectedLinks) }

func readYAML(t *testing.T, path string, out any) {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(body, out))
}

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := NewCache()
	require.NoError(t, err)
	return c
}

func teamID(league, name string) string { return league + "/" + name }

func TestTeamMatchingMeetsPrecisionTargetPerSport(t *testing.T) {
	var leagues map[string]labelledLeague
	readYAML(t, "testdata/teams.yaml", &leagues)
	c := newTestCache(t)
	for code, league := range leagues {
		c.addLeague(code, league.sport())
		for _, name := range league.Teams {
			c.addTeam(Team{ID: teamID(code, name), League: code, Name: name})
		}
		for name, aliases := range league.Aliases {
			for _, alias := range aliases {
				c.addTeamAlias(seedSource, alias, teamID(code, name))
			}
		}
	}
	tallies := map[canon.Sport]*precisionTally{}
	for code, league := range leagues {
		tally := tallyFor(tallies, league.sport())
		for variant, expected := range league.Variants {
			m := c.matchTeam(testSource, code, variant)
			linkedName := ""
			if m.outcome == matchAutoLinked {
				team, _ := c.team(m.id)
				linkedName = team.Name
			}
			tally.record(code+":"+variant, expected, linkedName, m.outcome == matchAutoLinked)
		}
	}
	require.Len(t, tallies, len(canon.Sports))
	for sport, tally := range tallies {
		t.Run(string(sport), func(t *testing.T) {
			sort.Strings(tally.wrong)
			t.Logf("%s teams: %d auto-linked, precision %.3f, recall %.3f, wrong %v", sport, tally.linked, tally.precision(), tally.recall(), tally.wrong)
			assert.GreaterOrEqual(t, tally.precision(), teamPrecisionTarget)
			assert.GreaterOrEqual(t, tally.recall(), teamRecallFloor)
		})
	}
}

func tallyFor(tallies map[canon.Sport]*precisionTally, sport canon.Sport) *precisionTally {
	if tallies[sport] == nil {
		tallies[sport] = &precisionTally{}
	}
	return tallies[sport]
}

func TestPlayerMatchingMeetsPrecisionTargetPerSport(t *testing.T) {
	var file struct {
		Fixtures []labelledFixture `yaml:"fixtures"`
	}
	readYAML(t, "testdata/players.yaml", &file)
	c := newTestCache(t)
	tallies := map[canon.Sport]*precisionTally{}
	for i, fx := range file.Fixtures {
		sport := cmp.Or(fx.Sport, canon.SportFootball)
		c.addLeague(fx.League, sport)
		ev := Event{ID: "event-" + strconv.Itoa(i), League: fx.League, HomeTeamID: "home-" + strconv.Itoa(i), AwayTeamID: "away-" + strconv.Itoa(i)}
		for _, name := range fx.Home {
			c.addPlayer(Player{ID: ev.HomeTeamID + "/" + name, Name: name, TeamID: ev.HomeTeamID})
		}
		for _, name := range fx.Away {
			c.addPlayer(Player{ID: ev.AwayTeamID + "/" + name, Name: name, TeamID: ev.AwayTeamID})
		}
		tally := tallyFor(tallies, sport)
		for variant, expected := range fx.Variants {
			m := c.matchPlayer(testSource, canon.SourcePlayer{Name: variant}, ev)
			linkedName := ""
			if m.outcome == matchAutoLinked {
				linkedName = c.players[m.id].Name
			}
			tally.record(variant, expected, linkedName, m.outcome == matchAutoLinked)
		}
	}
	require.Len(t, tallies, len(canon.Sports))
	for sport, tally := range tallies {
		t.Run(string(sport), func(t *testing.T) {
			sort.Strings(tally.wrong)
			t.Logf("%s players: %d auto-linked, precision %.3f, recall %.3f, wrong %v", sport, tally.linked, tally.precision(), tally.recall(), tally.wrong)
			assert.GreaterOrEqual(t, tally.precision(), playerPrecisionTarget)
			assert.GreaterOrEqual(t, tally.recall(), playerRecallFloor)
		})
	}
}

func TestDecideByScoreThresholds(t *testing.T) {
	tests := []struct {
		name     string
		ranked   []scoredCandidate
		expected matchOutcome
	}{
		{name: "clear winner above threshold links", ranked: []scoredCandidate{{id: "a", score: 0.95}, {id: "b", score: 0.70}}, expected: matchAutoLinked},
		{name: "exactly at threshold links", ranked: []scoredCandidate{{id: "a", score: 0.93}}, expected: matchAutoLinked},
		{name: "review band", ranked: []scoredCandidate{{id: "a", score: 0.85}}, expected: matchNeedsReview},
		{name: "unknown band", ranked: []scoredCandidate{{id: "a", score: 0.50}}, expected: matchNeedsReview},
		{name: "ambiguous pair goes to review", ranked: []scoredCandidate{{id: "a", score: 0.96}, {id: "b", score: 0.95}}, expected: matchNeedsReview},
		{name: "no candidates", ranked: nil, expected: matchNeedsReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, decideByScore(tt.ranked, teamAutoLinkScore).outcome)
		})
	}
}

func TestReviewCarriesTopThreeCandidates(t *testing.T) {
	ranked := []scoredCandidate{{id: "a", score: 0.85}, {id: "b", score: 0.84}, {id: "c", score: 0.83}, {id: "d", score: 0.82}}
	m := decideByScore(ranked, teamAutoLinkScore)
	require.Len(t, m.candidates, 3)
	assert.Equal(t, "a", m.candidates[0].ID)
}

func TestExactAliasWinsOverFuzzy(t *testing.T) {
	c := newTestCache(t)
	c.addTeam(Team{ID: "t1", League: "EPL", Name: "Arsenal"})
	c.addTeam(Team{ID: "t2", League: "EPL", Name: "Chelsea"})
	c.addTeamAlias(testSource, "The Gunners", "t1")
	m := c.matchTeam(testSource, "EPL", "The Gunners")
	assert.Equal(t, matchExactAlias, m.outcome)
	assert.Equal(t, "t1", m.id)
}

func TestEventCandidates(t *testing.T) {
	c := newTestCache(t)
	kickoff := time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC)
	c.addEvent(Event{ID: "e1", League: "EPL", HomeTeamID: "ars", AwayTeamID: "che", Kickoff: kickoff})
	tests := []struct {
		name          string
		league        string
		home, away    string
		kickoff       time.Time
		expectedCount int
		isFlipped     bool
	}{
		{name: "same orientation within window", league: "EPL", home: "ars", away: "che", kickoff: kickoff.Add(10 * time.Minute), expectedCount: 1},
		{name: "flipped teams still match", league: "EPL", home: "che", away: "ars", kickoff: kickoff, expectedCount: 1, isFlipped: true},
		{name: "outside fifteen minutes", league: "EPL", home: "ars", away: "che", kickoff: kickoff.Add(16 * time.Minute), expectedCount: 0},
		{name: "different league", league: "CHAMP", home: "ars", away: "che", kickoff: kickoff, expectedCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.eventCandidates(tt.league, tt.home, tt.away, tt.kickoff)
			require.Len(t, got, tt.expectedCount)
			if tt.expectedCount == 1 {
				assert.Equal(t, tt.isFlipped, got[0].flipped)
			}
		})
	}
}
