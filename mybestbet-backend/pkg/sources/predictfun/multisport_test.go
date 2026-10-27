package predictfun

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var multiSportMatches = []string{
	"nfl-kc-sea-2026-10-26", "nba-orl-nyk-2026-10-25", "val-bbl1-ff3-2026-10-25", "cs2-vit-mglz-2026-10-25",
	"lol-gen-ly-2026-10-25", "dota2-ts8-pari-2026-10-26", "fifwc-esp-arg-2026-07-19",
	"atp-struff-grieksp-2026-10-25", "wta-rybakin-fernand-2026-10-25",
}

var multiSportLeagues = []string{"NFL", "NCAAF", "NBA", "VALORANT", "CS2", "LOL", "DOTA2", "WC", "ATP_CH_ZADAR", "WTA_AO"}

var unconfiguredTournaments = sources.Config{LeagueIDs: map[string]string{
	"ATP_CH_ZADAR": "atp/European Open",
	"WTA_AO":       "wta/Ningbo Open",
}}

func TestAdapter_DiscoverMultiSport(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, unconfiguredTournaments, nil).Discover(context.Background(), multiSportLeagues)
	require.NoError(t, err)
	byID := map[string]canon.SourceEvent{}
	for _, ev := range snap.Events {
		byID[ev.SourceID] = ev
	}
	for _, slug := range multiSportMatches {
		assert.Contains(t, byID, slug)
	}
	assert.NotContains(t, byID, "nhl-cal-col-2026-10-31")
	tests := []struct {
		name    string
		slug    string
		league  string
		home    string
		away    string
		kickoff time.Time
	}{
		{"nfl home flag wins over listing order", "nfl-kc-sea-2026-10-26", "NFL", "Seattle Seahawks", "Kansas City Chiefs", time.Date(2026, 10, 26, 0, 20, 0, 0, time.UTC)},
		{"nba home flag", "nba-orl-nyk-2026-10-25", "NBA", "Knicks", "Magic", time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC)},
		{"esports home flag", "val-bbl1-ff3-2026-10-25", "VALORANT", "BBL Esports", "Fire Flux Esports", time.Date(2026, 10, 25, 11, 30, 0, 0, time.UTC)},
		{"esports without home flag first listed is home", "lol-gen-ly-2026-10-25", "LOL", "Gen.G Esports", "LYON", time.Date(2026, 10, 25, 22, 0, 0, 0, time.UTC)},
		{"atp tournament from title via override", "atp-struff-grieksp-2026-10-25", "ATP_CH_ZADAR", "Jan-Lennard Struff", "Tallon Griekspoor", time.Date(2026, 10, 25, 15, 30, 0, 0, time.UTC)},
		{"wta tournament from title via override", "wta-rybakin-fernand-2026-10-25", "WTA_AO", "Elena Rybakina", "Leylah Fernandez", time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)},
		{"world cup kicks off at startsAt", "fifwc-esp-arg-2026-07-19", "WC", "Spain", "Argentina", time.Date(2026, 7, 19, 19, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ev, found := byID[tt.slug]
			require.True(t, found)
			assert.Equal(t, tt.league, ev.LeagueHint)
			assert.Equal(t, tt.home, ev.Home)
			assert.Equal(t, tt.away, ev.Away)
			assert.Equal(t, tt.kickoff, ev.Kickoff)
		})
	}
	sourcestest.AssertGolden(t, Name, "discover_multisport", snap)
}

func TestAdapter_DiscoverDropsTennisOutsideConfiguredTournaments(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Discover(context.Background(), []string{"ATP_CH_ZADAR", "ATP_USO", "WTA_AO"})
	require.NoError(t, err)
	assert.Empty(t, snap.Events)
}

func TestAdapter_PriceMultiSport(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, sources.Config{}, nil)
	snap, err := a.Price(context.Background(), multiSportMatches)
	require.NoError(t, err)
	football, err := a.Price(context.Background(), fixtureMatches)
	require.NoError(t, err)
	sourcestest.RequireMarkets(t, canon.Snapshot{Markets: append(snap.Markets, football.Markets...)}, supportedMarkets...)
	sourcestest.AssertGolden(t, Name, "price_multisport", snap)
}

