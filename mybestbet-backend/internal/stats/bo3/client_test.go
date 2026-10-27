package bo3

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func replayRoute(r *http.Request) (string, bool) {
	path := r.URL.Path
	switch {
	case path == "/matches":
		return "cs2_matches.json", r.URL.Query().Get("filter[matches.discipline_id][eq]") == "1" && r.URL.Query().Get("filter[matches.tier][in]") == "s,a"
	case strings.HasPrefix(path, "/matches/") && strings.HasSuffix(path, "/players_stats"):
		return "cs2_players_stats_" + strings.TrimSuffix(strings.TrimPrefix(path, "/matches/"), "/players_stats") + ".json", true
	case strings.HasPrefix(path, "/matches/"):
		return "match_" + strings.TrimPrefix(path, "/matches/") + ".json", true
	case strings.HasPrefix(path, "/teams/"):
		return "team_" + strings.TrimPrefix(path, "/teams/") + ".json", true
	case strings.HasPrefix(path, "/vlr/stats/matches/"), strings.HasPrefix(path, "/lol/stats/matches/"):
		parts := strings.Split(strings.Trim(path, "/"), "/")
		return parts[0] + "_" + parts[4] + "_" + parts[3] + ".json", len(parts) == 5
	}
	return "", false
}

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	return New(sourcestest.Deps(sourcestest.ReplayServer(t, SourceName, replayRoute), sources.Config{}))
}

func TestMatchesKeepsOnlyMatchesWithBothTeams(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	matches, err := newReplayClient(t).Matches(t.Context(), "CS2", from, from.Add(10*24*time.Hour))
	require.NoError(t, err)
	require.Len(t, matches, 6)
	assert.Equal(t, "the-mongolz-vs-private-21-10-2026", matches[0].Slug)
	assert.True(t, matches[0].IsFinished())
	assert.NotEmpty(t, matches[0].Team1.Name)
	assert.Regexp(t, `^https://files\.bo3\.gg/uploads/team/25382/image/`, matches[0].Team1.LogoURL)
}

func TestMatchesRejectsUnknownLeague(t *testing.T) {
	t.Parallel()
	_, err := newReplayClient(t).Matches(t.Context(), "DOTA2", time.Now(), time.Now())
	require.ErrorIs(t, err, ErrUnknownDiscipline)
}

func TestRosterSkipsCoaches(t *testing.T) {
	t.Parallel()
	roster, err := newReplayClient(t).Roster(t.Context(), "big")
	require.NoError(t, err)
	names := make([]string, 0, len(roster))
	for _, p := range roster {
		names = append(names, p.Name)
	}
	assert.ElementsMatch(t, []string{"tabseN", "faveN", "blameF", "JDC", "gr1ks"}, names)
}

func TestPlayerLinesPerDiscipline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		league   string
		slug     string
		player   string
		expected PlayerLine
	}{
		{name: "cs2 series totals", league: "CS2", slug: "big-vs-the-mongolz-22-10-2026", player: "JDC",
			expected: PlayerLine{Player: Player{ID: "25135", Name: "JDC"}, TeamID: "756", Kills: 46, Maps: 3}},
		{name: "valorant sums finished maps", league: "VALORANT", slug: "kru-blaze-vs-gen-g-gc-23-10-2026", player: "332",
			expected: PlayerLine{Player: Player{ID: "46704", Name: "332"}, TeamID: "8164", Kills: 65, Maps: 3}},
		{name: "lol single game", league: "LOL", slug: "los-vs-t1-lol-23-10-2026", player: "Oner",
			expected: PlayerLine{Player: Player{ID: "62339", Name: "Oner"}, TeamID: "17529", Kills: 8, Maps: 1}},
	}
	client := newReplayClient(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			match, err := client.Match(t.Context(), tt.slug)
			require.NoError(t, err)
			lines, err := client.PlayerLines(t.Context(), tt.league, match)
			require.NoError(t, err)
			require.Len(t, lines, 10)
			for _, l := range lines {
				if l.Player.Name == tt.player {
					assert.Equal(t, tt.expected, l)
					return
				}
			}
			t.Fatalf("player %s missing from %+v", tt.player, lines)
		})
	}
}
