package fotmob

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mybestbet/internal/names"
	"mybestbet/internal/stats/fetch"
	"mybestbet/pkg/sources"
)

const (
	SourceName       = "fotmob"
	DefaultBaseURL   = "https://www.fotmob.com/api/data"
	imageBaseURL     = "https://images.fotmob.com/image_resources/logo"
	kickoffTolerance = 15 * time.Minute
)

var DefaultLeagueIDs = map[string]string{
	"EPL": "47", "LALIGA": "87", "SERIEA": "55", "BUNDES": "54", "LIGUE1": "53",
	"ERED": "57", "PRIMEIRA": "61", "CHAMP": "48", "SPL": "536", "MLS": "130",
	"UCL": "42", "UEL": "73", "UECL": "10216", "UWCL": "9375", "FACUP": "132", "EFLCUP": "133",
	"COPADELREY": "138", "SUPERCOPA": "139", "COPPAITALIA": "141", "DFBPOKAL": "209", "CDF": "134",
	"SUPERLIG": "71", "SPFL": "64", "JPL": "40", "LIGAMX": "230", "BRA1": "268", "BRA2": "8814",
	"ARG1": "112", "SSL": "69", "GSL": "135", "UPL": "441", "RPL": "63", "ELITESERIEN": "59",
	"DSL": "46", "J1": "223", "J2": "8974", "KLEAGUE": "9080", "CSL": "120", "ISL": "9478",
	"ALEAGUE": "113", "LEAGUESCUP": "10043", "LIBERTADORES": "45", "SUDAMERICANA": "299",
}

type Client struct {
	fetcher   fetch.Fetcher
	baseURL   string
	leagueIDs map[string]string
}

type Match struct {
	ID       string
	Home     string
	HomeID   string
	Away     string
	AwayID   string
	Kickoff  time.Time
	Finished bool
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	leagueIDs := map[string]string{}
	for code, id := range DefaultLeagueIDs {
		leagueIDs[code] = id
	}
	for code, id := range deps.Config.LeagueIDs {
		leagueIDs[code] = id
	}
	return &Client{fetcher: fetch.Fetcher{Deps: deps, Profile: SourceName}, baseURL: baseURL, leagueIDs: leagueIDs}
}

func (c *Client) LeagueMatches(ctx context.Context, leagueCode string) ([]Match, error) {
	leagueID, ok := c.leagueIDs[leagueCode]
	if !ok {
		return nil, fmt.Errorf("fotmob league %s: %w", leagueCode, ErrUnknownLeague)
	}
	var resp leagueResponse
	if err := c.get(ctx, "/leagues?id="+url.QueryEscape(leagueID), &resp); err != nil {
		return nil, err
	}
	matches := make([]Match, 0, len(resp.Fixtures.AllMatches))
	for _, m := range resp.Fixtures.AllMatches {
		if m.Status.Cancelled {
			continue
		}
		matches = append(matches, Match{ID: m.ID, Home: m.Home.Name, HomeID: string(m.Home.ID), Away: m.Away.Name, AwayID: string(m.Away.ID), Kickoff: m.Status.UTCTime.UTC(), Finished: m.Status.Finished})
	}
	return matches, nil
}

type SquadPlayer struct {
	ID       string
	Name     string
	Position string
	ClubID   string
}

type Fixture struct {
	ID       string
	LeagueID string
	HomeID   string
	Home     string
	AwayID   string
	Away     string
	Kickoff  time.Time
}

const coachGroupTitle = "coach"

func (c *Client) TeamSquad(ctx context.Context, teamID string) ([]SquadPlayer, error) {
	var resp teamResponse
	if err := c.get(ctx, "/teams?id="+url.QueryEscape(teamID), &resp); err != nil {
		return nil, err
	}
	var players []SquadPlayer
	for _, group := range resp.Squad.Squad {
		if group.Title == coachGroupTitle {
			continue
		}
		for _, m := range group.Members {
			players = append(players, SquadPlayer{ID: string(m.ID), Name: m.Name, Position: group.Title, ClubID: clubIDOf(m)})
		}
	}
	return players, nil
}

func clubIDOf(m squadMember) string {
	id := string(m.ClubID)
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return ""
	}
	return id
}

func (c *Client) FinishedFixtures(ctx context.Context, teamID string) ([]Fixture, error) {
	var resp teamResponse
	if err := c.get(ctx, "/teams?id="+url.QueryEscape(teamID), &resp); err != nil {
		return nil, err
	}
	var fixtures []Fixture
	for _, f := range resp.Fixtures.AllFixtures.Fixtures {
		if !f.Status.Finished || f.Status.Cancelled {
			continue
		}
		fixtures = append(fixtures, Fixture{
			ID: string(f.ID), LeagueID: string(f.Tournament.LeagueID), Kickoff: f.Status.UTCTime.UTC(),
			HomeID: string(f.Home.ID), Home: f.Home.Name, AwayID: string(f.Away.ID), Away: f.Away.Name,
		})
	}
	return fixtures, nil
}

func (c *Client) LeagueCodeOf(leagueID string) (string, bool) {
	for code, id := range c.leagueIDs {
		if id == leagueID {
			return code, true
		}
	}
	return "", false
}

func (c *Client) MatchDetails(ctx context.Context, matchID string) (Details, error) {
	var resp matchResponse
	if err := c.get(ctx, "/matchDetails?matchId="+url.QueryEscape(matchID), &resp); err != nil {
		return Details{}, err
	}
	return Details{raw: resp}, nil
}

func TeamLogoURL(teamID string) string {
	if teamID == "" {
		return ""
	}
	return imageBaseURL + "/teamlogo/" + url.PathEscape(teamID) + ".png"
}

func (c *Client) LeagueLogoURL(leagueCode string) string {
	leagueID, ok := c.leagueIDs[leagueCode]
	if !ok {
		return ""
	}
	return imageBaseURL + "/leaguelogo/" + url.PathEscape(leagueID) + ".png"
}

func (c *Client) get(ctx context.Context, pathAndQuery string, out any) error {
	if err := c.fetcher.JSON(ctx, c.baseURL+pathAndQuery, out); err != nil {
		return fmt.Errorf("fotmob %s: %w", pathAndQuery, err)
	}
	return nil
}

func FindMatch(matches []Match, kickoff time.Time, home, away string) (Match, bool) {
	for _, m := range matches {
		if m.Kickoff.Sub(kickoff).Abs() > kickoffTolerance {
			continue
		}
		if names.TeamsLikelySame(m.Home, home) || names.TeamsLikelySame(m.Away, away) {
			return m, true
		}
	}
	return Match{}, false
}
