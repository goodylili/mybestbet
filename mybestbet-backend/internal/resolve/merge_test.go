package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

var mergeKickoff = time.Date(2026, 10, 26, 19, 30, 0, 0, time.UTC)

func mergeFixture(id string, home, away mergeTeam, kickoff time.Time, links int, sources ...string) mergeEvent {
	return mergeEvent{
		id: id, league: "LIGUE1", home: home, away: away, kickoff: kickoff,
		createdAt: mergeKickoff.Add(-72 * time.Hour), links: links, sources: sources,
	}
}

var (
	psg   = mergeTeam{id: "psg", name: "Paris Saint-Germain", sources: []string{"sofascore"}}
	lyon  = mergeTeam{id: "lyon", name: "Olympique Lyonnais", sources: []string{"sofascore"}}
	wolvs = mergeTeam{id: "wolves", name: "Wolverhampton", sources: []string{"sofascore"}}
	wolvf = mergeTeam{id: "wolves-fm", name: "Wolverhampton Wanderers", sources: []string{"fotmob"}}
	galax = mergeTeam{id: "galaxy", name: "LA Galaxy", sources: []string{"fotmob"}}
	lafc  = mergeTeam{id: "lafc", name: "Los Angeles FC", sources: []string{"fotmob"}}
)

func notCollege(string) bool { return false }

func TestPlanMergesFoldsTheClockShiftedCopyIntoTheBestLinkedEvent(t *testing.T) {
	real := mergeFixture("real", psg, lyon, mergeKickoff, 12, "sofascore", "fotmob", "betway", "kalshi")
	shifted := mergeFixture("bet9ja-copy", psg, lyon, mergeKickoff.Add(-time.Hour), 1, "bet9ja")
	shifted.createdAt = mergeKickoff.Add(-6 * time.Hour)
	merges, groups := planMerges([]mergeEvent{shifted, real}, notCollege)
	assert.Equal(t, 1, groups)
	assert.Equal(t, []eventMerge{{duplicate: "bet9ja-copy", canonical: "real"}}, merges)
}

func TestPlanMergesBreaksLinkTiesByEarliestCreation(t *testing.T) {
	older := mergeFixture("older", psg, lyon, mergeKickoff, 2, "kalshi")
	newer := mergeFixture("newer", psg, lyon, mergeKickoff.Add(time.Hour), 2, "polymarket")
	newer.createdAt = older.createdAt.Add(time.Minute)
	merges, _ := planMerges([]mergeEvent{newer, older}, notCollege)
	assert.Equal(t, []eventMerge{{duplicate: "newer", canonical: "older"}}, merges)
}

func TestPlanMergesRecordsReversedOrientation(t *testing.T) {
	real := mergeFixture("real", psg, lyon, mergeKickoff, 5, "betway")
	reversed := mergeFixture("reversed", lyon, psg, mergeKickoff.Add(2*time.Hour), 1, "kalshi")
	merges, _ := planMerges([]mergeEvent{real, reversed}, notCollege)
	assert.Equal(t, []eventMerge{{duplicate: "reversed", canonical: "real", flipped: true}}, merges)
}

func TestPlanMergesKeepsDistinctFixturesApart(t *testing.T) {
	real := mergeFixture("real", psg, lyon, mergeKickoff, 10, "betway", "sofascore")
	cup := mergeFixture("cup", psg, lyon, mergeKickoff, 1, "bet9ja")
	cup.league = "CDF"
	tests := []struct {
		name    string
		real    mergeEvent
		other   mergeEvent
		college bool
	}{
		{name: "kickoff beyond the window", real: real, other: mergeFixture("later", psg, lyon, mergeKickoff.Add(3*time.Hour+time.Minute), 1, "bet9ja")},
		{name: "same source lists both", real: real, other: mergeFixture("second", psg, lyon, mergeKickoff.Add(time.Hour), 1, "betway")},
		{name: "other league", real: real, other: cup},
		{
			name:  "teams with ids from the same source",
			real:  mergeFixture("galaxy", galax, lyon, mergeKickoff, 10, "betway"),
			other: mergeFixture("lafc", lafc, lyon, mergeKickoff, 1, "bet9ja"),
		},
		{
			name:    "college league never aliases",
			real:    mergeFixture("wolves", wolvs, lyon, mergeKickoff, 10, "betway"),
			other:   mergeFixture("wolves-fm", wolvf, lyon, mergeKickoff, 1, "bet9ja"),
			college: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merges, groups := planMerges([]mergeEvent{tt.real, tt.other}, func(string) bool { return tt.college })
			assert.Empty(t, merges)
			assert.Zero(t, groups)
		})
	}
}

