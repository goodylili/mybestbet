package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheMergePlayerRedirectsTheDuplicate(t *testing.T) {
	t.Parallel()
	c, err := NewCache()
	require.NoError(t, err)
	c.addPlayer(Player{ID: "national-copy", Name: "Bukayo Saka", TeamID: "england"})
	c.addPlayer(Player{ID: "club-copy", Name: "Bukayo Saka", TeamID: "arsenal"})
	c.addPlayerAlias("fotmob", "961995", "national-copy")
	c.addPlayerAlias("betway", "B. Saka", "national-copy")
	c.addPlayerAlias("betway", "Bukayo Saka", "club-copy")

	c.mergePlayer("national-copy", "club-copy")
	c.addSquadMember("england", "club-copy")

	assert.False(t, c.hasPlayer("national-copy"))
	for _, key := range []aliasKey{{"fotmob", "961995"}, {"betway", "B. Saka"}, {"betway", "Bukayo Saka"}} {
		id, ok := c.playerAlias(key.source, key.sourceKey)
		assert.True(t, ok)
		assert.Equal(t, "club-copy", id)
	}
	claimant, _ := c.playerClaimant("fotmob", "club-copy")
	assert.Equal(t, "961995", claimant)
	claimant, _ = c.playerClaimant("betway", "club-copy")
	assert.Equal(t, "Bukayo Saka", claimant)
	for _, team := range []string{"england", "arsenal"} {
		squad := c.squadsOf(team)
		require.Len(t, squad, 1, team)
		assert.Equal(t, "club-copy", squad[0].ID, team)
	}
}
