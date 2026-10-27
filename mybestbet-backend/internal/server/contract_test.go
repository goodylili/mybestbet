package server

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/auth"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/internal/review"
	"mybestbet/internal/stats"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/fake"
)

const (
	testAdminToken  = "admin-test-token"
	contractLogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`
	contractLogoSHA = "c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00c0ffee00"
)

var contractLogoURL = "?v=" + contractLogoSHA[:12]

type fakeStats struct {
	opponentIDs []string
}

func (fakeStats) PlayerStats(context.Context, string, int) ([]stats.MatchStat, error) {
	rating := 7.1
	return []stats.MatchStat{{EventID: uuid.NewString(), Sport: canon.SportFootball, IsHome: true, PlayedAt: time.Now().Add(-72 * time.Hour), Minutes: 90, Started: true, Shots: 3, Rating: &rating}}, nil
}

func (f fakeStats) Histories(_ context.Context, playerIDs []string) (map[string]stats.History, error) {
	rating := 7.1
	var matches []stats.MatchStat
	for i, opponentID := range f.opponentIDs {
		matches = append(matches, stats.MatchStat{EventID: uuid.NewString(), Sport: canon.SportFootball, OpponentTeamID: opponentID, IsHome: i%2 == 0,
			PlayedAt: time.Now().Add(-time.Duration(i+1) * 72 * time.Hour), Minutes: 90, Started: true, Shots: 3, Rating: &rating})
	}
	histories := make(map[string]stats.History, len(playerIDs))
	for _, id := range playerIDs {
		histories[id] = stats.NewHistory(id, matches)
	}
	return histories, nil
}

func (fakeStats) LineupStatuses(_ context.Context, _ string, playerIDs []string) (map[string]stats.LineupStatus, error) {
	statuses := make(map[string]stats.LineupStatus, len(playerIDs))
	for _, id := range playerIDs {
		statuses[id] = stats.LineupBench
	}
	return statuses, nil
}

type fakeReview struct {
	item review.Item
}

func (f fakeReview) List(context.Context, string, int) ([]review.Item, error) {
	return []review.Item{f.item}, nil
}

func (f fakeReview) Decide(_ context.Context, d review.Decision) (review.Item, error) {
	switch {
	case d.ReviewID != f.item.ID:
		return review.Item{}, review.ErrNotFound
	case d.Action == review.ActionIgnore:
		return review.Item{}, review.ErrAlreadyDecided
	case d.Action == review.ActionLink && d.TargetID == "":
		return review.Item{}, review.ErrInvalidDecision
	}
	decided := f.item
	decided.Status = review.StatusLinked
	return decided, nil
}

type contractFixture struct {
	eventID      string
	playerID     string
	propKey      string
	teamShotsKey string
	ouKey        string
	dedupeKey    string
	nbaEventID   string
	nbaPropKey   string
	nbaSpreadKey string
	nbaMoneyline string
	nbaPlayerID  string
	reviewItem   review.Item
	homeTeamID   string
	teamIDs      []string
}

func integrationDeps(t *testing.T) (*pgxpool.Pool, *goredis.Client) {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL are required")
	}
	ctx := context.Background()
	pool, err := postgres.Open(ctx, dbURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	rdb, err := redis.Open(ctx, redisURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	return pool, rdb
}

func seedContractFixture(t *testing.T, pool *pgxpool.Pool, rdb *goredis.Client) contractFixture {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	var teamIDs, eventIDs, playerIDs []string
	f := contractFixture{dedupeKey: "contract:" + suffix}
	exec := func(sql string, args ...any) {
		_, err := pool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	insertID := func(sql string, args ...any) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, sql, args...).Scan(&id))
		return id
	}
	seedEvent := func(league string) (eventID, playerID string) {
		insertTeam := func(name string) string {
			id := insertID(`INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ($1, $2, $2) RETURNING id::text`, league, name+" "+league+" "+suffix)
			teamIDs = append(teamIDs, id)
			return id
		}
		homeID, awayID := insertTeam("Home"), insertTeam("Away")
		eventID = insertID(`INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ($1, $2, $3, $4) RETURNING id::text`,
			league, homeID, awayID, time.Now().Add(time.Hour).Truncate(time.Second))
		playerID = insertID(`INSERT INTO players (name, "normalizedName", "teamId") VALUES ($1, $1, $2) RETURNING id::text`, "Player "+league+" "+suffix, homeID)
		eventIDs, playerIDs = append(eventIDs, eventID), append(playerIDs, playerID)
		return eventID, playerID
	}
	exec(`INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	exec(`INSERT INTO leagues (code, name, country, sport) VALUES ('NBA', 'NBA', 'USA', 'basketball') ON CONFLICT DO NOTHING`)
	f.eventID, f.playerID = seedEvent("EPL")
	f.homeTeamID, f.teamIDs = teamIDs[0], teamIDs
	exec(`UPDATE players SET position = 'FW' WHERE id = $1`, f.playerID)
	f.nbaEventID, f.nbaPlayerID = seedEvent("NBA")
	f.nbaMoneyline = canon.MarketKey{EventID: f.nbaEventID, Code: canon.CodeMoneyline}.String()
	f.nbaSpreadKey = canon.MarketKey{EventID: f.nbaEventID, Code: canon.CodeSpread, Line: canon.Line(-2.5), Period: canon.PeriodQuarter1}.String()
	f.nbaPropKey = canon.MarketKey{EventID: f.nbaEventID, Code: canon.PropPoints, Line: canon.Line(24.5), PlayerID: f.nbaPlayerID}.String()

	oneXTwo := oneXTwoKey(f.eventID)
	f.ouKey = canon.MarketKey{EventID: f.eventID, Code: canon.CodeOU, Line: canon.Line(2.5)}.String()
	f.propKey = canon.MarketKey{EventID: f.eventID, Code: canon.PropRating, Line: canon.Line(7), PlayerID: f.playerID}.String()
	f.teamShotsKey = canon.MarketKey{EventID: f.eventID, Code: canon.TeamShots, Line: canon.Line(11.5), TeamID: f.homeTeamID}.String()
	observed := time.Now().UTC().Truncate(time.Millisecond)
	exec(`INSERT INTO prices_current ("marketKey", source, side, label, "eventId", code, raw, form, prob, "observedAt") VALUES ($1, 'bet9ja', 'home', '', $2, '1X2', 2.1, 'decimal', 0.476, $3)`, oneXTwo, f.eventID, observed)
	exec(`INSERT INTO prices_history ("marketKey", source, side, label, "eventId", raw, form, prob, "observedAt") VALUES ($1, 'bet9ja', 'over', '', $2, 1.9, 'decimal', 0.526, $3)`, f.ouKey, f.eventID, observed)
	legs := `[{"source":"bet9ja","side":"home","price":{"raw":2.1,"form":"decimal","prob":0.476,"observedAt":"2026-10-23T10:00:00Z"},"stake":48,"observedAt":"2026-10-23T10:00:00Z"}]`
	exec(`INSERT INTO signals ("dedupeKey", type, "eventId", "leagueCode", "marketKey", margin, legs, "oldestLegAgeMs", "detectedAt") VALUES ($1, 'arb', $2, 'EPL', $3, 0.021, $4, 1500, now())`,
		f.dedupeKey, f.eventID, oneXTwo, legs)
	moveLegs := `[{"source":"bet9ja","marketKey":"` + oneXTwo + `","side":"home","price":{"raw":2.0,"form":"decimal","prob":0.5,"observedAt":"2026-10-23T09:50:00Z"},"observedAt":"2026-10-23T09:50:00Z"},` +
		`{"source":"bet9ja","marketKey":"` + oneXTwo + `","side":"home","price":{"raw":1.6,"form":"decimal","prob":0.625,"observedAt":"2026-10-23T10:00:00Z"},"observedAt":"2026-10-23T10:00:00Z"}]`
	exec(`INSERT INTO signals ("dedupeKey", type, "eventId", "leagueCode", "marketKey", margin, legs, "oldestLegAgeMs", "detectedAt") VALUES ($1, 'move', $2, 'EPL', $3, 0.25, $4, 0, now())`,
		f.dedupeKey+"|move", f.eventID, oneXTwo, moveLegs)
	suspendedPropKey := canon.MarketKey{EventID: f.eventID, Code: canon.PropShots, Line: canon.Line(1.5), PlayerID: f.playerID}.String()
	for _, prop := range []struct {
		key         string
		code        canon.MarketCode
		isSuspended bool
	}{{f.propKey, canon.PropRating, false}, {suspendedPropKey, canon.PropShots, true}} {
		exec(`INSERT INTO prices_current ("marketKey", source, side, label, "eventId", code, "playerId", raw, form, prob, suspended, "observedAt") VALUES ($1, 'bet9ja', 'yes', '', $2, $3, $4, 1.8, 'decimal', 0.556, $5, $6)`,
			prop.key, f.eventID, prop.code, f.playerID, prop.isSuspended, observed)
	}

	fair := 0.47
	cells := map[string][]canon.BoardCell{
		oneXTwo: {
			decimalCell(oneXTwo, "bet9ja", canon.SideHome, "", 2.1),
			decimalCell(oneXTwo, "betano", canon.SideDraw, "", 3.4),
			{MarketKey: oneXTwo, Source: consensusSource, Side: canon.SideHome, Price: canon.NewPrice(2.12, canon.FormDecimal, observed), Fair: &fair},
		},
		f.ouKey: {
			{MarketKey: f.ouKey, Source: "kalshi", Side: canon.SideOver, Price: canon.NewBookPrice(new(0.5), new(0.52), new(800.0), canon.FormShare, observed)},
		},
		f.propKey: {
			decimalCell(f.propKey, "bayse", canon.SideYes, "", 1.8),
			decimalCell(f.propKey, "bayse", canon.SideNo, "", 2.0),
		},
		f.teamShotsKey: {
			decimalCell(f.teamShotsKey, "betano", canon.SideOver, "", 1.9),
		},
		f.nbaMoneyline: {
			decimalCell(f.nbaMoneyline, "betway", canon.SideHome, "", 1.7),
			decimalCell(f.nbaMoneyline, "betking", canon.SideHome, "", 1.75),
			decimalCell(f.nbaMoneyline, "betway", canon.SideAway, "", 2.2),
		},
		f.nbaSpreadKey: {
			decimalCell(f.nbaSpreadKey, "betway", canon.SideHome, "", 1.9),
			decimalCell(f.nbaSpreadKey, "betway", canon.SideAway, "", 1.9),
		},
		f.nbaPropKey: {
			decimalCell(f.nbaPropKey, "betking", canon.SideOver, "", 1.85),
			decimalCell(f.nbaPropKey, "betking", canon.SideUnder, "", 1.95),
		},
	}
	for marketKey, marketCells := range cells {
		key, err := canon.ParseMarketKey(marketKey)
		require.NoError(t, err)
		require.NoError(t, rdb.SAdd(ctx, redis.EventMarketsKey(key.EventID), marketKey).Err())
		for _, cell := range marketCells {
			encoded, err := json.Marshal(cell)
			require.NoError(t, err)
			require.NoError(t, rdb.HSet(ctx, redis.BoardKey(marketKey), redis.BoardField(cell.Source, string(cell.Side), cell.Label), encoded).Err())
		}
	}
	require.NoError(t, rdb.HSet(ctx, redis.SourceHealthKey("bet9ja"), map[string]any{
		"lastSuccess": observed.Format(time.RFC3339Nano), "lastError": "", "consecutiveFailures": 0, "degraded": "0", "requests": 10, "errors": 1, "lastFetchMs": 120, "freshnessMs": 900,
	}).Err())

	logoRows := [][2]string{{"team", f.homeTeamID}, {"league", "NBA"}, {"source", "bet9ja"}}
	for _, row := range logoRows {
		exec(`INSERT INTO logos (kind, "entityId", "sourceUrl", "contentType", sha256, bytes, "fetchedAt") VALUES ($1, $2, 'https://contract.test/logo.svg', 'image/svg+xml', $3, $4, now())
			ON CONFLICT (kind, "entityId") DO UPDATE SET "sourceUrl" = EXCLUDED."sourceUrl", "contentType" = EXCLUDED."contentType", sha256 = EXCLUDED.sha256, bytes = EXCLUDED.bytes`,
			row[0], row[1], contractLogoSHA, []byte(contractLogoSVG))
	}

	f.reviewItem = review.Item{ID: uuid.NewString(), Kind: "team", Source: "bet9ja", SourceKey: "Man Utd " + suffix, Payload: []byte(`{"name":"Man Utd"}`), Status: review.StatusPending, CreatedAt: observed}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM signals WHERE "dedupeKey" = ANY($1)`, []string{f.dedupeKey, f.dedupeKey + "|move"})
		_, _ = pool.Exec(ctx, `DELETE FROM prices_current WHERE "eventId" = $1`, f.eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM prices_history WHERE "eventId" = $1`, f.eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM players WHERE id = ANY($1::uuid[])`, playerIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE id = ANY($1::uuid[])`, eventIDs)
		_, _ = pool.Exec(ctx, `DELETE FROM teams WHERE id = ANY($1::uuid[])`, teamIDs)
		for _, row := range logoRows {
			_, _ = pool.Exec(ctx, `DELETE FROM logos WHERE kind = $1 AND "entityId" = $2 AND "sourceUrl" = 'https://contract.test/logo.svg'`, row[0], row[1])
		}
		keys := []string{redis.EventMarketsKey(f.eventID), redis.EventMarketsKey(f.nbaEventID)}
		for marketKey := range cells {
			keys = append(keys, redis.BoardKey(marketKey))
		}
		_ = rdb.Del(ctx, keys...).Err()
	})
	return f
}

