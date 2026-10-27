package ingest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"mybestbet/internal/resolve"
	"mybestbet/pkg/canon"
)

type pollOnlyTestError struct{}

func (pollOnlyTestError) Error() string { return "needs an api key" }

func (pollOnlyTestError) PollsInstead() bool { return true }

func TestIsPollOnly(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil", err: nil, expected: false},
		{name: "plain stream failure", err: errors.New("dial stream: refused"), expected: false},
		{name: "poll-only sentinel", err: pollOnlyTestError{}, expected: true},
		{name: "wrapped poll-only sentinel", err: fmt.Errorf("%w: %w", errStreamEnded, pollOnlyTestError{}), expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isPollOnly(tt.err))
		})
	}
}

func TestPolledEventsListEverySourceMarketPerEvent(t *testing.T) {
	market := func(eventID, id string) canon.RawMarket {
		return canon.RawMarket{EventSourceID: eventID, Code: canon.Code1X2, SourceMarketID: id}
	}
	all := []canon.RawMarket{market("a", "1"), market("b", "1"), market("a", "2"), market("a", "")}
	assert.Equal(t, []resolve.PolledEvent{
		{SourceEventID: "a", SourceKeys: []string{"1", "2"}, Complete: true},
		{SourceEventID: "b", SourceKeys: []string{"1"}, Complete: true},
	}, polledEvents(all, true))
	assert.False(t, polledEvents(all, false)[0].Complete)
}
