package flashscore

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"mybestbet/pkg/sources"
)

const (
	SourceName        = "flashscore"
	DefaultBaseURL    = "https://www.flashscore.com"
	OldestDayOffset   = -7
	NewestDayOffset   = 7
	resultsFeedName   = "results"
	tennisDayFeedPath = "/x/feed/f_2_%d_3_en_1"
	serveStatsPath    = "/x/feed/df_st_1_%s"
	resultsPagePath   = tennisPathPrefix + "%s/results/"
)

var (
	ErrNoStats       = errors.New("flashscore: match has no serve statistics")
	ErrFeedMissing   = errors.New("flashscore: page has no embedded feed")
	ErrMalformedStat = errors.New("flashscore: stat value is not a count")
	ErrUnknownLeague = errors.New("flashscore: no tournament for league code")
	errEmptyMatchID  = errors.New("flashscore: empty match id")
)

type Client struct {
	deps    sources.Deps
	baseURL string
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{deps: deps, baseURL: baseURL}
}

func (c *Client) DayMatches(ctx context.Context, dayOffset int) ([]Match, error) {
	body, err := c.deps.Get(ctx, c.baseURL+fmt.Sprintf(tennisDayFeedPath, dayOffset))
	if err != nil {
		return nil, fmt.Errorf("flashscore tennis day %d: %w", dayOffset, err)
	}
	return matchesFrom(parseRecords(string(body))), nil
}

func (c *Client) TournamentResults(ctx context.Context, leagueCode string) ([]Match, error) {
	tournamentPath, ok := Tournaments[leagueCode]
	if !ok {
		return nil, fmt.Errorf("%s: %w", leagueCode, ErrUnknownLeague)
	}
	page, err := c.deps.Get(ctx, c.baseURL+fmt.Sprintf(resultsPagePath, tournamentPath))
	if err != nil {
		return nil, fmt.Errorf("flashscore %s results: %w", leagueCode, err)
	}
	feed, err := embeddedFeed(string(page), resultsFeedName)
	if err != nil {
		return nil, fmt.Errorf("flashscore %s results: %w", leagueCode, err)
	}
	return matchesFrom(parseRecords(feed)), nil
}

func (c *Client) ServeStats(ctx context.Context, matchID string) (MatchServeStats, error) {
	if matchID == "" {
		return MatchServeStats{}, errEmptyMatchID
	}
	body, err := c.deps.Get(ctx, c.baseURL+fmt.Sprintf(serveStatsPath, url.PathEscape(matchID)))
	if err != nil {
		return MatchServeStats{}, fmt.Errorf("flashscore match %s stats: %w", matchID, err)
	}
	stats, err := serveStatsFrom(parseRecords(string(body)))
	if err != nil {
		return MatchServeStats{}, fmt.Errorf("flashscore match %s stats: %w", matchID, err)
	}
	return stats, nil
}
