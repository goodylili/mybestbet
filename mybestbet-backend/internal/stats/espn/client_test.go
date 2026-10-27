package espn

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	srv := sourcestest.ReplayServer(t, SourceName, func(r *http.Request) (string, bool) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		query := r.URL.Query()
		switch path {
		case "basketball/nba/scoreboard":
			return "nba_scoreboard_" + query.Get("dates") + ".json", query.Get("dates") == "20261024" || query.Get("dates") == "20261025"
		case "football/nfl/scoreboard":
			return "nfl_scoreboard_" + query.Get("dates") + ".json", query.Get("dates") == "20261025"
		case "tennis/atp/scoreboard":
			return "atp_scoreboard_" + query.Get("dates") + ".json", query.Get("dates") == "20260325"
		case "basketball/nba/teams":
			return "nba_teams.json", true
		case "basketball/nba/teams/30/roster":
			return "nba_roster_30.json", true
		case "football/nfl/teams/29/roster":
			return "nfl_roster_29.json", true
		case "basketball/nba/summary":
			return "nba_summary_401909848.json", query.Get("event") == "401909848"
		case "football/nfl/summary":
			return "nfl_summary_401873008.json", query.Get("event") == "401873008"
		}
		return "", false
	})
	return New(sourcestest.Deps(srv, sources.Config{}))
}

func TestGamesMapsHomeAndAway(t *testing.T) {
	t.Parallel()
	board, err := newReplayClient(t).Games(t.Context(), "NBA", time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Games: %v", err)
	}
	games := board.Games
	if board.LeagueLogoURL != "https://a.espncdn.com/i/teamlogos/leagues/500/nba.png" {
		t.Fatalf("league logo = %q", board.LeagueLogoURL)
	}
	if len(games) != 3 {
		t.Fatalf("got %d games, want 3", len(games))
	}
	first := games[0]
	if first.ID != "401909853" || first.Home.Name != "Charlotte Hornets" || first.Away.ID != "14" || first.Kickoff.IsZero() || first.Status != "STATUS_SCHEDULED" {
		t.Fatalf("first game = %+v", first)
	}
	if first.Home.LogoURL != "https://a.espncdn.com/i/teamlogos/nba/500/scoreboard/cha.png" {
		t.Fatalf("home logo = %q", first.Home.LogoURL)
	}
}

func TestGamesUnknownLeague(t *testing.T) {
	t.Parallel()
	if _, err := newReplayClient(t).Games(t.Context(), "EPL", time.Now()); err == nil {
		t.Fatal("expected ErrUnknownLeague")
	}
}

func TestTeamsAndRoster(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	teams, err := c.Teams(t.Context(), "NBA")
	if err != nil {
		t.Fatalf("Teams: %v", err)
	}
	if len(teams) != 30 || teams[0].Abbreviation == "" || !strings.HasPrefix(teams[0].LogoURL, "https://a.espncdn.com/i/teamlogos/nba/500/") {
		t.Fatalf("teams = %d, first %+v", len(teams), teams[0])
	}
	roster, err := c.Roster(t.Context(), "NBA", "30")
	if err != nil {
		t.Fatalf("Roster: %v", err)
	}
	if len(roster) != 6 || roster[0].ID == "" || roster[0].Name == "" {
		t.Fatalf("roster = %+v", roster)
	}
}

func TestFootballRosterFlattensPositionGroups(t *testing.T) {
	t.Parallel()
	roster, err := newReplayClient(t).Roster(t.Context(), "NFL", "29")
	if err != nil {
		t.Fatalf("Roster: %v", err)
	}
	if len(roster) != 10 || roster[0].ID == "" || roster[0].Position == "" {
		t.Fatalf("roster = %+v", roster)
	}
}

func TestBasketballBoxscore(t *testing.T) {
	t.Parallel()
	box, err := newReplayClient(t).Boxscore(t.Context(), "NBA", "401909848")
	if err != nil {
		t.Fatalf("Boxscore: %v", err)
	}
	if !box.IsFinal() || len(box.Lines) == 0 || box.HomeTeamID != "15" {
		t.Fatalf("box = %+v", box)
	}
	smith := findLine(t, box, "Jalen Smith")
	expected := statkey.Values{statkey.Points: 14, statkey.Rebounds: 13, statkey.Assists: 1, statkey.Threes: 2}
	for key, want := range expected {
		if smith.Values[key] != want {
			t.Errorf("%s = %v, want %v", key, smith.Values[key], want)
		}
	}
	if smith.Minutes != 24 || !smith.Started || smith.TeamID != "4" {
		t.Errorf("smith = %+v", smith)
	}
}

func TestFootballBoxscoreMergesCategoriesAndZeroFills(t *testing.T) {
	t.Parallel()
	box, err := newReplayClient(t).Boxscore(t.Context(), "NFL", "401873008")
	if err != nil {
		t.Fatalf("Boxscore: %v", err)
	}
	daniels := findLine(t, box, "Jayden Daniels")
	if daniels.Values[statkey.PassYards] != 300 || daniels.Values[statkey.PassTDs] != 2 {
		t.Errorf("daniels = %+v", daniels.Values)
	}
	okonkwo := findLine(t, box, "Chig Okonkwo")
	if okonkwo.Values[statkey.Receptions] != 7 || okonkwo.Values[statkey.RecYards] != 69 {
		t.Errorf("okonkwo = %+v", okonkwo.Values)
	}
	if value, present := okonkwo.Values[statkey.RushYards]; !present || value != 0 {
		t.Errorf("rush yards should be zero-filled, got %v %v", value, present)
	}
}

func TestTennisMatchesMapsTournamentAndDraw(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).TennisMatches(t.Context(), "tennis/atp", time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("TennisMatches: %v", err)
	}
	byLeague := map[string]int{}
	for _, m := range matches {
		byLeague[m.LeagueCode]++
	}
	if byLeague["ATP_MIAMI"] != 3 || byLeague["WTA_MIAMI"] != 2 || len(matches) != 5 {
		t.Fatalf("matches by league = %v", byLeague)
	}
	if matches[0].Home.Name == "" || matches[0].Away.ID == "" {
		t.Fatalf("first = %+v", matches[0])
	}
}

func TestTennisLeagueCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, draw, expected string
		ok                   bool
	}{
		{"Miami Open presented by Itau", "mens-singles", "ATP_MIAMI", true},
		{"Wimbledon", "womens-singles", "WTA_WIM", true},
		{"US Open", "mens-doubles", "", false},
		{"Almaty Open", "mens-singles", "", false},
	}
	for _, tt := range tests {
		got, ok := TennisLeagueCode(tt.name, tt.draw)
		if got != tt.expected || ok != tt.ok {
			t.Errorf("TennisLeagueCode(%q, %q) = %q, %v", tt.name, tt.draw, got, ok)
		}
	}
}

func findLine(t *testing.T, box Boxscore, name string) PlayerLine {
	t.Helper()
	for _, l := range box.Lines {
		if l.Athlete.Name == name {
			return l
		}
	}
	t.Fatalf("no line for %s", name)
	return PlayerLine{}
}
