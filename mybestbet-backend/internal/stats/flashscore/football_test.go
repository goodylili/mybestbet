package flashscore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

const shandongSecondHalfStart = 1793003523

func TestStartedFootballMatchesKeepsLiveAndFinished(t *testing.T) {
	t.Parallel()
	now := time.Unix(shandongSecondHalfStart+14*60+6, 0)
	matches, err := newReplayClient(t).StartedFootballMatches(t.Context(), 0, now)
	require.NoError(t, err)

	byID := map[string]FootballMatch{}
	for _, m := range matches {
		byID[m.ID] = m
	}
	require.Len(t, byID, 4)
	assert.Equal(t, FootballMatch{
		ID: "byF4Qllg", Kickoff: time.Unix(1792999800, 0).UTC(), Home: "Shandong Taishan B", Away: "Jiangxi Lushan",
		Score: canon.EventScore{Home: 1, Away: 1, Period: canon.PeriodSecondHalf, Minute: 60, Clock: "60"},
	}, byID["byF4Qllg"])
	assert.Equal(t, FootballMatch{
		ID: "4K3UUOde", Kickoff: time.Unix(1792976400, 0).UTC(), Home: "Los Angeles FC", Away: "Los Angeles Galaxy", Finished: true,
		Score: canon.EventScore{Home: 2, Away: 2, Period: canon.PeriodFinished, Detail: "FT"},
	}, byID["4K3UUOde"])
	assert.Equal(t, canon.EventScore{Home: 1, Away: 2, Period: canon.PeriodFinished, Detail: "FT"}, byID["6ZoODtBL"].Score)
	assert.NotContains(t, byID, "8MQeSuQE")
	assert.NotContains(t, byID, "rJwRzd56")
}

func TestStartedFootballMatchLivePeriods(t *testing.T) {
	t.Parallel()
	periodStart := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		detail  string
		elapsed time.Duration
		want    canon.EventScore
	}{
		{name: "first minute", detail: "12", elapsed: 20 * time.Second, want: canon.EventScore{Period: canon.PeriodFirstHalf, Minute: 1, Clock: "1"}},
		{name: "first half stoppage", detail: "12", elapsed: 46*time.Minute + 30*time.Second, want: canon.EventScore{Period: canon.PeriodFirstHalf, Minute: 45, Clock: "45+2"}},
		{name: "half time", detail: "38", elapsed: 5 * time.Minute, want: canon.EventScore{Period: canon.PeriodHalfTime, Clock: "HT"}},
		{name: "second half stoppage", detail: "13", elapsed: 48 * time.Minute, want: canon.EventScore{Period: canon.PeriodSecondHalf, Minute: 90, Clock: "90+4"}},
		{name: "extra time", detail: "6", elapsed: 10 * time.Minute, want: canon.EventScore{Period: canon.PeriodExtraTime}},
		{name: "penalties", detail: "7", elapsed: time.Minute, want: canon.EventScore{Period: canon.PeriodExtraTime, Detail: "Pen"}},
		{name: "unknown live detail", detail: "99", elapsed: time.Minute, want: canon.EventScore{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := record{
				keyMatchID: "x", keyStartUnix: "1792999800", keyStage: stageLive, keyStatusDetail: tt.detail,
				keyHomeTeam: "Home", keyAwayTeam: "Away", keyPeriodStartUnix: "1793001600",
			}
			match, ok := startedFootballMatchFrom(rec, periodStart.Add(tt.elapsed))
			require.True(t, ok)
			assert.False(t, match.Finished)
			assert.Equal(t, tt.want, match.Score)
		})
	}
}

func TestStartedFootballMatchFinishedDetails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		detail   string
		want     string
		included bool
	}{
		{name: "full time", detail: "3", want: "FT", included: true},
		{name: "after extra time", detail: "10", want: "AET", included: true},
		{name: "after penalties", detail: "11", want: "Pen", included: true},
		{name: "postponed", detail: "4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := record{keyMatchID: "x", keyStartUnix: "1792999800", keyStage: stageFinished, keyStatusDetail: tt.detail, keyHomeTeam: "Home", keyAwayTeam: "Away", keyHomeGoals: "1"}
			match, ok := startedFootballMatchFrom(rec, time.Now())
			require.Equal(t, tt.included, ok)
			if ok {
				assert.True(t, match.Finished)
				assert.Equal(t, canon.EventScore{Home: 1, Period: canon.PeriodFinished, Detail: tt.want}, match.Score)
			}
		})
	}
}
