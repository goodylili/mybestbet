package server

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

func TestSportSummariesGroupLeaguesInCanonicalOrder(t *testing.T) {
	t.Parallel()
	summaries := sportSummaries([]League{
		{Code: "NBA", Sport: canon.SportBasketball, FixtureCount: 4},
		{Code: "EPL", Sport: canon.SportFootball, FixtureCount: 10},
		{Code: "WNBA", Sport: canon.SportBasketball, FixtureCount: 1},
	})
	require.Len(t, summaries, 2)
	football, basketball := summaries[0], summaries[1]
	assert.Equal(t, canon.SportFootball, football.Sport)
	assert.Equal(t, 10, football.FixtureCount)
	assert.Contains(t, football.MatchMarkets, canon.Code1X2)
	assert.Equal(t, canon.SportBasketball, basketball.Sport)
	assert.Equal(t, 5, basketball.FixtureCount)
	assert.Len(t, basketball.Leagues, 2)
	assert.Equal(t, []canon.MarketCode{canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal}, basketball.MatchMarkets)
	assert.Contains(t, basketball.PlayerProps, canon.PropPoints)
	assert.Len(t, basketball.Markets, len(basketball.MatchMarkets)+len(basketball.PlayerProps))
	assert.Equal(t, canon.CodeMoneyline, basketball.Markets[0].Code)
}

func TestLeagueCodesOfSport(t *testing.T) {
	t.Parallel()
	s := New(Deps{Config: config.Config{Leagues: []config.League{
		{Code: "EPL", Sport: canon.SportFootball},
		{Code: "NBA", Sport: canon.SportBasketball},
		{Code: "LEGACY"},
	}}})
	tests := []struct {
		name     string
		sport    canon.Sport
		expected []string
	}{
		{name: "no sport means no filter", sport: "", expected: nil},
		{name: "unset sport counts as football", sport: canon.SportFootball, expected: []string{"EPL", "LEGACY"}},
		{name: "basketball", sport: canon.SportBasketball, expected: []string{"NBA"}},
		{name: "sport without leagues matches nothing", sport: canon.SportTennis, expected: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, s.leagueCodesOf(tt.sport))
		})
	}
}

func TestLeagueCodesOfSportIncludesStoredLeagues(t *testing.T) {
	t.Parallel()
	s := New(Deps{Config: config.Config{Leagues: []config.League{
		{Code: "ATP_WIM", Sport: canon.SportTennis},
		{Code: "EPL", Sport: canon.SportFootball},
	}}})
	s.storedLeagues.Store(&map[string]canon.Sport{"ATP_VIENNA": canon.SportTennis, "ATP_BASEL": canon.SportTennis})
	assert.Equal(t, []string{"ATP_WIM", "ATP_BASEL", "ATP_VIENNA"}, s.leagueCodesOf(canon.SportTennis))
	assert.Equal(t, canon.SportTennis, s.sportOf("ATP_BASEL"))
	assert.Equal(t, canon.SportFootball, s.sportOf("UNKNOWN"))
}

func TestSportQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		target   string
		expected canon.Sport
		isValid  bool
	}{
		{name: "absent", target: "/v1/events", expected: "", isValid: true},
		{name: "known sport", target: "/v1/events?sport=american-football", expected: canon.SportAmericanFootball, isValid: true},
		{name: "unknown sport", target: "/v1/events?sport=curling", isValid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sport, err := sportQuery(httptest.NewRequest("GET", tt.target, nil))
			if !tt.isValid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, sport)
		})
	}
}

func TestMarketsOfSport(t *testing.T) {
	t.Parallel()
	byCode := func(markets []MarketInfo) map[canon.MarketCode]MarketInfo {
		indexed := make(map[canon.MarketCode]MarketInfo, len(markets))
		for _, m := range markets {
			indexed[m.Code] = m
		}
		return indexed
	}
	football := byCode(marketsOf(canon.SportFootball))
	assert.Contains(t, football, canon.CodeQualify)
	assert.NotContains(t, football, canon.TeamShots)
	assert.Equal(t, MarketInfo{Code: canon.PropSOT, Label: "Shots on target", Kind: canon.KindPlayer, Shape: canon.ShapeOverUnder, Periods: []string{""}, StatField: "shotsOnTarget", RegulationOnly: true}, football[canon.PropSOT])
	assert.Equal(t, []string{"1H"}, football[canon.Code1H1X2].Periods)
	assert.Equal(t, []string{"", "1H", "2H"}, football[canon.CodeOU].Periods)

	tests := []struct {
		name     string
		sport    canon.Sport
		code     canon.MarketCode
		periods  []string
		field    string
		isListed bool
	}{
		{name: "basketball spread by halves and quarters", sport: canon.SportBasketball, code: canon.CodeSpread, periods: []string{"", "1H", "2H", "Q1", "Q2", "Q3", "Q4"}, isListed: true},
		{name: "basketball points prop", sport: canon.SportBasketball, code: canon.PropPoints, periods: []string{""}, field: "points", isListed: true},
		{name: "tennis winner and first set", sport: canon.SportTennis, code: canon.CodeMoneyline, periods: []string{"", "S1"}, isListed: true},
		{name: "tennis aces", sport: canon.SportTennis, code: canon.PropAces, periods: []string{""}, field: "aces", isListed: true},
		{name: "esports kills per map", sport: canon.SportEsports, code: canon.PropMapKills, periods: []string{"", "M1", "M1-2"}, field: "kills", isListed: true},
		{name: "no football markets for tennis", sport: canon.SportTennis, code: canon.Code1X2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			market, isListed := byCode(marketsOf(tt.sport))[tt.code]
			require.Equal(t, tt.isListed, isListed)
			if !isListed {
				return
			}
			assert.Equal(t, tt.periods, market.Periods)
			assert.Equal(t, tt.field, market.StatField)
			assert.NotEmpty(t, market.Label)
		})
	}
}
