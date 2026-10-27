package kalshi

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

var tennisFixtureCompetition = map[string]string{"ATP_MIAMI": "ATP Almaty"}

func TestAdapter_DiscoverMultiSport(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, sources.Config{LeagueIDs: tennisFixtureCompetition})
	snap, err := a.Discover(context.Background(), []string{"NBA", "NFL", "ATP_MIAMI", "WTA_MIAMI", "CS2", "ACB", "COPADELREY"})
	require.NoError(t, err)
	byID := map[string]canon.SourceEvent{}
	for _, ev := range snap.Events {
		byID[ev.SourceID] = ev
	}
	nba := byID["KXNBAGAME-26OCT23HOUSAS"]
	assert.Equal(t, "San Antonio", nba.Home)
	assert.Equal(t, "Houston", nba.Away)
	assert.Equal(t, "NBA", nba.LeagueHint)
	tennis := byID["KXATPMATCH-26OCT24MEDBUB"]
	assert.Equal(t, "ATP_MIAMI", tennis.LeagueHint)
	assert.NotContains(t, byID, "KXATPMATCH-26OCT24BAEMPE")
	acb := byID["KXACBGAME-26OCT241200SPBMAN"]
	assert.Equal(t, time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC), acb.Kickoff)
	sourcestest.AssertGolden(t, Name, "discover_multisport", snap)
}

func TestAdapter_PriceMultiSport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		games    []string
		expected []canon.MarketCode
	}{
		{
			name:  "basketball",
			games: []string{"KXNBAGAME-26OCT23HOUSAS", "KXNBAGAME-26OCT24MIACHA"},
			expected: []canon.MarketCode{canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal,
				canon.PropPoints, canon.PropRebounds, canon.PropBkAssists, canon.PropThrees, canon.PropPtsRebAst},
		},
		{
			name:  "american_football",
			games: []string{"KXNFLGAME-26OCT25PITNO"},
			expected: []canon.MarketCode{canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.PropPassYards,
				canon.PropRushYards, canon.PropRecYards, canon.PropReceptions, canon.PropPassTDs, canon.PropAnytimeTD},
		},
		{
			name:     "tennis",
			games:    []string{"KXATPMATCH-26OCT24MEDBUB"},
			expected: []canon.MarketCode{canon.CodeMoneyline, canon.CodeTotalGames},
		},
		{
			name:     "esports",
			games:    []string{"KXCS2GAME-26OCT240300MGLZAUR"},
			expected: []canon.MarketCode{canon.CodeMoneyline, canon.CodeMapHcp, canon.CodeTotalMaps},
		},
		{
			name:     "cup",
			games:    []string{"KXCOPADELREYGAME-26OCT27CALEUR"},
			expected: []canon.MarketCode{canon.Code1X2, canon.CodeQualify},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snap, err := newTestAdapter(t, sources.Config{}).Price(context.Background(), tt.games)
			require.NoError(t, err)
			sourcestest.RequireMarkets(t, snap, tt.expected...)
			sourcestest.AssertGolden(t, Name, "price_"+tt.name, snap)
		})
	}
}

func TestAdapter_PriceMapsPeriodsAndPlayers(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, sources.Config{})
	snap, err := a.Price(context.Background(), []string{"KXNBAGAME-26OCT23HOUSAS", "KXNBAGAME-26OCT24MIACHA", "KXATPMATCH-26OCT24MEDBUB", "KXCS2GAME-26OCT240300MGLZAUR", "KXCOPADELREYGAME-26OCT27CALEUR"})
	require.NoError(t, err)
	periods := map[string]bool{}
	for _, m := range snap.Markets {
		periods[m.Period] = true
		if m.Code.IsPlayerProp() {
			require.NotNil(t, m.Player)
			assert.NotEmpty(t, m.Player.Name)
			assert.NotEmpty(t, m.Player.Team)
			assert.NotEmpty(t, m.Player.SourceID)
		}
		if m.Code == canon.CodeQualify {
			assert.False(t, m.Rules.RegulationOnly)
		}
	}
	for _, p := range []string{canon.PeriodFirstHalf, canon.PeriodQuarter1, canon.PeriodFirstSet, canon.PeriodFirstMap} {
		assert.True(t, periods[p], "period %s", p)
	}
}

func TestBuildEventMarkets_WinnerWithTieIsDropped(t *testing.T) {
	t.Parallel()
	game := event{EventTicker: "KXEPLGAME-26OCT24ARSEVE", Markets: []market{
		{Ticker: "KXEPLGAME-26OCT24ARSEVE-ARS", Status: activeStatus},
		{Ticker: "KXEPLGAME-26OCT24ARSEVE-TIE", Status: activeStatus},
		{Ticker: "KXEPLGAME-26OCT24ARSEVE-EVE", Status: activeStatus},
	}}
	var dropped []string
	spec := seriesSpec{gameSeriesSuffix, "", canon.CodeMoneyline, canon.PeriodFullTime, shapeWinnerGroup}
	got := buildEventMarkets(game.EventTicker, game, spec, lineup{home: competitor{code: "ARS"}, away: competitor{code: "EVE"}}, func(id string) { dropped = append(dropped, id) })
	assert.Empty(t, got)
	assert.Equal(t, []string{game.EventTicker}, dropped)
}

func TestKickoffFromTicker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		ticker   string
		expected time.Time
		ok       bool
	}{
		{name: "eastern time in key", ticker: "KXACBGAME-26OCT241200SPBMAN", expected: time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC), ok: true},
		{name: "team code starting with digit", ticker: "KXCS2GAME-26OCT2404009INEABRE", expected: time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC), ok: true},
		{name: "date only", ticker: "KXNBAGAME-26OCT23HOUSAS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := kickoffFromTicker(tt.ticker)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.expected, got)
		})
	}
}
