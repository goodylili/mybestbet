package seed

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources/sourcestest"
)

const flashscoreRecordedSeedDay = "f_2_1_3_en_1"

func flashscoreSeedRoute(r *http.Request) (string, bool) {
	feed, isFeed := strings.CutPrefix(r.URL.Path, "/x/feed/")
	switch {
	case isFeed && feed == flashscoreRecordedSeedDay:
		return "feed_" + feed + ".txt", true
	case isFeed && strings.HasPrefix(feed, "f_2_"):
		return "feed_empty_day.txt", true
	}
	return "", false
}

func TestFlashscoreTennisSeedCreatesChallengerDraw(t *testing.T) {
	pool := openTestPool(t)
	var since time.Time
	if err := pool.QueryRow(t.Context(), `SELECT now()`).Scan(&since); err != nil {
		t.Fatalf("db now: %v", err)
	}
	removeOtherSportRowsCreatedSince(t, pool, since)
	store := NewStore(pool)
	seedTwice(t, store, newReplaySeeder(t, store, sourcestest.FixedNow),
		League{Code: "ATP_CH_ZADAR", Name: "ATP Challenger Zadar", Country: "Croatia", Sport: canon.SportTennis},
	)

	assertCount(t, pool, `SELECT count(*) FROM event_sources s JOIN events e ON e.id = s."eventId"
		WHERE s.source = 'flashscore' AND e."leagueCode" = 'ATP_CH_ZADAR' AND e.status = 'scheduled'
		  AND s."sourceId" IN ('xOq6aXM7', 'AJS23nbt', '8pPpECVs')`, 3)
	assertCount(t, pool, `SELECT count(*) FROM team_sources s JOIN players p ON p."teamId" = s."teamId" JOIN teams t ON t.id = s."teamId"
		JOIN player_aliases a ON a.source = 'flashscore' AND a."sourceId" = s."sourceId" AND a."playerId" = p.id
		WHERE s.source = 'flashscore' AND p.name = t.name AND s."createdAt" >= $1`, 6, since)
	assertCount(t, pool, `SELECT count(*) FROM team_aliases a JOIN team_sources s ON s."teamId" = a."teamId"
		WHERE s.source = 'flashscore' AND s."sourceId" = 'Kb8ttRy2' AND a."sourceName" IN ('Nikoloz Basilashvili', 'Basilashvili N.')`, 2)
}
