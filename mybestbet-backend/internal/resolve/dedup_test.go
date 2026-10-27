package resolve

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheMergeEventRepointsLinksAndForgetsTheDuplicate(t *testing.T) {
	c, err := NewCache()
	require.NoError(t, err)
	kickoff := time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)
	c.addEvent(Event{ID: "keep", League: "EPL", HomeTeamID: "h", AwayTeamID: "a", Kickoff: kickoff})
	c.addEvent(Event{ID: "dup", League: "EPL", HomeTeamID: "a", AwayTeamID: "h", Kickoff: kickoff})
	c.linkEvent("book", "1", EventLink{EventID: "dup"})
	c.linkEvent("book", "2", EventLink{EventID: "dup", Flipped: true})
	c.linkEvent("book", "3", EventLink{EventID: "keep"})

	c.mergeEvent("dup", EventLink{EventID: "keep", Flipped: true})

	tests := []struct {
		name string
		key  string
		want EventLink
	}{
		{name: "unflipped link flips", key: "1", want: EventLink{EventID: "keep", Flipped: true}},
		{name: "flipped link unflips", key: "2", want: EventLink{EventID: "keep"}},
		{name: "survivor link untouched", key: "3", want: EventLink{EventID: "keep"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := c.eventLink("book", tt.key)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
	_, known := c.event("dup")
	assert.False(t, known)
	between := c.eventsBetween("h", "a")
	require.Len(t, between, 1)
	assert.Equal(t, "keep", between[0].ID)
	for _, team := range []string{"h", "a"} {
		for _, e := range c.eventsOf(team) {
			assert.NotEqual(t, "dup", e.ID)
		}
	}
}

func TestLoadDedupConfig(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want DedupConfig
	}{
		{name: "missing block uses defaults", yaml: "resolve:\n  kickoffTolerance: 15m\n", want: DefaultDedupConfig},
		{name: "explicit values", yaml: "resolve:\n  dedup: {every: 2m, purgeGrace: 30m, purgeBatch: 10}\n", want: DedupConfig{Every: 2 * time.Minute, PurgeGrace: 30 * time.Minute, PurgeBatch: 10}},
		{name: "partial values fall back", yaml: "resolve:\n  dedup: {purgeGrace: 0s}\n", want: DedupConfig{Every: DefaultDedupConfig.Every, PurgeGrace: 0, PurgeBatch: DefaultDedupConfig.PurgeBatch}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.yaml), 0o600))
			got, err := LoadDedupConfig(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
