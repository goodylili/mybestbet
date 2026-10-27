package stats

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/redis"
	"mybestbet/internal/seed"
	"mybestbet/internal/stats/acb"
	"mybestbet/internal/stats/bo3"
	"mybestbet/internal/stats/espn"
	"mybestbet/internal/stats/esports"
	"mybestbet/internal/stats/euroleague"
	"mybestbet/internal/stats/flashscore"
	"mybestbet/internal/stats/fotmob"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var liveRoundNow = time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)

type liveFixtureEvents struct {
	football, basketball, tennis, esports SourcedEvent
}

func TestLiveScoreRoundStoresPublishesAndSkipsUnchanged(t *testing.T) {
	f := openSportFixture(t)
	rdb := openLiveRedis(t)
	events := seedLiveFixtureEvents(t, f)
	eventChannels := []string{redis.EventChannel(events.football.ID), redis.EventChannel(events.basketball.ID), redis.EventChannel(events.tennis.ID), redis.EventChannel(events.esports.ID)}
	pubsub := rdb.Subscribe(t.Context(), append(eventChannels, redis.ChannelBoard)...)
	t.Cleanup(func() { _ = pubsub.Close() })
	_, err := pubsub.Receive(t.Context())
	require.NoError(t, err)

	collector := newLiveFixtureCollector(t, f, rdb, liveRoundNow)
	first := collector.CollectRound(t.Context())

	want := map[string]struct {
		status string
		score  canon.EventScore
	}{
		events.football.ID:   {"finished", canon.EventScore{Home: 2, Away: 2, Period: canon.PeriodFinished, Detail: "FT"}},
		events.basketball.ID: {"finished", canon.EventScore{Home: 123, Away: 132, Period: canon.PeriodFinished, Detail: "Final"}},
		events.tennis.ID:     {"live", canon.EventScore{Period: "S1", Clock: "0-1", Detail: "6-6"}},
		events.esports.ID:    {"live", canon.EventScore{Period: "M1"}},
	}
	for eventID, expected := range want {
		status, score := storedLiveScore(t, f, eventID)
		assert.Equal(t, expected.status, status, eventID)
		assert.Equal(t, expected.score, score, eventID)
	}
	for _, sport := range []canon.Sport{canon.SportFootball, canon.SportBasketball, canon.SportTennis, canon.SportEsports} {
		require.NotNil(t, first[sport], sport)
		assert.GreaterOrEqual(t, first[sport].Updated, 1, sport)
	}

	published := receiveScoreMessages(t, pubsub, 2*len(want))
	for eventID, expected := range want {
		assert.Equal(t, expected.score, published[redis.EventChannel(eventID)], eventID)
		assert.Equal(t, expected.score, published[redis.ChannelBoard+"|"+eventID], eventID)
	}

	second := newLiveFixtureCollector(t, f, rdb, liveRoundNow).CollectRound(t.Context())
	for _, sport := range []canon.Sport{canon.SportTennis, canon.SportEsports} {
		require.NotNil(t, second[sport], sport)
		assert.GreaterOrEqual(t, second[sport].Scored, 1, sport)
		assert.Zero(t, second[sport].Updated, sport)
	}
	assertNoMessage(t, pubsub)
}

