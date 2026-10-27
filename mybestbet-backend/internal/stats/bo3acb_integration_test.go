package stats

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/seed"
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/canon"
)

func bo3StatsRoute(r *http.Request) (string, bool) {
	path := r.URL.Path
	switch {
	case path == "/matches":
		return "cs2_matches.json", r.URL.Query().Get("filter[matches.discipline_id][eq]") == "1"
	case strings.HasPrefix(path, "/matches/") && strings.HasSuffix(path, "/players_stats"):
		return "cs2_players_stats_" + strings.TrimSuffix(strings.TrimPrefix(path, "/matches/"), "/players_stats") + ".json", true
	case strings.HasPrefix(path, "/matches/"):
		return "match_" + strings.TrimPrefix(path, "/matches/") + ".json", true
	case strings.HasPrefix(path, "/vlr/stats/matches/"), strings.HasPrefix(path, "/lol/stats/matches/"):
		parts := strings.Split(strings.Trim(path, "/"), "/")
		return parts[0] + "_" + parts[4] + "_" + parts[3] + ".json", len(parts) == 5
	}
	return "", false
}

func acbStatsRoute(r *http.Request) (string, bool) {
	switch r.URL.Path {
	case "/es/liga":
		return "liga.html", true
	case "/_next/static/chunks/0cz1d0mv5g_q7.js":
		return "chunk_other.js", true
	case "/_next/static/chunks/2z438bai2hw49.js":
		return "chunk_key.js", true
	case "/api/matchdata/Result/boxscores":
		return "boxscores_105410.json", r.URL.Query().Get("matchId") == "105410"
	}
	return "", false
}

func statsByPlayerName(t *testing.T, f sportFixture, eventID string) map[string]MatchStat {
	t.Helper()
	stats, err := f.store.EventMatchStats(t.Context(), eventID)
	require.NoError(t, err)
	byName := make(map[string]MatchStat, len(stats))
	for _, s := range stats {
		var name string
		require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT name FROM players WHERE id = $1`, s.PlayerID).Scan(&name))
		byName[name] = s
	}
	return byName
}

func TestCounterStrikeKillsFromLiquipediaEventMatchedOnBo3(t *testing.T) {
	f := openSportFixture(t)
	league := seed.League{Code: "CS2", Name: "CS2", Country: "International", Sport: canon.SportEsports}
	e := f.event(t, league, esports.LiquipediaSource, "CS2:fixture-big-mongolz", time.Date(2026, 10, 22, 11, 0, 0, 0, time.UTC),
		seed.SourceTeam{Source: esports.LiquipediaSource, SourceID: "CS2:BIG", LeagueCode: league.Code, Name: "BIG"},
		seed.SourceTeam{Source: esports.LiquipediaSource, SourceID: "CS2:The MongolZ", LeagueCode: league.Code, Name: "The MongolZ"})

	collectTwice(t, newSportService(t, f), e)

	byName := statsByPlayerName(t, f, e.ID)
	require.Len(t, byName, 10)
	jdc := byName["JDC"]
	assert.Equal(t, statkey.Values{statkey.Kills: 46, statkey.Maps: 3}, jdc.Stats)
	assert.True(t, jdc.IsHome)
	assert.False(t, byName["910"].IsHome)

	rates, err := NewReader(f.pool).HitRates(t.Context(), jdc.PlayerID, canon.PropMapKills, 14.5, []int{5})
	require.NoError(t, err)
	assert.Equal(t, HitRate{Window: 5, Matches: 1, Hits: 1, Rate: 1}, rates.Windows[0])
}

func TestValorantAndLoLKillsFromBo3Events(t *testing.T) {
	tests := []struct {
		name     string
		league   seed.League
		slug     string
		home     seed.SourceTeam
		away     seed.SourceTeam
		player   string
		expected statkey.Values
		isHome   bool
	}{
		{
			name:   "valorant series",
			league: seed.League{Code: "VALORANT", Name: "Valorant", Country: "International", Sport: canon.SportEsports},
			slug:   "kru-blaze-vs-gen-g-gc-23-10-2026",
			home:   seed.SourceTeam{Source: bo3.SourceName, SourceID: "6593", LeagueCode: "VALORANT", Name: "KRÜ Blaze"},
			away:   seed.SourceTeam{Source: bo3.SourceName, SourceID: "8164", LeagueCode: "VALORANT", Name: "Gen.G GC"},
			player: "332", expected: statkey.Values{statkey.Kills: 65, statkey.Maps: 3},
		},
		{
			name:   "lol single game",
			league: seed.League{Code: "LOL", Name: "League of Legends", Country: "International", Sport: canon.SportEsports},
			slug:   "los-vs-t1-lol-23-10-2026",
			home:   seed.SourceTeam{Source: bo3.SourceName, SourceID: "16923", LeagueCode: "LOL", Name: "LOS"},
			away:   seed.SourceTeam{Source: bo3.SourceName, SourceID: "17529", LeagueCode: "LOL", Name: "T1"},
			player: "Oner", expected: statkey.Values{statkey.Kills: 8, statkey.Maps: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := openSportFixture(t)
			e := f.event(t, tt.league, bo3.SourceName, seed.Bo3EventSourceID(tt.league.Code, tt.slug), time.Date(2026, 10, 23, 20, 30, 0, 0, time.UTC), tt.home, tt.away)

			collectTwice(t, newSportService(t, f), e)

			byName := statsByPlayerName(t, f, e.ID)
			require.Len(t, byName, 10)
			assert.Equal(t, tt.expected, byName[tt.player].Stats)
			assert.Equal(t, tt.isHome, byName[tt.player].IsHome)
		})
	}
}

func TestACBBoxscoreStatsAndHitRates(t *testing.T) {
	f := openSportFixture(t)
	league := seed.League{Code: acb.LeagueCode, Name: "Liga ACB", Country: "Spain", Sport: canon.SportBasketball}
	e := f.event(t, league, acb.SourceName, "105410", time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC),
		seed.SourceTeam{Source: acb.SourceName, SourceID: "8", LeagueCode: league.Code, Name: "Asisa Joventut"},
		seed.SourceTeam{Source: acb.SourceName, SourceID: "2", LeagueCode: league.Code, Name: "Barça"})

	collectTwice(t, newSportService(t, f), e)

	byName := statsByPlayerName(t, f, e.ID)
	require.Len(t, byName, 23)
	rubio := byName["Ricky Rubio"]
	assert.Equal(t, statkey.Values{statkey.Points: 19, statkey.Rebounds: 3, statkey.Assists: 6, statkey.Threes: 3}, rubio.Stats)
	assert.True(t, rubio.IsHome)
	assert.Equal(t, 22, rubio.Minutes)
	assert.False(t, byName["Kevin Punter"].IsHome)

	rates, err := NewReader(f.pool).HitRates(t.Context(), rubio.PlayerID, canon.PropPoints, 15.5, []int{5})
	require.NoError(t, err)
	assert.Equal(t, HitRate{Window: 5, Matches: 1, Hits: 1, Rate: 1}, rates.Windows[0])
}
