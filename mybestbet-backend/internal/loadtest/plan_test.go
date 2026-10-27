package loadtest

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func repoTestdata(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

func TestPlanReplayPairs_FromGoldenFixtures(t *testing.T) {
	t.Parallel()
	eventsBySource, err := loadFixtureEvents(repoTestdata(t))
	require.NoError(t, err)
	require.Contains(t, eventsBySource, "bet9ja")
	eventIDs := syntheticEventIDs(3)
	pairs := planReplayPairs(eventsBySource, eventIDs)
	require.Len(t, pairs, len(eventIDs)*len(eventsBySource))
	for _, pair := range pairs {
		require.NotEmpty(t, pair.markets, pair.source)
		keys := map[string]bool{}
		for _, m := range pair.markets {
			assert.Equal(t, pair.eventID, m.Key.EventID)
			assert.NotEmpty(t, m.Selections)
			assert.False(t, keys[m.Key.String()], "duplicate market key %s", m.Key)
			keys[m.Key.String()] = true
		}
	}
}

func TestLoadFixtureEvents_MissingDirectory(t *testing.T) {
	t.Parallel()
	_, err := loadFixtureEvents(t.TempDir())
	require.ErrorIs(t, err, errNoFixtures)
}

func TestCanonicalMarkets(t *testing.T) {
	t.Parallel()
	eventID := syntheticEventIDs(1)[0]
	selection := canon.Selection{Side: canon.SideOver, Price: canon.NewPrice(1.9, canon.FormDecimal, time.Time{})}
	player := &canon.SourcePlayer{SourceID: "77", Name: "Bukayo Saka"}
	fixture := fixtureEvent{
		{Code: canon.CodeOU, Line: canon.Line(2.5), Selections: []canon.Selection{selection}, SourceMarketID: "ou"},
		{Code: canon.CodeOU, Line: canon.Line(2.5), Selections: []canon.Selection{selection}, SourceMarketID: "ou-duplicate"},
		{Code: canon.CodeBTTS, SourceMarketID: "empty"},
		{Code: canon.PropShots, Line: canon.Line(1.5), Player: player, Selections: []canon.Selection{selection}, SourceMarketID: "shots"},
	}
	markets := canonicalMarkets(eventID, fixture)
	require.Len(t, markets, 2)
	assert.Equal(t, "ou", markets[0].SourceKey)
	assert.Equal(t, syntheticID("player", "bukayo saka"), markets[1].Key.PlayerID)
	assert.Equal(t, eventID, markets[1].Key.EventID)
}

func TestSyntheticEventIDs_StableAcrossRuns(t *testing.T) {
	t.Parallel()
	assert.Equal(t, syntheticEventIDs(2), syntheticEventIDs(2))
	assert.NotEqual(t, syntheticEventIDs(2)[0], syntheticEventIDs(2)[1])
}
