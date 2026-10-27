package fbref

import (
	"context"
	"fmt"
	"strings"

	"mybestbet/internal/stats/fetch"
	"mybestbet/pkg/sources"
)

const (
	SourceName     = "fbref"
	DefaultBaseURL = "https://fbref.com/en"
)

var DefaultCompetitions = map[string]string{
	"EPL": "9/Premier-League", "LALIGA": "12/La-Liga", "SERIEA": "11/Serie-A", "BUNDES": "20/Bundesliga",
	"LIGUE1": "13/Ligue-1", "ERED": "23/Eredivisie", "PRIMEIRA": "32/Primeira-Liga", "CHAMP": "10/Championship",
	"SPL": "70/Saudi-Pro-League", "MLS": "22/Major-League-Soccer",
}

type Client struct {
	fetcher      fetch.Fetcher
	baseURL      string
	competitions map[string]string
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	competitions := map[string]string{}
	for code, comp := range DefaultCompetitions {
		competitions[code] = comp
	}
	for code, comp := range deps.Config.LeagueIDs {
		competitions[code] = comp
	}
	return &Client{fetcher: fetch.Fetcher{Deps: deps, Profile: SourceName}, baseURL: baseURL, competitions: competitions}
}

func (c *Client) LeagueSeason(ctx context.Context, leagueCode string) ([]PlayerSeason, error) {
	competition, ok := c.competitions[leagueCode]
	if !ok {
		return nil, fmt.Errorf("fbref league %s: %w", leagueCode, ErrUnknownLeague)
	}
	compID, slug, _ := strings.Cut(competition, "/")
	byPlayer := map[string]*PlayerSeason{}
	var order []string
	for _, page := range statPages {
		target := fmt.Sprintf("%s/comps/%s/%s/%s-Stats", c.baseURL, compID, page.path, slug)
		body, err := c.fetcher.Body(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("fbref %s %s: %w", leagueCode, page.path, err)
		}
		rows, season, err := parseTable(body, page.tableID)
		if err != nil {
			return nil, fmt.Errorf("fbref %s %s: %w", leagueCode, page.path, err)
		}
		for _, r := range rows {
			p, seen := byPlayer[r.fbrefID]
			if !seen {
				p = &PlayerSeason{FBrefID: r.fbrefID, Name: r.cells["player"], Season: season}
				byPlayer[r.fbrefID] = p
				order = append(order, r.fbrefID)
			}
			p.Team = r.cells["team"]
			page.apply(p, r.cells)
		}
	}
	seasons := make([]PlayerSeason, 0, len(order))
	for _, id := range order {
		seasons = append(seasons, *byPlayer[id])
	}
	return seasons, nil
}