func newTestServer(t *testing.T, pool *pgxpool.Pool, rdb *goredis.Client, f contractFixture) *Server {
	t.Helper()
	for _, name := range []string{"bet9ja", "kalshi", fake.Name} {
		sources.Register(name, fake.New)
	}
	cfg := config.Config{
		AdminToken: testAdminToken,
		Leagues: []config.League{
			{Code: "EPL", Name: "Premier League", Country: "England", Sport: canon.SportFootball, Group: "domestic", Order: 1},
			{Code: "LALIGA", Name: "La Liga", Country: "Spain", Sport: canon.SportFootball, Group: "domestic", Order: 2},
			{Code: "NBA", Name: "NBA", Country: "USA", Sport: canon.SportBasketball, Group: "domestic", Order: 1},
		},
		Sources:  map[string]sources.Config{"bet9ja": {Enabled: true}, "kalshi": {Enabled: true}, fakeSourceName: {}},
		Feeds:    map[string]string{"bet9ja": "bet9ja-feed"},
		Profiles: map[string]config.SourceProfile{"bet9ja": {Type: "sportsbook", Note: "contract note", Countries: []string{"NG"}}, "kalshi": {Type: "prediction", BlockedCountries: []string{"GB"}}},
		Env:      "test",
		Auth:     testAuthConfig(),
	}
	authService := newTestAuth(t, cfg, pool, rdb, newResendMock(t).URL, auth.GoogleEndpoints{})
	s := New(Deps{Config: cfg, Pool: pool, Redis: rdb, Stats: fakeStats{opponentIDs: f.teamIDs}, Review: fakeReview{item: f.reviewItem}, Auth: authService, Log: slog.Default()})
	snapshot, err := s.loadCoverage(t.Context())
	require.NoError(t, err)
	s.coverage.snapshot = snapshot
	require.NoError(t, s.refreshEventIndex(t.Context()))
	require.NoError(t, s.refreshSignals(t.Context()))
	require.NoError(t, s.refreshSources(t.Context()))
	return s
}

