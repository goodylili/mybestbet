package resolve

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestCanonicalMarketFlipsSidesLabelsAndHandicap(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	price := canon.NewPrice(2, canon.FormDecimal, now)
	tests := []struct {
		name           string
		raw            canon.RawMarket
		expectedLine   *float64
		expectedSides  []canon.Side
		expectedLabels []string
	}{
		{
			name:          "1x2 swaps home and away",
			raw:           canon.RawMarket{Code: canon.Code1X2, Selections: []canon.Selection{{Side: canon.SideHome, Price: price}, {Side: canon.SideDraw, Price: price}, {Side: canon.SideAway, Price: price}}},
			expectedSides: []canon.Side{canon.SideAway, canon.SideDraw, canon.SideHome},
		},
		{
			name:          "asian handicap negates the home line",
			raw:           canon.RawMarket{Code: canon.CodeAH, Line: canon.Line(-0.75), Selections: []canon.Selection{{Side: canon.SideHome, Price: price}, {Side: canon.SideAway, Price: price}}},
			expectedLine:  canon.Line(0.75),
			expectedSides: []canon.Side{canon.SideAway, canon.SideHome},
		},
		{
			name:          "map handicap negates the home line",
			raw:           canon.RawMarket{Code: canon.CodeMapHcp, Line: canon.Line(-1.5), Selections: []canon.Selection{{Side: canon.SideHome, Price: price}, {Side: canon.SideAway, Price: price}}},
			expectedLine:  canon.Line(1.5),
			expectedSides: []canon.Side{canon.SideAway, canon.SideHome},
		},
		{
			name:          "point spread negates the home line",
			raw:           canon.RawMarket{Code: canon.CodeSpread, Line: canon.Line(3.5), Selections: []canon.Selection{{Side: canon.SideAway, Price: price}, {Side: canon.SideHome, Price: price}}},
			expectedLine:  canon.Line(-3.5),
			expectedSides: []canon.Side{canon.SideHome, canon.SideAway},
		},
		{
			name:          "set handicap negates the home line",
			raw:           canon.RawMarket{Code: canon.CodeSetHcp, Line: canon.Line(1.5), Selections: []canon.Selection{{Side: canon.SideHome, Price: price}, {Side: canon.SideAway, Price: price}}},
			expectedLine:  canon.Line(-1.5),
			expectedSides: []canon.Side{canon.SideAway, canon.SideHome},
		},
		{
			name:          "totals keep line and sides",
			raw:           canon.RawMarket{Code: canon.CodeOU, Line: canon.Line(2.5), Selections: []canon.Selection{{Side: canon.SideOver, Price: price}, {Side: canon.SideUnder, Price: price}}},
			expectedLine:  canon.Line(2.5),
			expectedSides: []canon.Side{canon.SideOver, canon.SideUnder},
		},
		{
			name:           "double chance labels mirror",
			raw:            canon.RawMarket{Code: canon.CodeDC, Selections: []canon.Selection{{Label: "1X", Price: price}, {Label: "12", Price: price}, {Label: "X2", Price: price}}},
			expectedSides:  []canon.Side{"", "", ""},
			expectedLabels: []string{"X2", "12", "1X"},
		},
		{
			name:           "correct score reverses",
			raw:            canon.RawMarket{Code: canon.CodeCS, Selections: []canon.Selection{{Label: "2-1", Price: price}, {Label: "Other", Price: price}}},
			expectedSides:  []canon.Side{"", ""},
			expectedLabels: []string{"1-2", "Other"},
		},
		{
			name:           "half time full time swaps team digits",
			raw:            canon.RawMarket{Code: canon.CodeHTFT, Selections: []canon.Selection{{Label: "1/X", Price: price}, {Label: "2/1", Price: price}}},
			expectedSides:  []canon.Side{"", ""},
			expectedLabels: []string{"2/X", "1/2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFirst := tt.raw.Selections[0]
			m := canonicalMarket(tt.raw, EventLink{EventID: "e1", Flipped: true}, "", "")
			assert.Equal(t, tt.expectedLine, m.Key.Line)
			require.Len(t, m.Selections, len(tt.expectedSides))
			for i, s := range m.Selections {
				assert.Equal(t, tt.expectedSides[i], s.Side)
				if tt.expectedLabels != nil {
					assert.Equal(t, tt.expectedLabels[i], s.Label)
				}
			}
			assert.Equal(t, originalFirst, tt.raw.Selections[0])
		})
	}
}

func TestResolveTeamTotalFollowsTheTeamAcrossAFlippedLink(t *testing.T) {
	c, err := NewCache()
	require.NoError(t, err)
	c.addLeague("EPL", canon.SportFootball)
	c.addTeam(Team{ID: "ars", League: "EPL", Name: "Arsenal"})
	c.addTeam(Team{ID: "eve", League: "EPL", Name: "Everton"})
	c.addEvent(Event{ID: "e1", League: "EPL", HomeTeamID: "ars", AwayTeamID: "eve", Kickoff: time.Date(2026, 10, 24, 14, 0, 0, 0, time.UTC)})
	c.linkEvent("straight", "s1", EventLink{EventID: "e1"})
	c.linkEvent("reversed", "r1", EventLink{EventID: "e1", Flipped: true})
	r := &Resolver{cache: c}
	price := canon.NewPrice(1.9, canon.FormDecimal, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
	tests := []struct {
		name           string
		source         string
		eventSourceID  string
		team           string
		expectedTeamID string
	}{
		{name: "home team total on a straight link", source: "straight", eventSourceID: "s1", team: "Arsenal", expectedTeamID: "ars"},
		{name: "away team total on a straight link", source: "straight", eventSourceID: "s1", team: "Everton", expectedTeamID: "eve"},
		{name: "source home total is the canonical away team when flipped", source: "reversed", eventSourceID: "r1", team: "Everton", expectedTeamID: "eve"},
		{name: "source away total is the canonical home team when flipped", source: "reversed", eventSourceID: "r1", team: "Arsenal", expectedTeamID: "ars"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := canon.RawMarket{
				EventSourceID: tt.eventSourceID,
				Code:          canon.TeamGoals,
				Line:          canon.Line(1.5),
				Team:          tt.team,
				Selections:    []canon.Selection{{Side: canon.SideOver, Price: price}, {Side: canon.SideUnder, Price: price}},
			}
			m, pending, err := r.resolveMarket(t.Context(), tt.source, raw)
			require.NoError(t, err)
			require.Nil(t, pending)
			assert.Equal(t, tt.expectedTeamID, m.Key.TeamID)
			assert.Equal(t, canon.Line(1.5), m.Key.Line)
			assert.Equal(t, []canon.Side{canon.SideOver, canon.SideUnder}, []canon.Side{m.Selections[0].Side, m.Selections[1].Side})
		})
	}
}
