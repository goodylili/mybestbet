package fotmob

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestDayMatchesReadsScoresAndSkipsCancelled(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).DayMatches(t.Context(), time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	byID := map[string]LiveMatch{}
	for _, m := range matches {
		byID[m.ID] = m
	}
	require.Len(t, byID, 3)
	assert.NotContains(t, byID, "6050083")
	assert.Equal(t, LiveMatch{ID: "5071455", Started: true, Finished: true, Score: canon.EventScore{Home: 2, Away: 2, Period: canon.PeriodFinished, Detail: "FT"}}, byID["5071455"])
	assert.False(t, byID["5868109"].Started)
}

func TestLiveMatchFromDerivesMinuteAndPeriod(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		liveShort string
		want      canon.EventScore
	}{
		{name: "first half", liveShort: "23’", want: canon.EventScore{Home: 1, Period: canon.PeriodFirstHalf, Minute: 23, Clock: "23"}},
		{name: "first half stoppage", liveShort: "45+2’", want: canon.EventScore{Home: 1, Period: canon.PeriodFirstHalf, Minute: 45, Clock: "45+2"}},
		{name: "half time", liveShort: "HT", want: canon.EventScore{Home: 1, Period: canon.PeriodHalfTime, Clock: "HT"}},
		{name: "second half", liveShort: "67'", want: canon.EventScore{Home: 1, Period: canon.PeriodSecondHalf, Minute: 67, Clock: "67"}},
		{name: "extra time", liveShort: "105’", want: canon.EventScore{Home: 1, Period: canon.PeriodExtraTime, Minute: 105, Clock: "105"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var m dayMatch
			m.ID, m.Home.Score, m.Status.Started = "1", 1, true
			m.Status.LiveTime = &liveTimeWire{Short: tt.liveShort}
			assert.Equal(t, LiveMatch{ID: "1", Started: true, Score: tt.want}, liveMatchFrom(m))
		})
	}
}
