package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

const (
	testPreMatchMaxAge = 20 * time.Minute
	testLiveMaxAge     = 90 * time.Second
	testStreamedMaxAge = 6 * time.Minute
)

func freshnessBoard(untilKickoff time.Duration) *board {
	cfg := testConfig()
	cfg.Pricing.QuoteMaxAge = config.QuoteMaxAge{
		Live:      testLiveMaxAge,
		PreMatch:  []config.AgeTier{{Within: 2160 * time.Hour, MaxAge: testPreMatchMaxAge}},
		MinByType: map[string]time.Duration{"prediction": testStreamedMaxAge},
	}
	cfg.Profiles = map[string]config.SourceProfile{"bayse": {Type: "prediction"}}
	b := newBoard(cfg, func(string) bool { return false })
	b.untilKickoff = func(string, time.Time) time.Duration { return untilKickoff }
	return b
}

func applyConsensusBooks(b *board, at, now time.Time) {
	for _, source := range []string{"bet9ja", "betking", "sportybet"} {
		b.apply(matchResultDelta(source, 2.1, 3.4, 3.6, at), now)
	}
}

func valueSignalsFrom(cs changeSet, status canon.SignalStatus, source string) []canon.Signal {
	var out []canon.Signal
	for _, s := range signalsOf(cs, canon.SignalEV, status) {
		if s.Legs[0].Source == source {
			out = append(out, s)
		}
	}
	return out
}

func TestBoardClosesValueOnceItsQuoteOutlivesThePreMatchLimit(t *testing.T) {
	t.Parallel()
	b := freshnessBoard(48 * time.Hour)
	applyConsensusBooks(b, testNow, testNow)
	cs := b.apply(matchResultDelta("betway", 2.6, 3.0, 3.0, testNow), testNow)
	opened := valueSignalsFrom(cs, canon.SignalOpen, "betway")
	require.NotEmpty(t, opened)

	stillFresh := b.recheckOpenSignals(testNow.Add(testPreMatchMaxAge - time.Minute))
	assert.Empty(t, valueSignalsFrom(stillFresh, canon.SignalClosed, "betway"))

	expired := b.recheckOpenSignals(testNow.Add(testPreMatchMaxAge + time.Minute))
	closed := valueSignalsFrom(expired, canon.SignalClosed, "betway")
	require.Len(t, closed, len(opened))
	assert.Equal(t, opened[0].ID, closed[0].ID)
}

func TestBoardConfirmedQuoteStaysEligible(t *testing.T) {
	t.Parallel()
	b := freshnessBoard(48 * time.Hour)
	applyConsensusBooks(b, testNow, testNow)
	b.apply(matchResultDelta("betway", 2.6, 3.0, 3.0, testNow), testNow)
	later := testNow.Add(testPreMatchMaxAge + time.Minute)
	for _, source := range []string{"bet9ja", "betking", "sportybet", "betway"} {
		b.stageFreshness(canon.Freshness{Source: source, EventID: matchResult.EventID, ObservedAt: later, SourceKeys: []string{"1x2"}})
	}
	cs := b.recheckOpenSignals(later)
	assert.Empty(t, valueSignalsFrom(cs, canon.SignalClosed, "betway"))
}

func TestBoardLiveLimitDependsOnSourceType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		source  string
		age     time.Duration
		expects bool
	}{
		{name: "fresh live sportsbook quote", source: "betway", age: time.Minute, expects: true},
		{name: "stale live sportsbook quote", source: "betway", age: 2 * time.Minute},
		{name: "streamed quote within its live limit", source: "bayse", age: 5 * time.Minute, expects: true},
		{name: "streamed quote past its live limit", source: "bayse", age: 7 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := freshnessBoard(-time.Minute)
			applyConsensusBooks(b, testNow, testNow)
			cs := b.apply(matchResultDelta(tt.source, 2.6, 3.0, 3.0, testNow.Add(-tt.age)), testNow)
			assert.Equal(t, tt.expects, len(valueSignalsFrom(cs, canon.SignalOpen, tt.source)) > 0)
		})
	}
}

func TestBoardConsensusIgnoresStaleQuotes(t *testing.T) {
	t.Parallel()
	b := freshnessBoard(-time.Minute)
	applyConsensusBooks(b, testNow.Add(-2*time.Minute), testNow)
	assert.Empty(t, b.markets[matchResult.String()].consensus)
}

func TestEventKickoffsDistance(t *testing.T) {
	t.Parallel()
	k := newEventKickoffs(nil, nil)
	k.byEvent.Store(&map[string]time.Time{"ev1": testNow.Add(time.Hour)})
	assert.Equal(t, time.Hour, k.untilKickoff("ev1", testNow))
	assert.Equal(t, -time.Minute, k.untilKickoff("ev1", testNow.Add(61*time.Minute)))
	assert.Equal(t, unknownKickoff, k.untilKickoff("missing", testNow))
}