func TestPlanMergesTreatsAliasedTeamRowsAsOneTeam(t *testing.T) {
	real := mergeFixture("real", wolvs, lyon, mergeKickoff, 10, "betway", "sofascore")
	fotmobCopy := mergeFixture("fotmob-copy", wolvf, lyon, mergeKickoff, 0, "fotmob")
	merges, _ := planMerges([]mergeEvent{fotmobCopy, real}, notCollege)
	assert.Equal(t, []eventMerge{{duplicate: "fotmob-copy", canonical: "real"}}, merges)
}

func TestPlanMergesFoldsASimultaneousFixtureWithAnUnrelatedTeamName(t *testing.T) {
	rennais := mergeTeam{id: "rennais", name: "Stade Rennais", sources: []string{"sofascore"}}
	rennes := mergeTeam{id: "rennes", name: "Rennes", sources: []string{"fotmob"}}
	nice := mergeTeam{id: "nice", name: "Nice", sources: []string{"sofascore"}}
	real := mergeFixture("real", nice, rennais, mergeKickoff, 12, "betway")
	copied := mergeFixture("copy", nice, rennes, mergeKickoff, 1, "bet9ja")
	merges, _ := planMerges([]mergeEvent{copied, real}, notCollege)
	assert.Equal(t, []eventMerge{{duplicate: "copy", canonical: "real"}}, merges)

	later := mergeFixture("later", nice, rennes, mergeKickoff.Add(time.Hour), 1, "bet9ja")
	merges, _ = planMerges([]mergeEvent{later, real}, notCollege)
	assert.Empty(t, merges)

	galaxyGame := mergeFixture("galaxy", galax, lyon, mergeKickoff, 3, "polymarket")
	lafcGame := mergeFixture("lafc", lafc, lyon, mergeKickoff, 1, "kalshi")
	merges, _ = planMerges([]mergeEvent{galaxyGame, lafcGame}, notCollege)
	assert.Empty(t, merges)
}

func TestIsClockShift(t *testing.T) {
	tests := []struct {
		name   string
		offset time.Duration
		want   bool
	}{
		{name: "same kickoff", offset: 0},
		{name: "one hour early", offset: -time.Hour, want: true},
		{name: "two hours late within tolerance", offset: 2*time.Hour + 10*time.Minute, want: true},
		{name: "three hours", offset: 3 * time.Hour, want: true},
		{name: "ninety minutes", offset: 90 * time.Minute},
		{name: "four hours", offset: 4 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isClockShift(tt.offset, 15*time.Minute))
		})
	}
}

func TestFixtureCandidatesFallBackToAClockShiftedEvent(t *testing.T) {
	c := newTestCache(t)
	c.addLeague("LALIGA", canon.SportFootball)
	c.addTeam(Team{ID: "sevilla", League: "LALIGA", Name: "Sevilla"})
	c.addTeam(Team{ID: "osasuna", League: "LALIGA", Name: "Osasuna"})
	c.addEvent(Event{ID: "real", League: "LALIGA", HomeTeamID: "sevilla", AwayTeamID: "osasuna", Kickoff: mergeKickoff})

	shifted := c.fixtureCandidates("LALIGA", "sevilla", "osasuna", mergeKickoff.Add(-time.Hour))
	assert.Len(t, shifted, 1)
	assert.Equal(t, "real", shifted[0].event.ID)
	assert.Empty(t, c.eventCandidates("LALIGA", "sevilla", "osasuna", mergeKickoff.Add(-time.Hour)))
	assert.Empty(t, c.fixtureCandidates("LALIGA", "sevilla", "osasuna", mergeKickoff.Add(-90*time.Minute)))
	assert.Empty(t, c.fixtureCandidates("CDR", "sevilla", "osasuna", mergeKickoff.Add(-time.Hour)))

	c.addEvent(Event{ID: "near", League: "LALIGA", HomeTeamID: "osasuna", AwayTeamID: "sevilla", Kickoff: mergeKickoff.Add(-time.Hour)})
	exact := c.fixtureCandidates("LALIGA", "sevilla", "osasuna", mergeKickoff.Add(-time.Hour))
	assert.Len(t, exact, 1)
	assert.Equal(t, "near", exact[0].event.ID)
	assert.True(t, exact[0].flipped)
}
