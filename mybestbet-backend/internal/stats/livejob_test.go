package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/pkg/canon"
)

func TestEventsByLiveFeedPrefersDirectSourceAndDropsUnsupported(t *testing.T) {
	t.Parallel()
	events := []SourcedEvent{
		{ID: "cs", Source: esports.LiquipediaSource, SourceID: "CS2:abc", LeagueCode: "CS2"},
		{ID: "cs", Source: bo3.SourceName, SourceID: "1:slug", LeagueCode: "CS2"},
		{ID: "val", Source: esports.LiquipediaSource, SourceID: "VALORANT:x", LeagueCode: "VALORANT"},
		{ID: "dota", Source: esports.LiquipediaSource, SourceID: "DOTA2:y", LeagueCode: "DOTA2"},
		{ID: "nba", Source: espn.SourceName, SourceID: "nba:1", LeagueCode: "NBA"},
		{ID: "euro", Source: espn.SourceName, SourceID: "x", LeagueCode: "EUROLEAGUE"},
		{ID: "epl", Source: fotmob.SourceName, SourceID: "5", LeagueCode: "EPL"},
		{ID: "atp", Source: flashscore.SourceName, SourceID: "abc", LeagueCode: "ATP_AO"},
		{ID: "acb", Source: acb.SourceName, SourceID: "1", LeagueCode: "ACB"},
		{ID: "el", Source: euroleague.SourceName, SourceID: "E2026:69", LeagueCode: "EUROLEAGUE"},
		{ID: "el-unknown", Source: euroleague.SourceName, SourceID: "E2026:70", LeagueCode: "NBA"},
		{ID: "mls", Source: unlinkedFootball, LeagueCode: "MLS"},
	}
	byFeed := eventsByLiveFeed(events)

	assert.Equal(t, []SourcedEvent{events[1], events[2]}, byFeed[bo3.SourceName])
	assert.Equal(t, []SourcedEvent{events[3]}, byFeed[esports.OpenDotaSource])
	assert.Equal(t, []SourcedEvent{events[4]}, byFeed[espn.SourceName])
	assert.Equal(t, []SourcedEvent{events[6]}, byFeed[fotmob.SourceName])
	assert.Equal(t, []SourcedEvent{events[7]}, byFeed[flashscore.SourceName])
	assert.Equal(t, []SourcedEvent{events[8]}, byFeed[acb.SourceName])
	assert.Equal(t, []SourcedEvent{events[9]}, byFeed[euroleague.SourceName])
	assert.Equal(t, []SourcedEvent{events[11]}, byFeed[unlinkedFootball])
	assert.Len(t, byFeed, 8)
}

func TestBasketballProgressScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		progress basketballProgress
		want     canon.EventScore
	}{
		{name: "third quarter", progress: basketballProgress{home: 61, away: 70, quarter: 3, clock: "04:12"}, want: canon.EventScore{Home: 61, Away: 70, Period: canon.PeriodQuarter3, Clock: "04:12"}},
		{name: "half time", progress: basketballProgress{home: 40, away: 38, quarter: 2, halfTime: true}, want: canon.EventScore{Home: 40, Away: 38, Period: canon.PeriodHalfTime}},
		{name: "overtime", progress: basketballProgress{home: 90, away: 90, quarter: 5, clock: "01:00"}, want: canon.EventScore{Home: 90, Away: 90, Period: canon.PeriodOvertime, Clock: "01:00"}},
		{name: "finished", progress: basketballProgress{home: 86, away: 122, quarter: 4, finished: true}, want: canon.EventScore{Home: 86, Away: 122, Period: canon.PeriodFinished}},
		{name: "quarter unknown", progress: basketballProgress{home: 2}, want: canon.EventScore{Home: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.progress.score())
		})
	}
}

func TestDotaSeriesScore(t *testing.T) {
	t.Parallel()
	won := func(radiantIsHome, radiantWin bool) dotaSeriesGame {
		return dotaSeriesGame{ProGame: esports.ProGame{SeriesType: esports.BestOfThree, RadiantWin: radiantWin}, radiantIsHome: radiantIsHome}
	}
	live := dotaSeriesGame{ProGame: esports.ProGame{Live: true, RadiantKills: 15, DireKills: 13}, radiantIsHome: false}
	tests := []struct {
		name         string
		series       []dotaSeriesGame
		want         canon.EventScore
		wantFinished bool
	}{
		{name: "first map live", series: []dotaSeriesGame{live}, want: canon.EventScore{Period: "M1", Detail: "13-15"}},
		{name: "second map live", series: []dotaSeriesGame{won(true, true), live}, want: canon.EventScore{Home: 1, Period: "M2", Detail: "13-15"}},
		{name: "between maps", series: []dotaSeriesGame{won(true, false), won(false, false)}, want: canon.EventScore{Home: 1, Away: 1, Period: "M3"}},
		{name: "decided", series: []dotaSeriesGame{won(false, true), won(true, false)}, want: canon.EventScore{Away: 2, Period: canon.PeriodFinished}, wantFinished: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			score, finished := dotaSeriesScore(tt.series)
			assert.Equal(t, tt.want, score)
			assert.Equal(t, tt.wantFinished, finished)
		})
	}
}