func TestLiveScoreRoundCoversDotaEuroLeagueACBAndUnlinkedFootball(t *testing.T) {
	f := openSportFixture(t)
	rdb := openLiveRedis(t)
	suffix := uuid.NewString()[:8]
	var since time.Time
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since))
	t.Cleanup(func() {
		teamSources := "live-" + suffix + "-%"
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM teams WHERE "createdAt" >= $2 AND id IN (SELECT "teamId" FROM team_sources WHERE "sourceId" LIKE $1)`, teamSources, since)
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM team_sources WHERE "sourceId" LIKE $1`, teamSources)
	})
	tests := []struct {
		name     string
		league   seed.League
		source   string
		sourceID string
		kickoff  time.Time
		home     string
		away     string
		now      time.Time
		status   string
		want     canon.EventScore
	}{
		{
			name: "dota series from opendota", league: seed.League{Code: "DOTA2", Name: "Dota 2", Country: "World", Sport: canon.SportEsports},
			source: esports.LiquipediaSource, sourceID: "DOTA2:live-" + suffix, kickoff: time.Date(2026, 10, 25, 17, 0, 0, 0, time.UTC),
			home: "PARIVISION", away: "BetBoom Team", now: time.Date(2026, 10, 25, 19, 40, 0, 0, time.UTC),
			status: "finished", want: canon.EventScore{Home: 2, Period: canon.PeriodFinished},
		},
		{
			name: "dota live map from opendota", league: seed.League{Code: "DOTA2", Name: "Dota 2", Country: "World", Sport: canon.SportEsports},
			source: esports.LiquipediaSource, sourceID: "DOTA2:live-map-" + suffix, kickoff: time.Date(2026, 10, 26, 6, 0, 0, 0, time.UTC),
			home: "BetBoom Team", away: "PARIVISION", now: time.Date(2026, 10, 26, 6, 40, 0, 0, time.UTC),
			status: "live", want: canon.EventScore{Period: "M1", Detail: "13-15"},
		},
		{
			name: "euroleague game", league: seed.League{Code: "EUROLEAGUE", Name: "EuroLeague", Country: "Europe", Sport: canon.SportBasketball},
			source: euroleague.SourceName, sourceID: "E2026:69", kickoff: time.Date(2026, 10, 23, 18, 45, 0, 0, time.UTC),
			home: "Paris Basketball", away: "Valencia Basket", now: time.Date(2026, 10, 23, 20, 45, 0, 0, time.UTC),
			status: "finished", want: canon.EventScore{Home: 93, Away: 104, Period: canon.PeriodFinished},
		},
		{
			name: "liga acb match", league: seed.League{Code: "ACB", Name: "Liga ACB", Country: "Spain", Sport: canon.SportBasketball},
			source: acb.SourceName, sourceID: "105419", kickoff: time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC),
			home: "iLERNA Lleida", away: "Real Madrid", now: time.Date(2026, 10, 25, 20, 30, 0, 0, time.UTC),
			status: "finished", want: canon.EventScore{Home: 86, Away: 122, Period: canon.PeriodFinished},
		},
		{
			name: "football without fotmob link", league: seed.League{Code: "MLS", Name: "Major League Soccer", Country: "USA", Sport: canon.SportFootball},
			source: "sofascore", sourceID: "live-" + suffix, kickoff: time.Date(2026, 10, 26, 1, 0, 0, 0, time.UTC),
			home: "Los Angeles FC", away: "Los Angeles Galaxy", now: liveRoundNow,
			status: "finished", want: canon.EventScore{Home: 2, Away: 2, Period: canon.PeriodFinished, Detail: "FT"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			team := func(name string) seed.SourceTeam {
				return seed.SourceTeam{Source: tt.source, SourceID: "live-" + suffix + "-" + name, LeagueCode: tt.league.Code, Name: name}
			}
			e := f.event(t, tt.league, tt.source, tt.sourceID, tt.kickoff, team(tt.home), team(tt.away))
			t.Cleanup(func() {
				_, _ = f.pool.Exec(context.Background(), `DELETE FROM events WHERE id = $1`, e.ID)
				_, _ = f.pool.Exec(context.Background(), `DELETE FROM event_sources WHERE source = $1 AND "sourceId" = $2`, tt.source, tt.sourceID)
			})
			pubsub := rdb.Subscribe(t.Context(), redis.EventChannel(e.ID), redis.ChannelBoard)
			t.Cleanup(func() { _ = pubsub.Close() })
			_, err := pubsub.Receive(t.Context())
			require.NoError(t, err)

			first := newLiveFixtureCollector(t, f, rdb, tt.now).CollectRound(t.Context())
			status, score := storedLiveScore(t, f, e.ID)
			assert.Equal(t, tt.status, status)
			assert.Equal(t, tt.want, score)
			sport := tt.league.Sport
			require.NotNil(t, first[sport])
			assert.GreaterOrEqual(t, first[sport].Updated, 1)
			eventUpdate, boardUpdate := receiveEventScore(t, pubsub, e.ID)
			assert.Equal(t, tt.want, eventUpdate)
			assert.Equal(t, tt.want, boardUpdate)

			second := newLiveFixtureCollector(t, f, rdb, tt.now).CollectRound(t.Context())
			assert.Zero(t, second.countOf(sport).Updated)
			if tt.status == "live" {
				assert.GreaterOrEqual(t, second.countOf(sport).Scored, 1)
			}
			status, score = storedLiveScore(t, f, e.ID)
			assert.Equal(t, tt.status, status)
			assert.Equal(t, tt.want, score)
			assertNoMessage(t, pubsub)
		})
	}
}

