package espn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestGamesReadFinalScores(t *testing.T) {
	t.Parallel()
	board, err := newReplayClient(t).Games(t.Context(), "NBA", time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, board.Games, 2)
	game := board.Games[0]
	assert.Equal(t, "401909861", game.ID)
	assert.True(t, game.IsFinished())
	assert.Equal(t, canon.EventScore{Home: 123, Away: 132, Period: canon.PeriodFinished, Detail: "Final"}, game.Score)
}

func TestScheduledGameHasNoLivePeriod(t *testing.T) {
	t.Parallel()
	board, err := newReplayClient(t).Games(t.Context(), "NBA", time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	game := board.Games[0]
	assert.False(t, game.IsLive() || game.IsFinished())
	assert.Empty(t, game.Score.Period)
}

func TestScoreOfLiveGameLabelsPeriod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		league string
		period int
		want   string
	}{
		{name: "nba third quarter", league: "NBA", period: 3, want: canon.PeriodQuarter3},
		{name: "nba overtime", league: "NBA", period: 5, want: canon.PeriodOvertime},
		{name: "nfl first quarter", league: "NFL", period: 1, want: canon.PeriodQuarter1},
		{name: "college basketball second half", league: "NCAAB", period: 2, want: canon.PeriodSecondHalf},
		{name: "college basketball overtime", league: "NCAAB", period: 3, want: canon.PeriodOvertime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var status statusWire
			status.Period, status.DisplayClock = tt.period, "5:21"
			status.Type.State, status.Type.ShortDetail = StateLive, "live detail"
			assert.Equal(t, canon.EventScore{Period: tt.want, Clock: "5:21", Detail: "live detail"}, scoreOf(status, Leagues[tt.league]))
		})
	}
}
