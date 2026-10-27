package server

import (
	"testing"

	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestEventSummaryEncodesWithAndWithoutScore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		score     *canon.EventScore
		wantScore any
	}{
		{name: "without score", score: nil, wantScore: nil},
		{name: "with score", score: &canon.EventScore{Home: 2, Away: 1, Period: canon.PeriodSecondHalf, Minute: 67}, wantScore: map[string]any{"home": 2.0, "away": 1.0, "period": "2H", "minute": 67.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, value := range []any{
				EventSummary{EventInfo: EventInfo{ID: "e1", Score: tt.score}},
				[]EventSummary{{EventInfo: EventInfo{ID: "e1", Score: tt.score}}},
				SignalView{Event: EventInfo{ID: "e1", Score: tt.score}},
			} {
				encoded, err := json.Marshal(value)
				require.NoError(t, err)
				var decoded any
				require.NoError(t, json.Unmarshal(encoded, &decoded))
				event := eventOf(decoded)
				assert.Equal(t, "e1", event["id"])
				assert.Equal(t, tt.wantScore, event["score"])
			}
		})
	}
}

func eventOf(decoded any) map[string]any {
	if list, ok := decoded.([]any); ok {
		decoded = list[0]
	}
	object := decoded.(map[string]any)
	if nested, ok := object["event"].(map[string]any); ok {
		return nested
	}
	return object
}

func TestFillLegMarketsBackfillsLegacyLegs(t *testing.T) {
	t.Parallel()
	key := canon.MarketKey{EventID: "e1", Code: canon.CodeOU, Line: canon.Line(2.5)}
	own := canon.MarketKey{EventID: "e1", Code: canon.CodeOU, Line: canon.Line(3.5)}.String()
	s := canon.Signal{Type: canon.SignalArb, Market: key, Legs: []canon.Leg{{Source: "a"}, {Source: "b", MarketKey: own, Line: canon.Line(3.5)}}}

	fillLegMarkets(&s)

	assert.Equal(t, key.String(), s.Legs[0].MarketKey)
	require.NotNil(t, s.Legs[0].Line)
	assert.InDelta(t, 2.5, *s.Legs[0].Line, 1e-9)
	assert.Equal(t, own, s.Legs[1].MarketKey)

	gap := canon.Signal{Type: canon.SignalLine, Market: key, Legs: []canon.Leg{{Source: "a"}}}
	fillLegMarkets(&gap)
	assert.Empty(t, gap.Legs[0].MarketKey)
}