func receiveEventScore(t *testing.T, pubsub *goredis.PubSub, eventID string) (canon.EventScore, canon.EventScore) {
	t.Helper()
	var eventUpdate, boardUpdate *canon.EventScore
	for eventUpdate == nil || boardUpdate == nil {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		msg, err := pubsub.ReceiveMessage(ctx)
		cancel()
		require.NoError(t, err)
		var update struct {
			EventID string           `json:"eventId"`
			Score   canon.EventScore `json:"score"`
		}
		require.NoError(t, json.Unmarshal([]byte(msg.Payload), &update))
		switch {
		case update.EventID != eventID:
		case msg.Channel == redis.ChannelBoard:
			boardUpdate = &update.Score
		default:
			eventUpdate = &update.Score
		}
	}
	return *eventUpdate, *boardUpdate
}

func openLiveRedis(t *testing.T) *goredis.Client {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	rdb, err := redis.Open(t.Context(), url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func seedLiveFixtureEvents(t *testing.T, f sportFixture) liveFixtureEvents {
	t.Helper()
	suffix := uuid.NewString()[:8]
	team := func(source, league, name string) seed.SourceTeam {
		return seed.SourceTeam{Source: source, SourceID: "live-" + suffix + "-" + name, LeagueCode: league, Name: name}
	}
	mls := seed.League{Code: "MLS", Name: "Major League Soccer", Country: "USA", Sport: canon.SportFootball}
	nba := seed.League{Code: "NBA", Name: "NBA", Country: "USA", Sport: canon.SportBasketball}
	zadar := seed.League{Code: "ATP_CH_ZADAR", Name: "Challenger Zadar", Country: "Croatia", Sport: canon.SportTennis}
	cs2 := seed.League{Code: "CS2", Name: "Counter-Strike 2", Country: "World", Sport: canon.SportEsports}
	events := liveFixtureEvents{
		football: f.event(t, mls, fotmob.SourceName, "5071455", liveRoundNow.Add(-2*time.Hour),
			team(fotmob.SourceName, "MLS", "Live Home FC "+suffix), team(fotmob.SourceName, "MLS", "Live Away FC "+suffix)),
		basketball: f.event(t, nba, espn.SourceName, "nba:401909861", liveRoundNow.Add(-3*time.Hour),
			team(espn.SourceName, "NBA", "Live Hoops "+suffix), team(espn.SourceName, "NBA", "Live Dunks "+suffix)),
		tennis: f.event(t, zadar, flashscore.SourceName, "fLgxrFAj", liveRoundNow.Add(-time.Hour),
			team(flashscore.SourceName, zadar.Code, "Milic Live "+suffix), team(flashscore.SourceName, zadar.Code, "Uchida Live "+suffix)),
		esports: f.event(t, cs2, bo3.SourceName, "1:the-mongolz-vs-mouz-cs2-26-10-2026", liveRoundNow.Add(-30*time.Minute),
			team(bo3.SourceName, "CS2", "The MongolZ"), team(bo3.SourceName, "CS2", "MOUZ")),
	}
	t.Cleanup(func() {
		ids := []string{events.football.ID, events.basketball.ID, events.tennis.ID, events.esports.ID}
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM events WHERE id = ANY($1::uuid[])`, ids)
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM teams WHERE id IN (SELECT "teamId" FROM team_sources WHERE "sourceId" LIKE $1)`, "live-"+suffix+"-%")
	})
	return events
}

func newLiveFixtureCollector(t *testing.T, f sportFixture, rdb *goredis.Client, now time.Time) *LiveScoreCollector {
	t.Helper()
	fotmobServer := sourcestest.ReplayServer(t, fotmob.SourceName, func(r *http.Request) (string, bool) {
		return "matches_" + r.URL.Query().Get("date") + ".json", r.URL.Path == "/matches" && r.URL.Query().Get("date") == "20261026"
	})
	fotmobDeps := sourcestest.Deps(fotmobServer, sources.Config{})
	espnDeps := sportReplayDeps(t, espn.SourceName, func(r *http.Request) (string, bool) {
		return "nba_scoreboard_20261025.json", r.URL.Path == "/basketball/nba/scoreboard" && r.URL.Query().Get("dates") == "20261025"
	})
	flashscoreDeps := sportReplayDeps(t, flashscore.SourceName, func(r *http.Request) (string, bool) {
		return "feed_" + strings.TrimPrefix(r.URL.Path, "/x/feed/") + ".txt", strings.HasPrefix(r.URL.Path, "/x/feed/f_")
	})
	bo3Deps := sportReplayDeps(t, bo3.SourceName, func(r *http.Request) (string, bool) {
		return "cs2_matches_20261026.json", r.URL.Path == "/matches"
	})
	openDotaDeps := sportReplayDeps(t, "esports", func(r *http.Request) (string, bool) {
		return map[string]string{"/proMatches": "opendota_pro_matches_20261025.json", "/live": "opendota_live.json"}[r.URL.Path], r.URL.Path == "/proMatches" || r.URL.Path == "/live"
	})
	euroLeagueServer := sourcestest.ReplayServer(t, euroleague.SourceName, func(r *http.Request) (string, bool) {
		if r.URL.Path == "/api/Header" {
			return "e_header_" + r.URL.Query().Get("gamecode") + ".json", r.URL.Query().Get("seasoncode") == "E2026"
		}
		return "e_game_69.json", r.URL.Path == "/v2/competitions/E/seasons/E2026/games/69"
	})
	euroLeagueDeps := sourcestest.Deps(euroLeagueServer, sources.Config{Extra: map[string]string{"liveBaseUrl": euroLeagueServer.URL}})
	acbServer := sourcestest.ReplayServer(t, acb.SourceName, func(r *http.Request) (string, bool) {
		switch r.URL.Path {
		case "/api/matchdata/Menu/competition-data":
			return "competition_data.json", true
		case "/api/matchdata/Menu/matchlist":
			return "matchlist_week_2991_finalized.json", r.URL.Query().Get("weekId") == "2991"
		}
		return acbStatsRoute(r)
	})
	acbDeps := sourcestest.Deps(acbServer, sources.Config{Extra: map[string]string{"keyPageUrl": acbServer.URL + "/es/liga"}})
	return NewLiveScoreCollector(LiveScoreCollectorDeps{
		FotMob: fotmob.New(fotmobDeps), ESPN: espn.New(espnDeps), Flashscore: flashscore.New(flashscoreDeps), Bo3: bo3.New(bo3Deps),
		OpenDota: esports.NewOpenDota(openDotaDeps), EuroLeague: euroleague.New(euroLeagueDeps), ACB: acb.New(acbDeps),
		Store: f.store, Redis: rdb, Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return now },
		SportOf: func(code string) canon.Sport {
			return map[string]canon.Sport{
				"MLS": canon.SportFootball, "NBA": canon.SportBasketball, "ATP_CH_ZADAR": canon.SportTennis, "CS2": canon.SportEsports,
				"DOTA2": canon.SportEsports, "EUROLEAGUE": canon.SportBasketball, "ACB": canon.SportBasketball,
			}[code]
		},
	})
}

