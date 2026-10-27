package engine

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

func spoolTestRows(n int) []priceRow {
	rows := make([]priceRow, n)
	for i := range rows {
		rows[i] = priceRow{
			marketKey: "ev|1X2||||", eventID: "00000000-0000-0000-0000-000000000001", code: canon.Code1X2,
			cell:  canon.BoardCell{Source: "betking", Side: canon.SideHome, Price: canon.NewPrice(2+float64(i)/100, canon.FormDecimal, testNow)},
			rules: canon.StandardRule, isRawChanged: true,
		}
	}
	return rows
}

func TestSpoolRoundTripsRowsAndRemovesDrainedSegment(t *testing.T) {
	t.Parallel()
	s, err := openSpool(t.TempDir())
	require.NoError(t, err)
	written := spoolTestRows(3)
	require.NoError(t, s.append(written[:2]))
	require.NoError(t, s.append(written[2:]))

	path, ok, err := s.claimOldest()
	require.NoError(t, err)
	require.True(t, ok)
	_, again, err := s.claimOldest()
	require.NoError(t, err)
	assert.False(t, again)

	read, err := readSpoolSegment(path)
	require.NoError(t, err)
	assert.Equal(t, written, read)

	require.NoError(t, s.release(path, true))
	assert.NoFileExists(t, path)
	assert.Zero(t, s.pendingSegments())
}

func TestSpoolKeepsSegmentWhenWriteFails(t *testing.T) {
	t.Parallel()
	s, err := openSpool(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, s.append(spoolTestRows(1)))
	path, _, err := s.claimOldest()
	require.NoError(t, err)

	require.NoError(t, s.release(path, false))
	assert.FileExists(t, path)
	again, ok, err := s.claimOldest()
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, path, again)
}

func TestSpoolAdoptsSegmentsLeftByACrashedEngine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	crashed, err := openSpool(dir)
	require.NoError(t, err)
	require.NoError(t, crashed.append(spoolTestRows(2)))
	require.NoError(t, crashed.active.buf.Flush())

	restarted, err := openSpool(dir)
	require.NoError(t, err)
	path, ok, err := restarted.claimOldest()
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, ".seg", filepath.Ext(path))
	rows, err := readSpoolSegment(path)
	require.NoError(t, err)
	assert.Len(t, rows, 2)

	require.NoError(t, restarted.append(spoolTestRows(1)))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestPriceFlusherSpillsOverflowInsteadOfDropping(t *testing.T) {
	t.Parallel()
	s, err := openSpool(t.TempDir())
	require.NoError(t, err)
	f := newPriceFlusher(nil, s, time.Hour, 1, slog.New(slog.DiscardHandler))
	f.enqueue(spoolTestRows(f.queueLimit()))
	f.enqueue(spoolTestRows(5))
	f.spillQueuedOverflow()

	assert.Equal(t, f.queueLimit(), f.pendingRows)
	path, ok, err := s.claimOldest()
	require.NoError(t, err)
	require.True(t, ok)
	spilled, err := readSpoolSegment(path)
	require.NoError(t, err)
	assert.Len(t, spilled, 5)
}
