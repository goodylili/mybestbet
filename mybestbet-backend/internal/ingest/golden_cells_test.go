package ingest

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

const adapterGoldenGlob = "../../testdata/*/golden/*.json"

func TestAdapterGoldensHaveOnePricePerCell(t *testing.T) {
	paths, err := filepath.Glob(adapterGoldenGlob)
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	for _, path := range paths {
		t.Run(filepath.Join(filepath.Base(filepath.Dir(filepath.Dir(path))), filepath.Base(path)), func(t *testing.T) {
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			var snap canon.Snapshot
			require.NoError(t, json.Unmarshal(body, &snap))
			require.Empty(t, duplicateCells(snap.Markets))
		})
	}
}

func duplicateCells(markets []canon.RawMarket) []string {
	seenIn := map[string]string{}
	var duplicates []string
	for _, m := range markets {
		for _, s := range m.Selections {
			cell := cellKey(m, s)
			if first, seen := seenIn[cell]; seen {
				duplicates = append(duplicates, cell+" in "+first+" and "+m.SourceMarketID)
				continue
			}
			seenIn[cell] = m.SourceMarketID
		}
	}
	return duplicates
}

func cellKey(m canon.RawMarket, s canon.Selection) string {
	line := ""
	if m.Line != nil {
		line = strconv.FormatFloat(*m.Line, 'f', -1, 64)
	}
	player := ""
	if m.Player != nil {
		player = m.Player.SourceID
	}
	return strings.Join([]string{m.EventSourceID, string(m.Code), line, player, m.Period, m.Team, string(s.Side), s.Label}, "|")
}