func storedLiveScore(t *testing.T, f sportFixture, eventID string) (string, canon.EventScore) {
	t.Helper()
	var status string
	var score canon.EventScore
	require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT status, score FROM events WHERE id = $1`, eventID).Scan(&status, &score))
	return status, score
}

func receiveScoreMessages(t *testing.T, pubsub *goredis.PubSub, count int) map[string]canon.EventScore {
	t.Helper()
	published := map[string]canon.EventScore{}
	for range count {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		msg, err := pubsub.ReceiveMessage(ctx)
		cancel()
		require.NoError(t, err)
		var update struct {
			EventID string           `json:"eventId"`
			Score   canon.EventScore `json:"score"`
		}
		require.NoError(t, json.Unmarshal([]byte(msg.Payload), &update))
		key := msg.Channel
		if msg.Channel == redis.ChannelBoard {
			key += "|" + update.EventID
		}
		published[key] = update.Score
	}
	return published
}

func assertNoMessage(t *testing.T, pubsub *goredis.PubSub) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	_, err := pubsub.ReceiveMessage(ctx)
	assert.Error(t, err)
}

func TestFinishOverdueEventsGuardsOnStatusAndLiveFeed(t *testing.T) {
	f := openSportFixture(t)
	events := seedLiveFixtureEvents(t, f)
	now := liveRoundNow.Add(4 * time.Hour)
	_, err := f.pool.Exec(t.Context(), `UPDATE events SET status = 'live', "liveUpdatedAt" = $2 WHERE id = $1`, events.basketball.ID, now.Add(-5*time.Minute))
	require.NoError(t, err)
	ids := []string{events.football.ID, events.basketball.ID, events.tennis.ID, events.esports.ID}
	finish := func() {
		_, err := f.store.FinishOverdueEvents(t.Context(), now, overdueAfterKickoff, overdueFallback, overdueQuietFor)
		require.NoError(t, err)
	}

	finish()
	finish()

	want := map[string]string{events.football.ID: "finished", events.basketball.ID: "live", events.tennis.ID: "scheduled", events.esports.ID: "scheduled"}
	for _, id := range ids {
		var status string
		require.NoError(t, f.pool.QueryRow(t.Context(), `SELECT status FROM events WHERE id = $1`, id).Scan(&status))
		assert.Equal(t, want[id], status, id)
	}
}
