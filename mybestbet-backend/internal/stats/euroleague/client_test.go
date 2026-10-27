package euroleague

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var (
	octoberSeason      = time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	liveHeaderFixtures = map[string]string{"69": "e_header_69.json", "70": "e_header_70_live_synthetic.json", "89": "e_header_89.json"}
)

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	srv := sourcestest.ReplayServer(t, SourceName, func(r *http.Request) (string, bool) {
		switch r.URL.Path {
		case "/v2/competitions/E/seasons/E2026/games":
			return "e_games.json", true
		case "/v2/competitions/E/seasons/E2026/people":
			return "e_people.json", r.URL.Query().Get("personType") == "J"
		case "/v2/competitions/E/seasons/E2026/games/69/stats":
			return "e_game_69_stats.json", true
		case "/v2/competitions/E/seasons/E2026/games/69":
			return "e_game_69.json", true
		case "/api/Header":
			return liveHeaderFixtures[r.URL.Query().Get("gamecode")], r.URL.Query().Get("seasoncode") == "E2026"
		}
		return "", false
	})
	return New(sourcestest.Deps(srv, sources.Config{Extra: map[string]string{liveBaseURLExtra: srv.URL}}))
}

func TestSeasonCode(t *testing.T) {
	t.Parallel()
	if got := SeasonCode("E", time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)); got != "E2026" {
		t.Errorf("March 2027 = %s", got)
	}
	if got := SeasonCode("U", octoberSeason); got != "U2026" {
		t.Errorf("October 2026 = %s", got)
	}
}

func TestDisplayName(t *testing.T) {
	t.Parallel()
	if got := DisplayName("WALKER IV, LONNIE"); got != "Lonnie Walker Iv" {
		t.Errorf("got %q", got)
	}
	if got := DisplayName("NENE"); got != "Nene" {
		t.Errorf("got %q", got)
	}
}

func TestGamesAndPlayers(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	games, err := c.Games(t.Context(), "EUROLEAGUE", octoberSeason)
	if err != nil {
		t.Fatalf("Games: %v", err)
	}
	if len(games) != 4 || games[0].Code != 69 || !games[0].Played || games[0].Home.Code != "PRS" || games[0].Away.Code != "PAM" || !strings.HasPrefix(games[0].Home.LogoURL, "https://media-cdn.incrowdsports.com/") {
		t.Fatalf("games = %+v", games)
	}
	if !games[1].Kickoff.Equal(time.Date(2026, 10, 27, 16, 0, 0, 0, time.UTC)) {
		t.Errorf("kickoff = %v", games[1].Kickoff)
	}
	players, err := c.Players(t.Context(), "EUROLEAGUE", octoberSeason)
	if err != nil {
		t.Fatalf("Players: %v", err)
	}
	if len(players) == 0 || players[0].ClubCode == "" || players[0].Code == "" {
		t.Fatalf("players = %+v", players)
	}
}

func TestGameStats(t *testing.T) {
	t.Parallel()
	lines, err := newReplayClient(t).GameStats(t.Context(), "E2026", 69)
	if err != nil {
		t.Fatalf("GameStats: %v", err)
	}
	if len(lines) != 8 {
		t.Fatalf("got %d lines", len(lines))
	}
	first := lines[0]
	if first.Player.Name != "Joel Ayayi" || !first.IsHome || first.Minutes != 19 || first.Player.ClubCode != "PRS" {
		t.Fatalf("first = %+v", first)
	}
	if _, ok := first.Values[statkey.Threes]; !ok {
		t.Fatal("threes missing")
	}
	if lines[4].IsHome {
		t.Fatal("road players marked home")
	}
}

func TestUnknownCompetition(t *testing.T) {
	t.Parallel()
	if _, err := newReplayClient(t).Games(t.Context(), "ACB", octoberSeason); err == nil {
		t.Fatal("expected error")
	}
}

func TestGameState(t *testing.T) {
	t.Parallel()
	client := newReplayClient(t)
	tests := []struct {
		name     string
		gameCode int
		want     GameState
	}{
		{name: "finished reads the played game", gameCode: 69, want: GameState{Started: true, Finished: true, HomeScore: 93, AwayScore: 104}},
		{name: "live reads the header", gameCode: 70, want: GameState{Started: true, Live: true, HomeScore: 61, AwayScore: 70, Quarter: 3, Clock: "04:12"}},
		{name: "empty header is not started", gameCode: 89, want: GameState{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := client.GameState(t.Context(), "E2026", tt.gameCode)
			if err != nil {
				t.Fatalf("GameState: %v", err)
			}
			if got != tt.want {
				t.Fatalf("GameState = %+v, want %+v", got, tt.want)
			}
		})
	}
}
