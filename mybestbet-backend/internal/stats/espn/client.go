package espn

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	SourceName     = "espn"
	DefaultBaseURL = "https://site.api.espn.com/apis/site/v2/sports"
	scoreboardDate = "20060102"
	maxListedItems = "1000"
)

var ErrUnknownLeague = errors.New("espn: no league path for league code")

type League struct {
	Path          string
	Group         string
	ListsAllTeams bool
	PlaysHalves   bool
}

var Leagues = map[string]League{
	"NBA":   {Path: "basketball/nba", ListsAllTeams: true},
	"WNBA":  {Path: "basketball/wnba", ListsAllTeams: true},
	"NBL":   {Path: "basketball/nbl", ListsAllTeams: true},
	"NCAAB": {Path: "basketball/mens-college-basketball", Group: "50", PlaysHalves: true},
	"NFL":   {Path: "football/nfl", ListsAllTeams: true},
	"NCAAF": {Path: "football/college-football", Group: "80"},
}

type Client struct {
	deps    sources.Deps
	baseURL string
}

type Team struct {
	ID           string
	Name         string
	ShortName    string
	Abbreviation string
	Location     string
	LogoURL      string
}

type Athlete struct {
	ID       string
	Name     string
	Position string
}

type Scoreboard struct {
	LeagueLogoURL string
	Games         []Game
}

type Game struct {
	ID      string
	Kickoff time.Time
	Status  string
	State   string
	Home    Team
	Away    Team
	Score   canon.EventScore
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{deps: deps, baseURL: baseURL}
}

func leagueOf(code string) (League, error) {
	league, ok := Leagues[code]
	if !ok {
		return League{}, fmt.Errorf("%s: %w", code, ErrUnknownLeague)
	}
	return league, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	target := c.baseURL + "/" + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return c.deps.GetJSON(ctx, target, out)
}

func (c *Client) Games(ctx context.Context, leagueCode string, day time.Time) (Scoreboard, error) {
	league, err := leagueOf(leagueCode)
	if err != nil {
		return Scoreboard{}, err
	}
	query := url.Values{"dates": {day.UTC().Format(scoreboardDate)}}
	if league.Group != "" {
		query.Set("groups", league.Group)
		query.Set("limit", maxListedItems)
	}
	var resp scoreboardResponse
	if err := c.get(ctx, league.Path+"/scoreboard", query, &resp); err != nil {
		return Scoreboard{}, fmt.Errorf("espn %s scoreboard: %w", leagueCode, err)
	}
	board := Scoreboard{Games: make([]Game, 0, len(resp.Events))}
	if len(resp.Leagues) > 0 {
		board.LeagueLogoURL = firstLogo(resp.Leagues[0].Logos)
	}
	for _, e := range resp.Events {
		if game, ok := gameFrom(e, league); ok {
			board.Games = append(board.Games, game)
		}
	}
	return board, nil
}

func gameFrom(e eventWire, league League) (Game, bool) {
	if len(e.Competitions) == 0 {
		return Game{}, false
	}
	competition := e.Competitions[0]
	game := Game{ID: e.ID, Kickoff: competition.Date.Time, Status: competition.Status.Type.Name, State: competition.Status.Type.State}
	game.Score = scoreOf(competition.Status, league)
	for _, c := range competition.Competitors {
		if c.Team == nil {
			continue
		}
		switch c.HomeAway {
		case "home":
			game.Home, game.Score.Home = teamFrom(*c.Team), pointsOf(c.Score)
		case "away":
			game.Away, game.Score.Away = teamFrom(*c.Team), pointsOf(c.Score)
		}
	}
	return game, game.Home.ID != "" && game.Away.ID != ""
}

func teamFrom(t teamWire) Team {
	logo := t.Logo
	if logo == "" {
		logo = firstLogo(t.Logos)
	}
	return Team{ID: t.ID, Name: t.DisplayName, ShortName: t.ShortDisplayName, Abbreviation: t.Abbreviation, Location: t.Location, LogoURL: logo}
}

func (c *Client) Teams(ctx context.Context, leagueCode string) ([]Team, error) {
	league, err := leagueOf(leagueCode)
	if err != nil {
		return nil, err
	}
	var resp teamsResponse
	if err := c.get(ctx, league.Path+"/teams", url.Values{"limit": {maxListedItems}}, &resp); err != nil {
		return nil, fmt.Errorf("espn %s teams: %w", leagueCode, err)
	}
	var teams []Team
	for _, sport := range resp.Sports {
		for _, l := range sport.Leagues {
			for _, t := range l.Teams {
				teams = append(teams, teamFrom(t.Team))
			}
		}
	}
	return teams, nil
}

func (c *Client) Roster(ctx context.Context, leagueCode, teamID string) ([]Athlete, error) {
	league, err := leagueOf(leagueCode)
	if err != nil {
		return nil, err
	}
	var resp rosterResponse
	if err := c.get(ctx, league.Path+"/teams/"+url.PathEscape(teamID)+"/roster", nil, &resp); err != nil {
		return nil, fmt.Errorf("espn %s roster %s: %w", leagueCode, teamID, err)
	}
	var athletes []Athlete
	for _, entry := range resp.Athletes {
		if entry.ID != "" {
			athletes = append(athletes, athleteFrom(entry.athleteWire))
		}
		for _, item := range entry.Items {
			athletes = append(athletes, athleteFrom(item))
		}
	}
	return athletes, nil
}

func athleteFrom(a athleteWire) Athlete {
	return Athlete{ID: a.ID, Name: a.DisplayName, Position: a.Position.Abbreviation}
}
