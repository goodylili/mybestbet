package flashscore

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
	switch path := r.URL.Path; {
	case strings.HasPrefix(path, "/x/feed/"):
		return "feed_" + strings.TrimPrefix(path, "/x/feed/") + ".txt", true
	case strings.HasPrefix(path, tennisPathPrefix):
		return "page_" + strings.ReplaceAll(strings.Trim(strings.TrimPrefix(path, tennisPathPrefix), "/"), "/", "_") + ".html", true
	}
	return "", false
}

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	return New(sourcestest.Deps(sourcestest.ReplayServer(t, SourceName, replayRoute), sources.Config{}))
}

func TestDayMatchesDiscoversTourSinglesTournaments(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).DayMatches(t.Context(), -1)
	require.NoError(t, err)

	tournamentByID := map[string]Tournament{}
	for _, m := range matches {
		assert.Equal(t, m.Tournament.Code, m.LeagueCode)
		tournamentByID[m.ID] = m.Tournament
	}
	almaty := Tournament{Code: "ATP_ALMATY", Name: "ATP Almaty", Country: "Kazakhstan"}
	assert.Equal(t, map[string]Tournament{
		"hd9LBSct": almaty,
		"27fjhO64": almaty,
		"44GtNVPa": {Code: "WTA_NINGBO", Name: "WTA Ningbo", Country: "China"},
		"QmxjgN3B": {Code: "ATP_CH_CALI", Name: "ATP Challenger Cali", Country: "Colombia"},
	}, tournamentByID)
}

func TestDayMatchesMapsConfiguredTournament(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).DayMatches(t.Context(), 1)
	require.NoError(t, err)

	require.Len(t, matches, 3)
	assert.Equal(t, Match{
		ID: "xOq6aXM7", LeagueCode: "ATP_CH_ZADAR", Tournament: Tournament{Code: "ATP_CH_ZADAR", Name: "ATP Challenger Zadar", Country: "Croatia"}, Kickoff: time.Unix(1792836000, 0).UTC(), Status: StatusScheduled,
		Home: Player{ID: "Kb8ttRy2", Name: "Nikoloz Basilashvili", ShortName: "Basilashvili N."},
		Away: Player{ID: "hb6oWfc6", Name: "Stefanos Tsitsipas", ShortName: "Tsitsipas S."},
	}, matches[0])
}

func TestTournamentResultsMapsMainDrawAndQualifyingToConfiguredLeague(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).TournamentResults(t.Context(), "ATP_WIM")
	require.NoError(t, err)

	leagueByID := map[string]string{}
	statusByID := map[string]MatchStatus{}
	for _, m := range matches {
		leagueByID[m.ID], statusByID[m.ID] = m.LeagueCode, m.Status
	}
	assert.Equal(t, map[string]string{
		"n5DWLXD5": "ATP_WIM", "nkXJ8mYa": "ATP_WIM", "dGA7OFrf": "ATP_WIM", "Q9aiCq5E": "ATP_WIM", "vXyt3VEd": "ATP_WIM",
	}, leagueByID)
	assert.Equal(t, StatusRetired, statusByID["Q9aiCq5E"])
	assert.Equal(t, StatusFinished, statusByID["n5DWLXD5"])
}

func TestTournamentResultsRejectsUnknownLeague(t *testing.T) {
	t.Parallel()
	_, err := newReplayClient(t).TournamentResults(t.Context(), "EPL")
	assert.ErrorIs(t, err, ErrUnknownLeague)
}

func TestServeStatsReadsWholeMatchCounts(t *testing.T) {
	t.Parallel()
	stats, err := newReplayClient(t).ServeStats(t.Context(), "hd9LBSct")
	require.NoError(t, err)
	assert.Equal(t, MatchServeStats{Home: ServeStats{Aces: 11, DoubleFaults: 2}, Away: ServeStats{Aces: 6, DoubleFaults: 1}}, stats)
}

func TestServeStatsWithoutPlayedPointsIsNoStats(t *testing.T) {
	t.Parallel()
	_, err := newReplayClient(t).ServeStats(t.Context(), "4WWj8VPE")
	assert.ErrorIs(t, err, ErrNoStats)
}

