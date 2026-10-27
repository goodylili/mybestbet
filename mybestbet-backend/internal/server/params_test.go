package server

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestMarketSelectorQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		query     string
		expected  canon.MarketKey
		wantError bool
	}{
		{name: "main market by default", query: "", expected: canon.MarketKey{EventID: "e1", Code: canon.CodeMoneyline}},
		{name: "main market of a period", query: "period=M1", expected: canon.MarketKey{EventID: "e1", Code: canon.CodeMoneyline, Period: canon.PeriodFirstMap}},
		{name: "totals with line", query: "market=TOTAL&line=220.50", expected: canon.MarketKey{EventID: "e1", Code: canon.CodeTotal, Line: canon.Line(220.5)}},
		{name: "spread of a quarter", query: "market=SPREAD&line=-2.5&period=Q1", expected: canon.MarketKey{EventID: "e1", Code: canon.CodeSpread, Line: canon.Line(-2.5), Period: canon.PeriodQuarter1}},
		{name: "yes no market", query: "market=BTTS", expected: canon.MarketKey{EventID: "e1", Code: canon.CodeBTTS}},
		{name: "line market without line", query: "market=OU", wantError: true},
		{name: "line on a market without lines", query: "market=ML&line=1", wantError: true},
		{name: "line without market", query: "line=2.5", wantError: true},
		{name: "unknown market", query: "market=NOPE", wantError: true},
		{name: "labeled market", query: "market=CS", wantError: true},
		{name: "player prop", query: "market=BK_PTS&line=20.5", wantError: true},
		{name: "unknown period", query: "period=Q9", wantError: true},
		{name: "malformed line", query: "market=OU&line=abc", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			selector, err := marketSelectorQuery(httptest.NewRequest("GET", "/v1/events?"+tt.query, nil))
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, selector.keyFor("e1", canon.SportBasketball))
		})
	}
}

func TestLocationQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		tz        string
		expected  string
		wantError bool
	}{
		{name: "utc by default", tz: "", expected: "UTC"},
		{name: "iana zone", tz: "Africa/Lagos", expected: "Africa/Lagos"},
		{name: "process local zone", tz: "Local", wantError: true},
		{name: "unknown zone", tz: "Mars/Olympus", wantError: true},
		{name: "path traversal", tz: "../../etc/passwd", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc, err := locationQuery(httptest.NewRequest("GET", "/v1/events/calendar?tz="+tt.tz, nil), "tz")
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, loc.String())
		})
	}
}
