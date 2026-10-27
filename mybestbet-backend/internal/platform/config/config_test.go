package config

import (
	"math"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/pkg/canon"
)

var adapterSources = []string{
	"polymarket", "kalshi", "predictfun", "bayse", "sportybet", "footballcom", "onexbet", "22bet", "paripesa",
	"betwinner", "bet9ja", "betking", "betpawa", "betano", "betway", "bet365", "prizepicks",
}

func loadRepoConfig(t *testing.T) Config {
	t.Helper()
	t.Setenv("CONFIG_PATH", "../../../config.yaml")
	cfg, err := Load()
	require.NoError(t, err)
	return cfg
}

func TestRepoConfigLeaguesHaveSidebarPlacement(t *testing.T) {
	cfg := loadRepoConfig(t)
	require.Len(t, cfg.Leagues, 72)
	orders := map[canon.Sport][]int{}
	for _, l := range cfg.Leagues {
		assert.Contains(t, []string{"international", "domestic", "more"}, l.Group, l.Code)
		assert.Positive(t, l.Order, l.Code)
		assert.NotContains(t, orders[l.Sport], l.Order, "%s reuses order %d", l.Code, l.Order)
		orders[l.Sport] = append(orders[l.Sport], l.Order)
	}
	bigFive := []string{"EPL", "LALIGA", "SERIEA", "BUNDES", "LIGUE1"}
	for _, l := range cfg.Leagues {
		if l.Sport == canon.SportFootball && slices.Contains(bigFive, l.Code) {
			assert.LessOrEqual(t, l.Order, len(bigFive), l.Code)
		}
	}
}

func TestRepoConfigSourceProfiles(t *testing.T) {
	cfg := loadRepoConfig(t)
	types := map[string]string{"polymarket": "prediction", "kalshi": "prediction", "predictfun": "prediction", "bayse": "prediction", "prizepicks": "pickem"}
	for _, name := range adapterSources {
		expected := types[name]
		if expected == "" {
			expected = "sportsbook"
		}
		assert.Equal(t, expected, cfg.Profiles[name].Type, name)
	}
	assert.Equal(t, "sporty", cfg.Feed("footballcom"))
	assert.Equal(t, "onexbet", cfg.Feed("22bet"))
	assert.Equal(t, "betway", cfg.Feed("betway"))
}

func TestRepoConfigSourceAvailability(t *testing.T) {
	cfg := loadRepoConfig(t)
	alpha2 := regexp.MustCompile(`^[A-Z]{2}$`)
	for _, name := range adapterSources {
		profile := cfg.Profiles[name]
		assert.NotEqual(t, len(profile.Countries) == 0, len(profile.BlockedCountries) == 0, "%s sets exactly one of countries or blockedCountries", name)
		for _, code := range slices.Concat(profile.Countries, profile.BlockedCountries) {
			assert.Regexp(t, alpha2, code, name)
		}
	}
	assert.Equal(t, []string{"US"}, cfg.Profiles["kalshi"].Countries)
	assert.Contains(t, cfg.Profiles["polymarket"].BlockedCountries, "US")
}

func TestRepoConfigQuoteMaxAge(t *testing.T) {
	limits := loadRepoConfig(t).Pricing.QuoteMaxAge
	tests := []struct {
		name         string
		sourceType   string
		untilKickoff time.Duration
		want         time.Duration
	}{
		{name: "live sportsbook", sourceType: "sportsbook", untilKickoff: -time.Minute, want: 90 * time.Second},
		{name: "live streamed prediction market", sourceType: "prediction", untilKickoff: -time.Minute, want: 6 * time.Minute},
		{name: "sportsbook within two hours", sourceType: "sportsbook", untilKickoff: time.Hour, want: 3 * time.Minute},
		{name: "prediction market within two hours", sourceType: "prediction", untilKickoff: time.Hour, want: 6 * time.Minute},
		{name: "sportsbook later today", sourceType: "sportsbook", untilKickoff: 10 * time.Hour, want: 20 * time.Minute},
		{name: "sportsbook days ahead", sourceType: "sportsbook", untilKickoff: 72 * time.Hour, want: time.Hour},
		{name: "beyond every tier", sourceType: "sportsbook", untilKickoff: time.Duration(math.MaxInt64), want: time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, limits.For(tt.sourceType, tt.untilKickoff))
		})
	}
}

func TestQuoteMaxAgeDisabledWhenUnset(t *testing.T) {
	t.Parallel()
	assert.Zero(t, QuoteMaxAge{}.For("sportsbook", time.Hour))
	assert.Zero(t, QuoteMaxAge{MinByType: map[string]time.Duration{"prediction": time.Minute}}.For("prediction", -time.Minute))
}
