package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func TestOneSourcePlayerPerCanonicalPlayer(t *testing.T) {
	ev := Event{ID: "e", HomeTeamID: "home", AwayTeamID: "away"}
	tests := []struct {
		name     string
		claimant string
		player   canon.SourcePlayer
		expected matchOutcome
	}{
		{name: "same source player keeps its link", claimant: "id-1", player: canon.SourcePlayer{SourceID: "id-1", Name: "Mohamed Al Dossari"}, expected: matchExactAlias},
		{name: "homonym with another id goes to review", claimant: "id-1", player: canon.SourcePlayer{SourceID: "id-2", Name: "Muhammed Al Dossari"}, expected: matchNeedsReview},
		{name: "spelling variant keyed by name goes to review", claimant: "Al Dossari, Mohammed", player: canon.SourcePlayer{Name: "Al Dossari, Muhammed"}, expected: matchNeedsReview},
		{name: "short form of the claimant name links", claimant: "Mohamed Al Dossari", player: canon.SourcePlayer{Name: "Al Dossari"}, expected: matchAutoLinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t)
			c.addPlayer(Player{ID: "p1", Name: "Mohamed Al Dossari", TeamID: "home"})
			c.addPlayer(Player{ID: "p2", Name: "Salem Al Dawsari", TeamID: "away"})
			c.addPlayerAlias(testSource, tt.claimant, "p1")
			m := c.matchPlayer(testSource, tt.player, ev)
			assert.Equal(t, tt.expected, m.outcome)
			if tt.expected == matchNeedsReview {
				require.Len(t, m.candidates, 1)
				assert.Equal(t, "p1", m.candidates[0].ID)
			}
		})
	}
}

func TestUncontestedPlayerAliases(t *testing.T) {
	tests := []struct {
		name     string
		aliases  []storedPlayerAlias
		expected []string
	}{
		{
			name:     "single claim is kept",
			aliases:  []storedPlayerAlias{{source: "s", sourceID: "a", playerID: "p", confidence: 0.91}},
			expected: []string{"a"},
		},
		{
			name: "exact claim beats fuzzy rival",
			aliases: []storedPlayerAlias{
				{source: "s", sourceID: "fuzzy", playerID: "p", confidence: 0.91},
				{source: "s", sourceID: "exact", playerID: "p", confidence: 1},
			},
			expected: []string{"exact"},
		},
		{
			name: "two exact claims are both dropped",
			aliases: []storedPlayerAlias{
				{source: "s", sourceID: "a", playerID: "p", confidence: 1},
				{source: "s", sourceID: "b", playerID: "p", confidence: 1},
			},
			expected: nil,
		},
		{
			name: "claims from different sources do not compete",
			aliases: []storedPlayerAlias{
				{source: "s1", sourceID: "a", playerID: "p", confidence: 0.92},
				{source: "s2", sourceID: "b", playerID: "p", confidence: 0.93},
			},
			expected: []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var kept []string
			for _, a := range uncontestedPlayerAliases(tt.aliases) {
				kept = append(kept, a.sourceID)
			}
			assert.Equal(t, tt.expected, kept)
		})
	}
}