type contractCase struct {
	name      string
	method    string
	template  string
	path      string
	body      string
	token     string
	anonymous bool
	status    int
	check     func(t *testing.T, body map[string]any)
}

func TestHandlersMatchOpenAPIContract(t *testing.T) {
	pool, rdb := integrationDeps(t)
	f := seedContractFixture(t, pool, rdb)
	s := newTestServer(t, pool, rdb, f)
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)
	token := userToken(t, s, pool)
	spec := loadSpec(t)
	from := url.QueryEscape(time.Now().Add(-time.Hour).Format(time.RFC3339))
	to := url.QueryEscape(time.Now().Add(3 * time.Hour).Format(time.RFC3339))
	decision := func(action, target string) string {
		encoded, _ := json.Marshal(review.Decision{ReviewID: f.reviewItem.ID, Action: action, TargetID: target})
		return string(encoded)
	}

	cases := []contractCase{
		{name: "sports", method: "get", template: "/v1/sports", path: "/v1/sports", status: 200, check: func(t *testing.T, body map[string]any) {
			items := asSlice(body["items"])
			require.Len(t, items, 2)
			assert.Equal(t, "football", asMap(items[0])["sport"])
			basketball := asMap(items[1])
			assert.Equal(t, "basketball", basketball["sport"])
			assert.Contains(t, basketball["playerProps"], "BK_PTS")
			markets := asSlice(basketball["markets"])
			moneyline := asMap(markets[0])
			assert.Equal(t, "ML", moneyline["code"])
			assert.Equal(t, "Match winner", moneyline["label"])
			assert.Equal(t, "two_way", moneyline["shape"])
			assert.Contains(t, moneyline["periods"], "Q1")
			assert.Nil(t, moneyline["statField"])
			assert.Equal(t, false, moneyline["regulationOnly"])
			points := findByID(t, markets, "", func(item map[string]any) bool { return item["code"] == "BK_PTS" })
			assert.Equal(t, "points", points["statField"])
			assert.Equal(t, "player", points["kind"])
			football := asMap(items[0])
			assert.NotNil(t, findOptional(football["markets"], func(item map[string]any) bool { return item["code"] == "QUALIFY" }))
			assert.Equal(t, true, asMap(asSlice(football["markets"])[0])["regulationOnly"])
			assert.Equal(t, "NBA", asMap(asSlice(basketball["leagues"])[0])["code"])
			assert.Equal(t, "/v1/logos/league/NBA"+contractLogoURL, asMap(asSlice(basketball["leagues"])[0])["logoUrl"])
		}},
		{name: "leagues", method: "get", template: "/v1/leagues", path: "/v1/leagues", status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Len(t, body["items"], 3)
			laliga := findByID(t, body["items"], "", func(item map[string]any) bool { return item["code"] == "LALIGA" })
			assert.Equal(t, "domestic", laliga["group"])
			assert.InDelta(t, 2, laliga["order"], 0)
		}},
		{name: "leagues of one sport", method: "get", template: "/v1/leagues", path: "/v1/leagues?sport=basketball", status: 200, check: func(t *testing.T, body map[string]any) {
			items := asSlice(body["items"])
			require.Len(t, items, 1)
			assert.Equal(t, "basketball", asMap(items[0])["sport"])
		}},
		{name: "leagues unknown sport", method: "get", template: "/v1/leagues", path: "/v1/leagues?sport=curling", status: 400},
		{name: "events of one sport", method: "get", template: "/v1/events", path: "/v1/events?sport=basketball&from=" + from + "&to=" + to, status: 200, check: func(t *testing.T, body map[string]any) {
			summary := findByID(t, body["items"], f.nbaEventID)
			assert.Equal(t, "basketball", summary["sport"])
			best := asMap(summary["best"])
			assert.Equal(t, "ML", best["market"])
			assert.Equal(t, "betking", asMap(best["home"])["source"])
			assert.Nil(t, best["draw"])
			assert.Nil(t, findOptional(body["items"], func(item map[string]any) bool { return item["id"] == f.eventID }))
		}},
		{name: "basketball event board", method: "get", template: "/v1/events/{id}", path: "/v1/events/" + f.nbaEventID, status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Equal(t, "basketball", asMap(body["event"])["sport"])
			tabs := asSlice(body["markets"])
			require.Len(t, tabs, 2)
			assert.Equal(t, "ML", asMap(tabs[0])["code"])
			spread := asMap(tabs[1])
			assert.Equal(t, "handicap", spread["shape"])
			assert.Equal(t, "Q1", asMap(asSlice(spread["periods"])[0])["period"])
		}},
		{name: "basketball props", method: "get", template: "/v1/events/{id}/props", path: "/v1/events/" + f.nbaEventID + "/props", status: 200, check: func(t *testing.T, body map[string]any) {
			rows := asSlice(body["rows"])
			require.Len(t, rows, 1)
			row := asMap(rows[0])
			assert.Equal(t, f.nbaPropKey, row["keyString"])
			assert.NotEmpty(t, asMap(row["research"])["windows"])
			assert.NotContains(t, row, "hitRate")
			player := asMap(asSlice(body["players"])[0])
			assert.Contains(t, row["keyString"], player["id"])
			assert.Nil(t, player["ratings"])
		}},
		{name: "signals of another sport", method: "get", template: "/v1/signals", path: "/v1/signals?sport=basketball", status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Nil(t, findOptional(body["items"], func(item map[string]any) bool { return item["dedupeKey"] == f.dedupeKey }))
		}},
		{name: "events in window", method: "get", template: "/v1/events", path: "/v1/events?league=EPL&from=" + from + "&to=" + to, status: 200, check: func(t *testing.T, body map[string]any) {
			summary := findByID(t, body["items"], f.eventID)
			assert.Equal(t, []any{"bayse", "bet9ja", "betano", "kalshi"}, summary["sources"])
			assert.InDelta(t, 1, summary["propCount"], 0)
			assert.Equal(t, "football", summary["sport"])
			assert.Equal(t, "bet9ja", asMap(asMap(summary["best"])["home"])["source"])
			assert.InDelta(t, 1, asMap(summary["badges"])["arbs"], 0)
			assert.Equal(t, "/v1/logos/team/"+f.homeTeamID+contractLogoURL, asMap(summary["home"])["logoUrl"])
			assert.Nil(t, asMap(summary["away"])["logoUrl"])
		}},
		{name: "events bad window", method: "get", template: "/v1/events", path: "/v1/events?to=yesterday", status: 400},
		{name: "events best of a chosen market", method: "get", template: "/v1/events", path: "/v1/events?league=EPL&market=OU&line=2.5&from=" + from + "&to=" + to, status: 200, check: func(t *testing.T, body map[string]any) {
			best := asMap(findByID(t, body["items"], f.eventID)["best"])
			assert.Equal(t, "OU", best["market"])
			assert.InDelta(t, 2.5, best["line"], 0)
			assert.Equal(t, "kalshi", asMap(best["over"])["source"])
			assert.Nil(t, best["home"])
		}},
		{name: "events market without its line", method: "get", template: "/v1/events", path: "/v1/events?market=OU", status: 400},
		{name: "events unsupported market", method: "get", template: "/v1/events", path: "/v1/events?market=CS", status: 400},
		{name: "event calendar", method: "get", template: "/v1/events/calendar", path: "/v1/events/calendar?league=EPL&from=" + from + "&to=" + to, status: 200, check: func(t *testing.T, body map[string]any) {
			day := findByID(t, body["days"], "", func(item map[string]any) bool {
				return item["date"] == time.Now().Add(time.Hour).UTC().Format(time.DateOnly)
			})
			assert.GreaterOrEqual(t, day["count"], 1.0)
		}},
		{name: "event calendar in a time zone", method: "get", template: "/v1/events/calendar", path: "/v1/events/calendar?sport=basketball&tz=Africa/Lagos", status: 200, check: func(t *testing.T, body map[string]any) {
			assert.NotEmpty(t, body["days"])
		}},
		{name: "event calendar bad time zone", method: "get", template: "/v1/events/calendar", path: "/v1/events/calendar?tz=Mars/Olympus", status: 400},
		{name: "board stream for one event", method: "get", template: "/v1/stream", path: "/v1/stream?scope=board&event=" + f.eventID, status: 400},
		{name: "stream unknown scope", method: "get", template: "/v1/stream", path: "/v1/stream?scope=everything", status: 400},
		{name: "event board", method: "get", template: "/v1/events/{id}", path: "/v1/events/" + f.eventID, status: 200, check: func(t *testing.T, body map[string]any) {
			assert.NotEmpty(t, asMap(asMap(body["event"])["home"])["logoUrl"])
			tabs := asSlice(body["markets"])
			require.Len(t, tabs, 3)
			assert.Equal(t, "1X2", asMap(tabs[0])["code"])
			assert.Equal(t, "OU", asMap(tabs[1])["code"])
			teamShots := asMap(asSlice(asMap(asSlice(asMap(tabs[2])["periods"])[0])["boards"])[0])
			assert.Equal(t, f.homeTeamID, asMap(teamShots["key"])["teamId"])
			assert.Equal(t, f.teamShotsKey, teamShots["keyString"])
			fullTime := asMap(asSlice(asMap(tabs[0])["periods"])[0])
			assert.Empty(t, fullTime["period"])
			home := asMap(asSlice(asMap(asSlice(fullTime["boards"])[0])["selections"])[0])
			assert.Equal(t, "bet9ja", home["bestSource"])
			assert.NotNil(t, home["consensus"])
		}},
		{name: "event unknown", method: "get", template: "/v1/events/{id}", path: "/v1/events/" + uuid.NewString(), status: 404},
		{name: "event malformed id", method: "get", template: "/v1/events/{id}", path: "/v1/events/not-a-uuid", status: 400},
		{name: "event props", method: "get", template: "/v1/events/{id}/props", path: "/v1/events/" + f.eventID + "/props", status: 200, check: func(t *testing.T, body map[string]any) {
			rows := asSlice(body["rows"])
			require.Len(t, rows, 1)
			row := asMap(rows[0])
			assert.Equal(t, f.propKey, row["keyString"])
			players := asSlice(body["players"])
			require.Len(t, players, 1)
			player := asMap(players[0])
			assert.Contains(t, row["keyString"], player["id"])
			assert.Equal(t, "Player", strings.Fields(player["name"].(string))[0])
			assert.Equal(t, true, player["benchWarning"])
			assert.NotNil(t, player["ratings"])
			assert.NotEmpty(t, player["minutesTrend"])
			research := asMap(row["research"])
			assert.NotContains(t, research, "minutesTrend")
			assert.InDelta(t, 10, asMap(asSlice(research["windows"])[1])["window"], 0)
			for _, selection := range asSlice(row["selections"]) {
				for _, cell := range asMap(asMap(selection)["cells"]) {
					assert.NotContains(t, asMap(cell), "marketKey")
					assert.NotContains(t, asMap(cell), "observedAt")
					assert.Contains(t, asMap(cell), "raw")
				}
			}
			vsMatch := asMap(asSlice(research["vsOpponentMatches"])[0])
			assert.Equal(t, "Away", strings.Fields(vsMatch["opponentTeamName"].(string))[0])
		}},
		{name: "market history", method: "get", template: "/v1/markets/{key}/history", path: "/v1/markets/" + url.PathEscape(f.ouKey) + "/history?source=bet9ja", status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Len(t, body["points"], 1)
		}},
		{name: "market history bad key", method: "get", template: "/v1/markets/{key}/history", path: "/v1/markets/nope/history", status: 400},
		{name: "signals", method: "get", template: "/v1/signals", path: "/v1/signals?type=arb&min=0.02&sport=football", status: 200, check: func(t *testing.T, body map[string]any) {
			signal := findByID(t, body["items"], "", func(item map[string]any) bool { return item["dedupeKey"] == f.dedupeKey })
			assert.Equal(t, "football", asMap(signal["event"])["sport"])
			assert.Equal(t, f.eventID, asMap(signal["event"])["id"])
			assert.NotEmpty(t, asMap(asMap(signal["event"])["home"])["logoUrl"])
			assert.InDelta(t, float64(1500*time.Millisecond), signal["oldestLegAge"], 0)
		}},
		{name: "move signal", method: "get", template: "/v1/signals", path: "/v1/signals?type=move&sport=football", status: 200, check: func(t *testing.T, body map[string]any) {
			move := findByID(t, body["items"], "", func(item map[string]any) bool { return item["dedupeKey"] == f.dedupeKey+"|move" })
			assert.InDelta(t, 0.5, asMap(move["fromPrice"])["prob"], 1e-9)
			assert.InDelta(t, 0.625, asMap(move["toPrice"])["prob"], 1e-9)
			assert.InDelta(t, 0.125, move["change"], 1e-9)
			assert.InDelta(t, 0.25, move["margin"], 1e-9)
			assert.Equal(t, oneXTwoKey(f.eventID), asMap(asSlice(move["legs"])[1])["marketKey"])
		}},
		{name: "signals bad type", method: "get", template: "/v1/signals", path: "/v1/signals?type=bogus", status: 400},
		{name: "player stats", method: "get", template: "/v1/players/{id}/stats", path: "/v1/players/" + f.playerID + "/stats?last=5", status: 200, check: func(t *testing.T, body map[string]any) {
			player := asMap(body["player"])
			assert.Equal(t, "FW", player["position"])
			assert.True(t, strings.HasPrefix(player["teamName"].(string), "Home EPL"))
			assert.Equal(t, "/v1/logos/team/"+f.homeTeamID+contractLogoURL, player["teamLogoUrl"])
			matchStats := asMap(asMap(asSlice(body["items"])[0])["stats"])
			assert.InDelta(t, 3, matchStats["shots"], 0)
			assert.InDelta(t, 7.1, matchStats["rating"], 1e-9)
			assert.Contains(t, matchStats, "shotsOnTarget")
		}},
		{name: "player unknown", method: "get", template: "/v1/players/{id}/stats", path: "/v1/players/" + uuid.NewString() + "/stats", status: 404},
		{name: "team", method: "get", template: "/v1/teams/{id}", path: "/v1/teams/" + f.homeTeamID, status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Equal(t, f.homeTeamID, body["id"])
			assert.Equal(t, "football", body["sport"])
			assert.Equal(t, []any{"EPL"}, body["leagueCodes"])
			assert.Equal(t, "/v1/logos/team/"+f.homeTeamID+contractLogoURL, body["logoUrl"])
		}},
		{name: "team unknown", method: "get", template: "/v1/teams/{id}", path: "/v1/teams/" + uuid.NewString(), status: 404},
		{name: "team malformed id", method: "get", template: "/v1/teams/{id}", path: "/v1/teams/not-a-uuid", status: 400},
		{name: "sources", method: "get", template: "/v1/sources", path: "/v1/sources", status: 200, check: func(t *testing.T, body map[string]any) {
			bet9ja := findByID(t, body["items"], "", func(item map[string]any) bool { return item["name"] == "bet9ja" })
			assert.InDelta(t, 0.1, bet9ja["errorRate"], 1e-9)
			assert.NotNil(t, bet9ja["lastSuccess"])
			assert.Equal(t, "/v1/logos/source/bet9ja"+contractLogoURL, bet9ja["logoUrl"])
			assert.Equal(t, "sportsbook", bet9ja["type"])
			assert.Equal(t, "poll", bet9ja["feed"])
			assert.Equal(t, "bet9ja-feed", bet9ja["feedGroup"])
			assert.Equal(t, "contract note", bet9ja["note"])
			assert.Equal(t, []any{"NG"}, bet9ja["countries"])
			assert.Equal(t, []any{}, bet9ja["blockedCountries"])
			coverage := asMap(bet9ja["coverage"])
			assert.Contains(t, coverage["leagues"], "EPL")
			assert.Contains(t, coverage["markets"], "1X2")
			kalshi := findByID(t, body["items"], "", func(item map[string]any) bool { return item["name"] == "kalshi" })
			assert.Equal(t, "kalshi", kalshi["feedGroup"])
			assert.Nil(t, kalshi["note"])
			assert.Equal(t, []any{"GB"}, kalshi["blockedCountries"])
			assert.Nil(t, findOptional(body["items"], func(item map[string]any) bool { return item["name"] == fakeSourceName }))
		}},
		{name: "logo unknown entity", method: "get", template: "/v1/logos/{kind}/{id}", path: "/v1/logos/team/" + uuid.NewString(), status: 404},
		{name: "logo unknown kind", method: "get", template: "/v1/logos/{kind}/{id}", path: "/v1/logos/player/x", status: 400},
		{name: "review without token", method: "get", template: "/v1/admin/review", path: "/v1/admin/review", anonymous: true, status: 401},
		{name: "review with a user token", method: "get", template: "/v1/admin/review", path: "/v1/admin/review", status: 401},
		{name: "review list", method: "get", template: "/v1/admin/review", path: "/v1/admin/review", token: testAdminToken, status: 200},
		{name: "review decide", method: "post", template: "/v1/admin/review", path: "/v1/admin/review", token: testAdminToken, body: decision(review.ActionLink, uuid.NewString()), status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Equal(t, review.StatusLinked, body["status"])
		}},
		{name: "review conflicting decision", method: "post", template: "/v1/admin/review", path: "/v1/admin/review", token: testAdminToken, body: decision(review.ActionIgnore, ""), status: 409},
		{name: "review invalid decision", method: "post", template: "/v1/admin/review", path: "/v1/admin/review", token: testAdminToken, body: decision(review.ActionLink, ""), status: 400},
		{name: "review malformed body", method: "post", template: "/v1/admin/review", path: "/v1/admin/review", token: testAdminToken, body: "{", status: 400},
		{name: "meta", method: "get", template: "/v1/meta", path: "/v1/meta", anonymous: true, status: 200, check: func(t *testing.T, body map[string]any) {
			assert.NotEmpty(t, body["version"])
			assert.Nil(t, body["telegramBotUrl"])
			assert.Equal(t, map[string]any{"google": false}, body["auth"])
			assert.Empty(t, body["country"])
		}},
		{name: "search league", method: "get", template: "/v1/search", path: "/v1/search?q=nba", status: 200, check: func(t *testing.T, body map[string]any) {
			league := findByID(t, body["items"], "NBA")
			assert.Equal(t, "league", league["kind"])
			assert.Equal(t, "basketball", league["sport"])
			assert.Equal(t, "/v1/logos/league/NBA"+contractLogoURL, league["logoUrl"])
		}},
		{name: "search other sport", method: "get", template: "/v1/search", path: "/v1/search?q=nba&sport=football&limit=5", status: 200, check: func(t *testing.T, body map[string]any) {
			assert.Nil(t, findOptional(body["items"], func(item map[string]any) bool { return item["id"] == "NBA" }))
		}},
		{name: "search too short", method: "get", template: "/v1/search", path: "/v1/search?q=a", status: 400},
		{name: "search bad limit", method: "get", template: "/v1/search", path: "/v1/search?q=nba&limit=500", status: 400},
	}
	runContractCases(t, httpServer.URL, spec, token, cases)
}

