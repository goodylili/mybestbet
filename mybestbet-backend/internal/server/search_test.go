package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/names"
	"mybestbet/pkg/canon"
)

func TestSearchAndLiveScoreMatchContract(t *testing.T) {
	pool, rdb := integrationDeps(t)
	f := seedContractFixture(t, pool, rdb)
	s := newTestServer(t, pool, rdb, f)
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)
	token := userToken(t, s, pool)
	spec := loadSpec(t)
	ctx := context.Background()

	marker := "zq" + uuid.NewString()[:6]
	homeName, awayName, playerName := "Xylo "+marker+" Rovers FC", "Opponent "+marker+" United", "Quinn "+marker
	insertID := func(sql string, args ...any) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, sql, args...).Scan(&id))
		return id
	}
	insertTeam := func(name string) string {
		return insertID(`INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $2) RETURNING id::text`, name, names.Team(name))
	}
	homeID, awayID := insertTeam(homeName), insertTeam(awayName)
	playerID := insertID(`INSERT INTO players (name, "normalizedName", "teamId") VALUES ($1, $2, $3) RETURNING id::text`, playerName, names.Person(playerName).Full(), homeID)
	upcomingID := insertID(`INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff, status, score) VALUES ('EPL', $1, $2, $3, 'live', '{"home":1,"away":0,"period":"2H","minute":67,"clock":"67"}') RETURNING id::text`,
		homeID, awayID, time.Now().Add(-30*time.Minute).Truncate(time.Second))
	pastID := insertID(`INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff, status) VALUES ('EPL', $1, $2, $3, 'finished') RETURNING id::text`,
		awayID, homeID, time.Now().Add(-72*time.Hour).Truncate(time.Second))
	_, err := pool.Exec(ctx, `INSERT INTO logos (kind, "entityId", "sourceUrl", "contentType", sha256, bytes, "fetchedAt") VALUES ('team', $1, 'https://contract.test/logo.svg', 'image/svg+xml', $2, $3, now())`,
		homeID, contractLogoSHA, []byte(contractLogoSVG))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM logos WHERE kind = 'team' AND "entityId" = $1`, homeID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM events WHERE id = ANY($1::uuid[])`, []string{upcomingID, pastID})
		_, _ = pool.Exec(context.Background(), `DELETE FROM players WHERE id = $1`, playerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM teams WHERE id = ANY($1::uuid[])`, []string{homeID, awayID})
	})

	cases := []contractCase{
		{name: "search finds team, event and player", method: "get", template: "/v1/search", path: "/v1/search?q=xylo+" + marker, status: 200, check: func(t *testing.T, body map[string]any) {
			team := findByID(t, body["items"], homeID)
			assert.Equal(t, "team", team["kind"])
			assert.Equal(t, "football", team["sport"])
			assert.Equal(t, "/v1/logos/team/"+homeID+contractLogoURL, team["logoUrl"])
			event := findByID(t, body["items"], upcomingID)
			assert.Equal(t, "event", event["kind"])
			assert.Equal(t, homeName+" vs "+awayName, event["name"])
			assert.Equal(t, "/v1/logos/team/"+homeID+contractLogoURL, event["logoUrl"])
			assert.NotContains(t, event, "awayLogoUrl")
			assert.NotNil(t, event["kickoff"])
			assert.Nil(t, findOptional(body["items"], func(item map[string]any) bool { return item["id"] == pastID }))
		}},
		{name: "search ranks prefix team first", method: "get", template: "/v1/search", path: "/v1/search?q=xylo+" + marker + "&limit=1", status: 200, check: func(t *testing.T, body map[string]any) {
			items := asSlice(body["items"])
			require.Len(t, items, 1)
			assert.Equal(t, homeID, asMap(items[0])["id"])
		}},
		{name: "search player by partial name", method: "get", template: "/v1/search", path: "/v1/search?q=Quinn+" + marker[:5], status: 200, check: func(t *testing.T, body map[string]any) {
			player := findByID(t, body["items"], playerID)
			assert.Equal(t, "player", player["kind"])
			assert.Equal(t, "EPL", player["leagueCode"])
			assert.Equal(t, "/v1/logos/team/"+homeID+contractLogoURL, player["logoUrl"])
		}},
		{name: "search accents and case folded", method: "get", template: "/v1/search", path: "/v1/search?q=X%C3%BFLO+" + marker, status: 200, check: func(t *testing.T, body map[string]any) {
			findByID(t, body["items"], homeID)
		}},
		{name: "search with another sport", method: "get", template: "/v1/search", path: "/v1/search?q=xylo+" + marker + "&sport=tennis", status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Empty(t, body["items"])
		}},
		{name: "search preferring another sport keeps every result", method: "get", template: "/v1/search", path: "/v1/search?q=xylo+" + marker + "&prefer=tennis", status: 200, check: func(t *testing.T, body map[string]any) {
			findByID(t, body["items"], homeID)
			findByID(t, body["items"], upcomingID)
		}},
		{name: "search with unknown preferred sport", method: "get", template: "/v1/search", path: "/v1/search?q=xylo+" + marker + "&prefer=curling", status: 400},
		{name: "event carries live score", method: "get", template: "/v1/events/{id}", path: "/v1/events/" + upcomingID, status: 200, check: func(t *testing.T, body map[string]any) {
			event := asMap(body["event"])
			assert.Equal(t, "live", event["status"])
			assert.Equal(t, map[string]any{"home": 1.0, "away": 0.0, "period": "2H", "minute": 67.0, "clock": "67"}, event["score"])
		}},
		{name: "event without score omits it", method: "get", template: "/v1/events/{id}", path: "/v1/events/" + f.eventID, status: 200, check: func(t *testing.T, body map[string]any) {
			assert.NotContains(t, asMap(body["event"]), "score")
		}},
	}
	runContractCases(t, httpServer.URL, spec, token, cases)
}

func TestSortSearchResultsPutsPreferredSportFirst(t *testing.T) {
	t.Parallel()
	item := func(id string, sport canon.Sport, rank float64) rankedSearchItem {
		return rankedSearchItem{SearchItem: SearchItem{ID: id, Sport: sport}, rank: rank}
	}
	tests := []struct {
		name   string
		prefer canon.Sport
		want   []string
	}{
		{name: "no preference ranks by match", prefer: "", want: []string{"nba", "epl", "liga"}},
		{name: "preferred sport first, each part by match", prefer: canon.SportFootball, want: []string{"epl", "liga", "nba"}},
		{name: "preferred sport with no results changes nothing", prefer: canon.SportTennis, want: []string{"nba", "epl", "liga"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ranked := []rankedSearchItem{item("epl", canon.SportFootball, 2), item("nba", canon.SportBasketball, 3), item("liga", canon.SportFootball, 1)}
			sortSearchResults(ranked, tt.prefer)
			got := make([]string, len(ranked))
			for i, r := range ranked {
				got[i] = r.ID
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
