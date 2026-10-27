package loadtest

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"mybestbet/pkg/canon"
)

type replayPair struct {
	source  string
	eventID string
	markets []canon.Market
}

var syntheticNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("mybestbet/loadtest"))

func syntheticID(kind, name string) string {
	return uuid.NewSHA1(syntheticNamespace, []byte(kind+":"+strings.ToLower(name))).String()
}

func syntheticEventIDs(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = syntheticID("event", strconv.Itoa(i))
	}
	return ids
}

func planReplayPairs(eventsBySource map[string][]fixtureEvent, eventIDs []string) []replayPair {
	sourceNames := slices.Sorted(maps.Keys(eventsBySource))
	pairs := make([]replayPair, 0, len(eventIDs)*len(sourceNames))
	for i, eventID := range eventIDs {
		for _, source := range sourceNames {
			fixtures := eventsBySource[source]
			pairs = append(pairs, replayPair{source: source, eventID: eventID, markets: canonicalMarkets(eventID, fixtures[i%len(fixtures)])})
		}
	}
	return pairs
}

func canonicalMarkets(eventID string, fixture fixtureEvent) []canon.Market {
	seen := map[string]bool{}
	markets := make([]canon.Market, 0, len(fixture))
	for _, raw := range fixture {
		key := canonicalKey(eventID, raw)
		if len(raw.Selections) == 0 || seen[key.String()] {
			continue
		}
		seen[key.String()] = true
		markets = append(markets, canon.Market{Key: key, SourceKey: raw.SourceMarketID, Selections: raw.Selections, Rules: raw.Rules})
	}
	return markets
}

func canonicalKey(eventID string, raw canon.RawMarket) canon.MarketKey {
	key := canon.MarketKey{EventID: eventID, Code: raw.Code, Line: raw.Line, Period: raw.Period}
	if raw.Player != nil {
		key.PlayerID = syntheticID("player", raw.Player.Name)
	}
	if raw.Team != "" {
		key.TeamID = syntheticID("team", raw.Team)
	}
	return key
}
