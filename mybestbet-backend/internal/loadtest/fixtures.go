package loadtest

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	_ "mybestbet/pkg/sources/all"
)

type fixtureEvent []canon.RawMarket

var errNoFixtures = errors.New("loadtest: no golden price snapshots of registered adapters found")

func loadFixtureEvents(testdataDir string) (map[string][]fixtureEvent, error) {
	paths, err := filepath.Glob(filepath.Join(testdataDir, "*", "golden", "*price.json"))
	if err != nil {
		return nil, fmt.Errorf("glob golden snapshots: %w", err)
	}
	eventsBySource := map[string][]fixtureEvent{}
	for _, path := range paths {
		snap, err := readSnapshot(path)
		if err != nil {
			return nil, err
		}
		if _, registered := sources.Lookup(snap.Source); registered && len(snap.Markets) > 0 {
			eventsBySource[snap.Source] = groupByEvent(snap.Markets)
		}
	}
	if len(eventsBySource) == 0 {
		return nil, fmt.Errorf("%w in %s", errNoFixtures, testdataDir)
	}
	return eventsBySource, nil
}

func readSnapshot(path string) (canon.Snapshot, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return canon.Snapshot{}, fmt.Errorf("read golden snapshot: %w", err)
	}
	var snap canon.Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return canon.Snapshot{}, fmt.Errorf("decode golden snapshot %s: %w", path, err)
	}
	return snap, nil
}

func groupByEvent(markets []canon.RawMarket) []fixtureEvent {
	bySourceEvent := map[string]fixtureEvent{}
	for _, m := range markets {
		bySourceEvent[m.EventSourceID] = append(bySourceEvent[m.EventSourceID], m)
	}
	events := make([]fixtureEvent, 0, len(bySourceEvent))
	for _, id := range slices.Sorted(maps.Keys(bySourceEvent)) {
		events = append(events, bySourceEvent[id])
	}
	return events
}