func TestAdapter_PriceMultiSportMapping(t *testing.T) {
	t.Parallel()
	snap, err := newTestAdapter(t, sources.Config{}, nil).Price(context.Background(), multiSportMatches)
	require.NoError(t, err)
	bySourceID := map[string]canon.RawMarket{}
	for _, m := range snap.Markets {
		bySourceID[m.SourceMarketID] = m
	}
	tests := []struct {
		name     string
		marketID string
		code     canon.MarketCode
		period   string
		line     *float64
		sides    []canon.Side
	}{
		{"nfl moneyline listed away first", "2280889", canon.CodeMoneyline, canon.PeriodFullTime, nil, []canon.Side{canon.SideAway, canon.SideHome}},
		{"nfl away spread flips to home line", "2314977", canon.CodeSpread, canon.PeriodFullTime, canon.Line(1.5), []canon.Side{canon.SideAway, canon.SideHome}},
		{"nfl home spread keeps line", "2315039", canon.CodeSpread, canon.PeriodFullTime, canon.Line(-1.5), []canon.Side{canon.SideHome, canon.SideAway}},
		{"nba moneyline outcome matched by abbreviation", "3541141", canon.CodeMoneyline, canon.PeriodFullTime, nil, []canon.Side{canon.SideAway, canon.SideHome}},
		{"nba spread outcome matched by slug code", "3774547", canon.CodeSpread, canon.PeriodFullTime, canon.Line(-1.5), []canon.Side{canon.SideHome, canon.SideAway}},
		{"nba total", "3774610", canon.CodeTotal, canon.PeriodFullTime, canon.Line(226.5), []canon.Side{canon.SideOver, canon.SideUnder}},
		{"valorant map one winner", "3769407", canon.CodeMoneyline, canon.PeriodFirstMap, nil, []canon.Side{canon.SideHome, canon.SideAway}},
		{"valorant total maps", "3769421", canon.CodeTotalMaps, canon.PeriodFullTime, canon.Line(3.5), []canon.Side{canon.SideOver, canon.SideUnder}},
		{"valorant away map handicap flips to home line", "3769423", canon.CodeMapHcp, canon.PeriodFullTime, canon.Line(2.5), []canon.Side{canon.SideAway, canon.SideHome}},
		{"dota2 map handicap outcome matched by its team", "3787811", canon.CodeMapHcp, canon.PeriodFullTime, canon.Line(1.5), []canon.Side{canon.SideAway, canon.SideHome}},
		{"lol moneyline first listed is home", "3790019", canon.CodeMoneyline, canon.PeriodFullTime, nil, []canon.Side{canon.SideHome, canon.SideAway}},
		{"tennis moneyline", "3785842", canon.CodeMoneyline, canon.PeriodFullTime, nil, []canon.Side{canon.SideHome, canon.SideAway}},
		{"world cup team to advance", "789374", canon.CodeQualify, canon.PeriodFullTime, nil, []canon.Side{canon.SideHome, canon.SideAway}},
		{"nfl anytime touchdown", "3627338", canon.PropAnytimeTD, canon.PeriodFullTime, nil, []canon.Side{canon.SideYes, canon.SideNo}},
		{"world cup team total", "789368", canon.TeamGoals, canon.PeriodFullTime, canon.Line(0.5), []canon.Side{canon.SideOver, canon.SideUnder}},
		{"world cup goalkeeper saves ladder", "790478", canon.PropSaves, canon.PeriodFullTime, canon.Line(1.5), []canon.Side{canon.SideOver, canon.SideUnder}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, found := bySourceID[tt.marketID]
			require.True(t, found)
			assert.Equal(t, tt.code, m.Code)
			assert.Equal(t, tt.period, m.Period)
			assert.Equal(t, tt.line, m.Line)
			assert.Equal(t, tt.code.DefaultRule(), m.Rules)
			sides := make([]canon.Side, 0, len(m.Selections))
			for _, sel := range m.Selections {
				sides = append(sides, sel.Side)
			}
			assert.Equal(t, tt.sides, sides)
		})
	}
	for _, dropped := range []string{"3769410", "3777050", "3777051", "3791459", "789375"} {
		assert.NotContains(t, bySourceID, dropped)
	}
	assert.Equal(t, "Spain", bySourceID["789368"].Team)
	require.NotNil(t, bySourceID["790478"].Player)
	assert.Equal(t, "David Raya", bySourceID["790478"].Player.Name)
}

func TestTeamTotalName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		title    string
		expected string
		ok       bool
	}{
		{name: "team before line", title: "Spain O/U 0.5", expected: "Spain", ok: true},
		{name: "multi word team", title: "Brighton & Hove Albion FC O/U 1.5", expected: "Brighton & Hove Albion FC", ok: true},
		{name: "match total has no team", title: "O/U 2.5", ok: false},
		{name: "not a total", title: "Team to Advance", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := teamTotalName(tt.title)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}
