package acb

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const fixtureAPIKey = "11111111-2222-4333-8444-555555555555"

func replayRoute(r *http.Request) (string, bool) {
	switch r.URL.Path {
	case "/es/liga":
		return "liga.html", true
	case "/_next/static/chunks/0cz1d0mv5g_q7.js":
		return "chunk_other.js", true
	case "/_next/static/chunks/2z438bai2hw49.js":
		return "chunk_key.js", true
	}
	if r.Header.Get(apiKeyHeader) != fixtureAPIKey {
		return "", false
	}
	query := r.URL.Query()
	switch r.URL.Path {
	case "/api/matchdata/Menu/competition-data":
		return "competition_data.json", query.Get("competitionIds") == "1"
	case "/api/matchdata/Menu/matchlist":
		return "matchlist_week_" + query.Get("weekId") + ".json", true
	case "/api/matchdata/Result/boxscores":
		if query.Get("matchId") == "105410" {
			return "boxscores_105410.json", true
		}
		return "empty_body.json", true
	}
	return "", false
}

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	srv := sourcestest.ReplayServer(t, SourceName, replayRoute)
	return New(sourcestest.Deps(srv, sources.Config{Extra: map[string]string{keyPageURLExtra: srv.URL + "/es/liga"}}))
}

func TestCurrentWeeksDiscoversAPIKeyFromSiteBundle(t *testing.T) {
	t.Parallel()
	weeks, err := newReplayClient(t).CurrentWeeks(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, weeks)
	assert.Equal(t, Week{ID: 2987, Start: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)}, weeks[0])
}

func TestWeekMatches(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).WeekMatches(t.Context(), 2990)
	require.NoError(t, err)
	require.Len(t, matches, 10)
	last := matches[9]
	assert.Equal(t, int64(105410), last.ID)
	assert.Equal(t, StatusFinalized, last.Status)
	assert.Equal(t, time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC), last.Start)
	assert.Equal(t, Team{ClubID: "8", Name: "Asisa Joventut", ShortName: "Asisa Joventut", Abbreviation: "JOV", LogoURL: "https://static.acb.com/img/www/clubes2026/202526AsisaJoventutLogoPosisitvo.png"}, last.Home)
}

func TestBoxscoreReadsGameTotals(t *testing.T) {
	t.Parallel()
	box, err := newReplayClient(t).Boxscore(t.Context(), 105410)
	require.NoError(t, err)
	assert.True(t, box.Finished)
	var rubio, niebla PlayerLine
	for _, l := range box.Lines {
		switch l.Player.Name {
		case "Ricky Rubio":
			rubio = l
		case "Diego Niebla":
			niebla = l
		}
	}
	assert.Equal(t, "20201871", rubio.Player.ID)
	assert.Equal(t, "8", rubio.Club.ClubID)
	assert.Equal(t, 22, rubio.Minutes)
	assert.True(t, rubio.Played)
	assert.Equal(t, 19.0, rubio.Values[statkey.Points])
	assert.False(t, niebla.Played)
}

func TestBoxscoreOfUnplayedMatchIsEmpty(t *testing.T) {
	t.Parallel()
	box, err := newReplayClient(t).Boxscore(t.Context(), 105411)
	require.NoError(t, err)
	assert.False(t, box.Finished)
	assert.Empty(t, box.Lines)
}

func TestMissingAPIKeyIsReported(t *testing.T) {
	t.Parallel()
	srv := sourcestest.ReplayServer(t, SourceName, func(r *http.Request) (string, bool) {
		return "chunk_other.js", r.URL.Path == "/es/liga"
	})
	client := New(sourcestest.Deps(srv, sources.Config{Extra: map[string]string{keyPageURLExtra: srv.URL + "/es/liga"}}))
	_, err := client.CurrentWeeks(t.Context())
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
}

func TestWeekMatchesReadsScores(t *testing.T) {
	t.Parallel()
	srv := sourcestest.ReplayServer(t, SourceName, func(r *http.Request) (string, bool) {
		if r.URL.Path == "/api/matchdata/Menu/matchlist" && r.URL.Query().Get("weekId") == "2991" {
			return "matchlist_week_2991_finalized.json", r.Header.Get(apiKeyHeader) == fixtureAPIKey
		}
		return replayRoute(r)
	})
	client := New(sourcestest.Deps(srv, sources.Config{Extra: map[string]string{keyPageURLExtra: srv.URL + "/es/liga"}}))
	matches, err := client.WeekMatches(t.Context(), 2991)
	require.NoError(t, err)
	require.Len(t, matches, 3)
	last := matches[2]
	assert.Equal(t, int64(105419), last.ID)
	assert.Equal(t, StatusFinalized, last.Status)
	assert.Equal(t, [2]int{86, 122}, [2]int{last.HomeScore, last.AwayScore})
	assert.Equal(t, 4, last.Quarter)
	assert.False(t, last.HalfTime)
}

func TestWeekOf(t *testing.T) {
	t.Parallel()
	weeks := []Week{{ID: 2991, Start: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}, {ID: 2992, Start: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}}
	tests := []struct {
		name   string
		at     time.Time
		want   int
		listed bool
	}{
		{name: "sunday evening", at: time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC), want: 2991, listed: true},
		{name: "first day of a week", at: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), want: 2992, listed: true},
		{name: "before the season", at: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			week, listed := WeekOf(weeks, tt.at)
			assert.Equal(t, tt.listed, listed)
			assert.Equal(t, tt.want, week.ID)
		})
	}
}
