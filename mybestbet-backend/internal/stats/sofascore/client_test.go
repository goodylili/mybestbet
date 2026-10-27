package sofascore

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var fixtureByPath = map[string]string{
	"/unique-tournament/17/seasons":                    "unique_tournament_17_seasons.json",
	"/unique-tournament/17/season/96668/teams":         "season_17_96668_teams.json",
	"/unique-tournament/17/season/96668/events/next/0": "season_17_96668_events_next_0.json",
	"/team/33/players":                                 "team_33_players.json",
	"/event/16363890":                                  "event_16363890.json",
	"/event/16363890/lineups":                          "event_16363890_lineups.json",
	"/event/16363890/incidents":                        "event_16363890_incidents.json",
	"/event/16363897/lineups":                          "event_16363897_lineups.json",
}

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	srv := sourcestest.ReplayServer(t, SourceName, func(r *http.Request) (string, bool) {
		name, ok := fixtureByPath[strings.TrimPrefix(r.URL.Path, "/api/v1")]
		return name, ok
	})
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Config.BaseURL = srv.URL + "/api/v1"
	return New(deps)
}

func TestCurrentSeasonIsNewest(t *testing.T) {
	t.Parallel()
	season, err := newReplayClient(t).CurrentSeason(t.Context(), 17)
	if err != nil {
		t.Fatalf("CurrentSeason: %v", err)
	}
	if season.ID != 96668 || season.Year != "26/27" {
		t.Fatalf("season = %+v", season)
	}
}

func TestSeasonTeamsAndSquad(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	teams, err := c.SeasonTeams(t.Context(), 17, 96668)
	if err != nil {
		t.Fatalf("SeasonTeams: %v", err)
	}
	if len(teams) != 10 || teams[2] != (Team{ID: 33, Name: "Tottenham Hotspur", ShortName: "Tottenham"}) {
		t.Fatalf("teams = %+v", teams)
	}
	players, err := c.TeamPlayers(t.Context(), 33)
	if err != nil {
		t.Fatalf("TeamPlayers: %v", err)
	}
	if len(players) != 6 || players[3] != (Player{ID: 361420, Name: "Dominic Solanke", ShortName: "D. Solanke", Position: "F"}) {
		t.Fatalf("players = %+v", players)
	}
}

func TestUpcomingEventsStopsAtHorizon(t *testing.T) {
	t.Parallel()
	until := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	events, err := newReplayClient(t).UpcomingEvents(t.Context(), 17, 96668, until)
	if err != nil {
		t.Fatalf("UpcomingEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events within horizon, want 2", len(events))
	}
	first := events[0]
	if first.ID != 16363897 || first.HomeTeam.ID != 32 || first.AwayTeam.ID != 14 || first.Tournament.UniqueTournament.ID != 17 {
		t.Fatalf("first event = %+v", first)
	}
	if !first.Kickoff().Equal(time.Date(2026, 10, 23, 19, 0, 0, 0, time.UTC)) || first.IsFinished() {
		t.Fatalf("kickoff/status = %v %+v", first.Kickoff(), first.Status)
	}
}

func TestEventStatus(t *testing.T) {
	t.Parallel()
	ev, err := newReplayClient(t).Event(t.Context(), 16363890)
	if err != nil {
		t.Fatalf("Event: %v", err)
	}
	if !ev.IsFinished() || ev.HomeTeam.ID != 33 || ev.AwayTeam.ID != 11 || ev.Season.ID != 96668 {
		t.Fatalf("event = %+v", ev)
	}
}

func TestMissingResourceMapsToNotFound(t *testing.T) {
	t.Parallel()
	_, err := newReplayClient(t).Lineups(t.Context(), 1)
	if err == nil || !strings.Contains(err.Error(), "/event/1/lineups") {
		t.Fatalf("err = %v", err)
	}
}
