package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/fake"
)

type streamingAdapter struct{ sources.Adapter }

func (streamingAdapter) Stream(context.Context, []string, chan<- canon.Snapshot) error { return nil }

func TestFeedOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		factory  sources.Factory
		expected string
	}{
		{name: "streamer streams", factory: func(deps sources.Deps) sources.Adapter { return streamingAdapter{fake.New(deps)} }, expected: feedStream},
		{name: "rest adapter polls", factory: fake.New, expected: feedPoll},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, feedOf(tt.factory, sources.Config{}))
		})
	}
}

func TestDescribeSource(t *testing.T) {
	t.Parallel()
	s := New(Deps{Config: config.Config{
		Feeds:    map[string]string{"shared-book": "shared-feed"},
		Profiles: map[string]config.SourceProfile{"shared-book": {Feed: "shared-feed", Type: "sportsbook", Note: "mirrors a sibling", BlockedCountries: []string{"US"}}},
	}})
	tests := []struct {
		name     string
		coverage SourceCoverage
		expected SourceHealth
	}{
		{
			name:     "profiled source with coverage",
			coverage: SourceCoverage{Leagues: []string{"EPL"}, Markets: []canon.MarketCode{canon.Code1X2}},
			expected: SourceHealth{Name: "shared-book", Type: "sportsbook", Note: "mirrors a sibling", Feed: feedPoll, FeedGroup: "shared-feed",
				Countries: []string{}, BlockedCountries: []string{"US"},
				Coverage: SourceCoverage{Leagues: []string{"EPL"}, Markets: []canon.MarketCode{canon.Code1X2}}},
		},
		{
			name:     "unprofiled source without prices",
			expected: SourceHealth{Name: "lonely-book", Feed: feedPoll, FeedGroup: "lonely-book", Countries: []string{}, BlockedCountries: []string{}, Coverage: SourceCoverage{Leagues: []string{}, Markets: []canon.MarketCode{}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			health := SourceHealth{Name: tt.expected.Name}
			s.describeSource(&health, tt.coverage)
			assert.Equal(t, tt.expected, health)
		})
	}
}

func TestErrorLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw      string
		expected string
	}{
		{raw: "", expected: ""},
		{raw: "betano leagues: GET https://www.betano.ng/api/sport/basketball/: status 403", expected: "Blocked from this region"},
		{raw: "price event: GET https://www.sportybet.com/api: empty body after 3 attempts: source throttled", expected: "Rate limited"},
		{raw: "discover coupon https://www.bet365.com/#/AC/: bet365: page produced no odds payload", expected: "No odds returned"},
		{raw: "list events: read https://relay.bayse.markets/v1/pm/events: context deadline exceeded", expected: "Timed out"},
		{raw: "GET https://api.example.com: dial tcp: lookup api.example.com: no such host", expected: "Connection failed"},
		{raw: "decode markets: invalid character '<' looking for beginning of value", expected: "Upstream error"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, errorLabel(tt.raw), tt.raw)
	}
}