func TestFootballMatchForNeedsKickoffAndBothTeams(t *testing.T) {
	t.Parallel()
	kickoff := time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC)
	matches := map[string]flashscore.FootballMatch{
		"lafc":  {ID: "lafc", Kickoff: kickoff, Home: "Los Angeles FC", Away: "Los Angeles Galaxy"},
		"later": {ID: "later", Kickoff: kickoff.Add(10 * time.Minute), Home: "Los Angeles FC", Away: "LA Galaxy"},
		"u21":   {ID: "u21", Kickoff: kickoff, Home: "Newcastle Utd U21", Away: "Sunderland U21"},
	}
	tests := []struct {
		name   string
		event  SourcedEvent
		wantID string
	}{
		{name: "closest kickoff wins", event: SourcedEvent{Kickoff: kickoff, HomeTeamName: "Los Angeles FC", AwayTeamName: "Los Angeles Galaxy"}, wantID: "lafc"},
		{name: "kickoff too far", event: SourcedEvent{Kickoff: kickoff.Add(time.Hour), HomeTeamName: "Los Angeles FC", AwayTeamName: "Los Angeles Galaxy"}},
		{name: "flipped sides", event: SourcedEvent{Kickoff: kickoff, HomeTeamName: "Los Angeles Galaxy", AwayTeamName: "Los Angeles FC"}},
		{name: "same name on both sides is ambiguous", event: SourcedEvent{Kickoff: kickoff, HomeTeamName: "Los Angeles FC", AwayTeamName: "Los Angeles FC"}},
		{name: "senior side is not the u21 side", event: SourcedEvent{Kickoff: kickoff, HomeTeamName: "Newcastle United", AwayTeamName: "Sunderland"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			match, found := footballMatchFor(tt.event, matches)
			assert.Equal(t, tt.wantID != "", found)
			assert.Equal(t, tt.wantID, match.ID)
		})
	}
}

func TestSeriesScoreOrientsMapsToEventSides(t *testing.T) {
	t.Parallel()
	match := bo3.Match{Status: "current", Team1: bo3.Team{ID: "1"}, Team2: bo3.Team{ID: "2"}, Maps: [2]int{1, 0}}
	tests := []struct {
		name  string
		match bo3.Match
		sides sideByTeamID
		want  canon.EventScore
	}{
		{name: "team1 is home", match: match, sides: sideByTeamID{"1": sideHome, "2": sideAway}, want: canon.EventScore{Home: 1, Period: "M2"}},
		{name: "team1 is away", match: match, sides: sideByTeamID{"1": sideAway, "2": sideHome}, want: canon.EventScore{Away: 1, Period: "M2"}},
		{name: "decided", match: bo3.Match{Status: "finished", Team1: bo3.Team{ID: "1"}, Maps: [2]int{2, 1}}, sides: sideByTeamID{"1": sideHome}, want: canon.EventScore{Home: 2, Away: 1, Period: canon.PeriodFinished}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, seriesScore(tt.match, tt.sides))
		})
	}
}

func TestScheduleDaysCoversUSDateOfLateKickoffs(t *testing.T) {
	t.Parallel()
	events := []SourcedEvent{
		{Kickoff: time.Date(2026, 10, 27, 2, 30, 0, 0, time.UTC)},
		{Kickoff: time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)},
	}
	assert.Equal(t, []time.Time{time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}, scheduleDays(events, usScheduleDateOffsets...))
	assert.Equal(t, []time.Time{time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}, scheduleDays(events, 0))
}

func TestSeriesCandidatesForMatchesSlugOrKickoff(t *testing.T) {
	t.Parallel()
	kickoff := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	series := []bo3.Match{{Slug: "a", Start: kickoff}, {Slug: "b", Start: kickoff.Add(30 * time.Minute)}, {Slug: "c", Start: kickoff.Add(3 * time.Hour)}}

	assert.Equal(t, []bo3.Match{series[1]}, seriesCandidatesFor(SourcedEvent{Source: bo3.SourceName, SourceID: "1:b", Kickoff: kickoff}, series))
	assert.Empty(t, seriesCandidatesFor(SourcedEvent{Source: bo3.SourceName, SourceID: "1:z", Kickoff: kickoff}, series))
	assert.Equal(t, series[:2], seriesCandidatesFor(SourcedEvent{Source: esports.LiquipediaSource, SourceID: "CS2:q", Kickoff: kickoff}, series))
}