func runContractCases(t *testing.T, baseURL string, spec *openAPISpec, token string, cases []contractCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.token == "" && !tc.anonymous {
				tc.token = token
			}
			body := callAPI(t, baseURL, tc)
			schema, err := spec.responseSchema(tc.template, tc.method, tc.status)
			require.NoError(t, err)
			assert.Empty(t, spec.validate(body, schema, "response"))
			if tc.check != nil {
				tc.check(t, asMap(body))
			}
		})
	}
}

func callAPI(t *testing.T, baseURL string, tc contractCase) any {
	t.Helper()
	req, err := http.NewRequest(strings.ToUpper(tc.method), baseURL+tc.path, bytes.NewBufferString(tc.body))
	require.NoError(t, err)
	req.Header.Set("Origin", testWebOrigin)
	req.Header.Set("X-Forwarded-For", testIP())
	if tc.token != "" {
		authorize(req, tc.token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, tc.status, resp.StatusCode, string(raw))
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	var body any
	require.NoError(t, json.Unmarshal(raw, &body))
	return body
}

func findOptional(items any, match func(map[string]any) bool) map[string]any {
	for _, item := range asSlice(items) {
		if m := asMap(item); match(m) {
			return m
		}
	}
	return nil
}

func findByID(t *testing.T, items any, id string, match ...func(map[string]any) bool) map[string]any {
	t.Helper()
	matcher := func(item map[string]any) bool { return item["id"] == id }
	if len(match) > 0 {
		matcher = match[0]
	}
	found := findOptional(items, matcher)
	require.NotNil(t, found, "item not found in %v", items)
	return found
}
