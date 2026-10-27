package flashscore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestDayMatchesReadLiveAndFinishedScores(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).DayMatches(t.Context(), 0)
	require.NoError(t, err)

	scores := map[string]canon.EventScore{}
	statuses := map[string]MatchStatus{}
	for _, m := range matches {
		scores[m.ID], statuses[m.ID] = m.Score, m.Status
	}
	require.Len(t, scores, 4)
	assert.Equal(t, StatusLive, statuses["fLgxrFAj"])
	assert.Equal(t, canon.EventScore{Period: "S1", Clock: "0-1", Detail: "6-6"}, scores["fLgxrFAj"])
	assert.Equal(t, canon.EventScore{Period: "S1", Clock: "0-0", Detail: "4-5"}, scores["pGVjw5Kh"])
	assert.Equal(t, StatusFinished, statuses["U3kIH9AT"])
	assert.Equal(t, canon.EventScore{Away: 2, Period: canon.PeriodFinished, Detail: "6-7 3-6"}, scores["U3kIH9AT"])
}

func TestScheduledMatchHasNoScore(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).DayMatches(t.Context(), 1)
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Zero(t, matches[0].Score)
}
