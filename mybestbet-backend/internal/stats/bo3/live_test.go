package bo3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchReadsLiveSeries(t *testing.T) {
	t.Parallel()
	match, err := newReplayClient(t).Match(t.Context(), "the-mongolz-vs-mouz-cs2-26-10-2026")
	require.NoError(t, err)
	assert.True(t, match.IsLive())
	assert.False(t, match.IsDecided())
	assert.Equal(t, [2]int{0, 0}, match.Maps)
}

func TestMatchReadsFinishedSeriesMaps(t *testing.T) {
	t.Parallel()
	match, err := newReplayClient(t).Match(t.Context(), "big-vs-the-mongolz-22-10-2026")
	require.NoError(t, err)
	assert.True(t, match.IsDecided())
	assert.Equal(t, [2]int{1, 2}, match.Maps)
}