func TestServeStatsRejectsMalformedCounts(t *testing.T) {
	t.Parallel()
	_, err := serveStatsFrom(parseRecords("SE÷Match¬~SG÷Aces¬SH÷x¬SI÷2¬~"))
	assert.ErrorIs(t, err, ErrMalformedStat)
}

func TestEmbeddedFeedMissing(t *testing.T) {
	t.Parallel()
	_, err := embeddedFeed("<html></html>", resultsFeedName)
	assert.ErrorIs(t, err, ErrFeedMissing)
}

func TestFullName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		shortName string
		slug      string
		expected  string
	}{
		{name: "surname first slug", shortName: "Sinner J.", slug: "sinner-jannik", expected: "Jannik Sinner"},
		{name: "given name first slug", shortName: "Fils A.", slug: "arthur-fils", expected: "Arthur Fils"},
		{name: "compound surname and two initials", shortName: "La Serna J. M.", slug: "la-serna-juan-manuel", expected: "Juan Manuel La Serna"},
		{name: "hyphenated initials", shortName: "Struff J-L.", slug: "struff-jan-lennard", expected: "Jan Lennard Struff"},
		{name: "lowercase particle kept", shortName: "van Assche L.", slug: "van-assche-luca", expected: "Luca van Assche"},
		{name: "slug disagrees with surname", shortName: "Samson L.", slug: "samsonova-laura", expected: "Samson L."},
		{name: "initial disagrees with slug", shortName: "Abe H.", slug: "abe-mio", expected: "Abe H."},
		{name: "no initials", shortName: "Bahrain W", slug: "bahrain", expected: "Bahrain W"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, fullName(tt.shortName, tt.slug))
		})
	}
}

func TestTournamentOfSection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		header        string
		path          string
		expected      Tournament
		isTourSingles bool
	}{
		{
			name:          "atp tour event",
			header:        "ATP - SINGLES: Basel (Switzerland), hard (indoor)",
			path:          "/tennis/atp-singles/basel/",
			expected:      Tournament{Code: "ATP_BASEL", Name: "ATP Basel", Country: "Switzerland"},
			isTourSingles: true,
		},
		{
			name:          "configured slam keeps its league code",
			header:        "ATP - SINGLES: Wimbledon (United Kingdom), grass",
			path:          "/tennis/atp-singles/wimbledon/",
			expected:      Tournament{Code: "ATP_WIM", Name: "ATP Wimbledon", Country: "United Kingdom"},
			isTourSingles: true,
		},
		{
			name:          "qualification shares the main draw code",
			header:        "CHALLENGER MEN - SINGLES: Sioux Falls (USA) - Qualification, hard (indoor)",
			path:          "/tennis/challenger-men-singles/sioux-falls/",
			expected:      Tournament{Code: "ATP_CH_SIOUX_FALLS", Name: "ATP Challenger Sioux Falls", Country: "USA"},
			isTourSingles: true,
		},
		{
			name:          "wta challenger",
			header:        "CHALLENGER WOMEN - SINGLES: Courmayeur (Italy), hard (indoor)",
			path:          "/tennis/challenger-women-singles/courmayeur/",
			expected:      Tournament{Code: "WTA_CH_COURMAYEUR", Name: "WTA Challenger Courmayeur", Country: "Italy"},
			isTourSingles: true,
		},
		{
			name:          "header without venue falls back to the slug",
			header:        "WTA - SINGLES",
			path:          "/tennis/wta-singles/hong-kong/",
			expected:      Tournament{Code: "WTA_HONG_KONG", Name: "WTA Hong Kong"},
			isTourSingles: true,
		},
		{
			name:   "doubles dropped",
			header: "ATP - DOUBLES: Basel (Switzerland), hard (indoor)",
			path:   "/tennis/atp-doubles/basel/",
		},
		{
			name:   "itf dropped",
			header: "ITF MEN - SINGLES: M15 Selva Gardena (Italy), hard",
			path:   "/tennis/itf-men-singles/m15-selva-gardena/",
		},
		{
			name:   "team event dropped",
			header: "TEAMS - WOMEN: Billie Jean King Cup - Group III (World) - Promotion",
			path:   "/tennis/teams-women/billie-jean-king-cup-group-iii/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tournamentOfSection(tt.header, tt.path)
			assert.Equal(t, tt.isTourSingles, ok)
			assert.Equal(t, tt.expected, got)
		})
	}
}
