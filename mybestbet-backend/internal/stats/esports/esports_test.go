package esports

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func replayDeps(t *testing.T) sources.Deps {
	t.Helper()
	srv := sourcestest.ReplayServer(t, "esports", func(r *http.Request) (string, bool) {
		path := r.URL.Path
		switch {
		case path == "/dota2/api.php" && r.URL.Query().Get("page") == "Liquipedia:Matches":
			return "liquipedia_dota2_matches.json", true
		case path == "/counterstrike/api.php":
			return "liquipedia_counterstrike_matches.json", true
		case path == "/proMatches":
			return "opendota_pro_matches.json", true
		case path == "/live":
			return "opendota_live.json", true
		case path == "/teams":
			return "opendota_teams.json", true
		case path == "/teams/10149530/players":
			return "opendota_team_10149530_players.json", true
		case strings.HasPrefix(path, "/matches/"):
			return "opendota_match_" + strings.TrimPrefix(path, "/matches/") + ".json", true
		}
		return "", false
	})
	return sourcestest.Deps(srv, sources.Config{})
}

func TestUpcomingMatchesDota(t *testing.T) {
	t.Parallel()
	matches, err := NewLiquipedia(replayDeps(t)).UpcomingMatches(t.Context(), "DOTA2")
	if err != nil {
		t.Fatalf("UpcomingMatches: %v", err)
	}
	if len(matches) != 6 {
		t.Fatalf("got %d matches, want 6", len(matches))
	}
	first := matches[0]
	expected := Match{
		ID: "6vIiKKtH6I_0002", LeagueCode: "DOTA2", Kickoff: time.Unix(1792778400, 0).UTC(), BestOf: 3,
		Tournament: "NEXUS SERIES II - October 23",
		Team1:      Team{Name: "Team Synapse", ShortName: "Synapse"}, Team2: Team{Name: "Zero Tenacity", ShortName: "Z10"},
	}
	if first != expected {
		t.Fatalf("first = %+v\nwant %+v", first, expected)
	}
}

func TestUpcomingMatchesSkipsTBDAndFallsBackToKey(t *testing.T) {
	t.Parallel()
	matches, err := NewLiquipedia(replayDeps(t)).UpcomingMatches(t.Context(), "CS2")
	if err != nil {
		t.Fatalf("UpcomingMatches: %v", err)
	}
	if len(matches) != 5 {
		t.Fatalf("got %d matches, want 5 (TBD skipped)", len(matches))
	}
	if matches[0].ID != "1792724400:Lynn Vision Gaming:Chinggis Warriors" {
		t.Fatalf("id = %q", matches[0].ID)
	}
	if logo := matches[0].Team1.LogoURL; !strings.HasSuffix(logo, "/commons/images/thumb/e/e7/Lynn_Vision_Gaming_2024_allmode.png/148px-Lynn_Vision_Gaming_2024_allmode.png") || !strings.HasPrefix(logo, "http") {
		t.Fatalf("team logo = %q", logo)
	}
}

func TestUnknownGame(t *testing.T) {
	t.Parallel()
	if _, err := NewLiquipedia(replayDeps(t)).UpcomingMatches(t.Context(), "NBA"); err == nil {
		t.Fatal("expected ErrUnknownGame")
	}
}

func TestOpenDota(t *testing.T) {
	t.Parallel()
	od := NewOpenDota(replayDeps(t))
	games, err := od.ProGames(t.Context())
	if err != nil {
		t.Fatalf("ProGames: %v", err)
	}
	if len(games) < 3 || games[0].Radiant.Name == "" || games[0].Start.IsZero() {
		t.Fatalf("games = %+v", games)
	}
	game, err := od.Game(t.Context(), 9058656823)
	if err != nil {
		t.Fatalf("Game: %v", err)
	}
	if game.Dire.Name != "HULIGANI" || len(game.Players) != 10 || game.Players[0].Name != "yowaai" || game.Players[0].Kills != 12 || !game.Players[0].IsRadiant {
		t.Fatalf("game = %+v", game)
	}
	teams, err := od.Teams(t.Context())
	if err != nil || len(teams) != 6 {
		t.Fatalf("Teams = %d, %v", len(teams), err)
	}
	if teams[0].LogoURL == "" {
		t.Fatalf("team logo missing: %+v", teams[0])
	}
	roster, err := od.CurrentRoster(t.Context(), "10149530")
	if err != nil || len(roster) == 0 {
		t.Fatalf("CurrentRoster = %+v, %v", roster, err)
	}
}

func TestProGamesCarrySeriesResult(t *testing.T) {
	t.Parallel()
	games, err := NewOpenDota(replayDeps(t)).ProGames(t.Context())
	if err != nil {
		t.Fatalf("ProGames: %v", err)
	}
	first := games[0]
	if first.MatchID != 9058656823 || first.SeriesType != BestOfThree || first.RadiantWin || first.RadiantKills != 41 || first.DireKills != 42 || first.Live {
		t.Fatalf("first game = %+v", first)
	}
}

func TestLiveGamesKeepOnlyLeagueGamesBetweenTeams(t *testing.T) {
	t.Parallel()
	games, err := NewOpenDota(replayDeps(t)).LiveGames(t.Context())
	if err != nil {
		t.Fatalf("LiveGames: %v", err)
	}
	expected := []ProGame{{
		MatchID: 9062443549, Start: time.Unix(1792995306, 0).UTC(), SeriesID: 1159750, Live: true,
		Radiant: DotaTeam{ID: "9824702", Name: "PARIVISION"}, Dire: DotaTeam{ID: "8255888", Name: "BETBOOM TEAM"},
		RadiantKills: 15, DireKills: 13,
	}}
	if !slices.Equal(games, expected) {
		t.Fatalf("games = %+v\nwant %+v", games, expected)
	}
}

func TestSeriesTypeIsDecided(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		seriesType SeriesType
		home, away int
		want       bool
	}{
		{name: "bo1 after one game", seriesType: BestOfOne, home: 1, want: true},
		{name: "bo3 at 1-1", seriesType: BestOfThree, home: 1, away: 1},
		{name: "bo3 at 2-1", seriesType: BestOfThree, home: 2, away: 1, want: true},
		{name: "bo5 at 2-0", seriesType: BestOfFive, home: 2},
		{name: "bo5 at 0-3", seriesType: BestOfFive, away: 3, want: true},
		{name: "bo2 drawn", seriesType: BestOfTwo, home: 1, away: 1, want: true},
		{name: "bo2 after one game", seriesType: BestOfTwo, home: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.seriesType.IsDecided(tt.home, tt.away); got != tt.want {
				t.Fatalf("IsDecided(%d, %d) = %v, want %v", tt.home, tt.away, got, tt.want)
			}
		})
	}
}
