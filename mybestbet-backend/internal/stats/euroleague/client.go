package euroleague

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/sources"
)

const (
	SourceName       = "euroleague"
	DefaultBaseURL   = "https://api-live.euroleague.net"
	seasonStartMonth = time.July
	playerPersonType = "J"
	secondsPerMinute = 60
)

var ErrUnknownCompetition = errors.New("euroleague: no competition for league code")

var Competitions = map[string]string{"EUROLEAGUE": "E", "EUROCUP": "U"}

type Client struct {
	deps        sources.Deps
	baseURL     string
	liveBaseURL string
}

type Club struct {
	Code      string
	Name      string
	ShortName string
	LogoURL   string
}

type Game struct {
	Code    int
	Kickoff time.Time
	Played  bool
	Home    Club
	Away    Club
}

type Player struct {
	Code     string
	Name     string
	ClubCode string
	Position string
}

type PlayerLine struct {
	Player  Player
	IsHome  bool
	Minutes int
	Started bool
	Values  statkey.Values
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	liveBaseURL := strings.TrimRight(deps.Config.Extra[liveBaseURLExtra], "/")
	if liveBaseURL == "" {
		liveBaseURL = DefaultLiveBaseURL
	}
	return &Client{deps: deps, baseURL: baseURL, liveBaseURL: liveBaseURL}
}

func SeasonCode(competition string, at time.Time) string {
	year := at.UTC().Year()
	if at.UTC().Month() < seasonStartMonth {
		year--
	}
	return competition + strconv.Itoa(year)
}

func (c *Client) seasonPath(leagueCode string, at time.Time) (string, error) {
	competition, ok := Competitions[leagueCode]
	if !ok {
		return "", fmt.Errorf("%s: %w", leagueCode, ErrUnknownCompetition)
	}
	return c.seasonCodePath(SeasonCode(competition, at)), nil
}

func (c *Client) seasonCodePath(seasonCode string) string {
	return fmt.Sprintf("%s/v2/competitions/%s/seasons/%s", c.baseURL, seasonCode[:1], seasonCode)
}

func (c *Client) Games(ctx context.Context, leagueCode string, at time.Time) ([]Game, error) {
	base, err := c.seasonPath(leagueCode, at)
	if err != nil {
		return nil, err
	}
	var resp gamesResponse
	if err := c.deps.GetJSON(ctx, base+"/games", &resp); err != nil {
		return nil, fmt.Errorf("euroleague %s games: %w", leagueCode, err)
	}
	games := make([]Game, 0, len(resp.Data))
	for _, g := range resp.Data {
		games = append(games, Game{Code: g.GameCode, Kickoff: g.UTCDate.UTC(), Played: g.Played, Home: clubFrom(g.Local.Club), Away: clubFrom(g.Road.Club)})
	}
	return games, nil
}

func (c *Client) Players(ctx context.Context, leagueCode string, at time.Time) ([]Player, error) {
	base, err := c.seasonPath(leagueCode, at)
	if err != nil {
		return nil, err
	}
	var resp peopleResponse
	if err := c.deps.GetJSON(ctx, base+"/people?personType="+playerPersonType, &resp); err != nil {
		return nil, fmt.Errorf("euroleague %s people: %w", leagueCode, err)
	}
	players := make([]Player, 0, len(resp.Data))
	for _, p := range resp.Data {
		if p.Type == playerPersonType && p.Active {
			players = append(players, Player{Code: p.Person.Code, Name: DisplayName(p.Person.Name), ClubCode: p.Club.Code, Position: p.PositionName})
		}
	}
	return players, nil
}

func (c *Client) GameStats(ctx context.Context, seasonCode string, gameCode int) ([]PlayerLine, error) {
	if seasonCode == "" {
		return nil, fmt.Errorf("game %d: %w", gameCode, ErrUnknownCompetition)
	}
	var resp gameStatsResponse
	if err := c.deps.GetJSON(ctx, fmt.Sprintf("%s/games/%d/stats", c.seasonCodePath(seasonCode), gameCode), &resp); err != nil {
		return nil, fmt.Errorf("euroleague %s game %d stats: %w", seasonCode, gameCode, err)
	}
	lines := sideLines(resp.Local.Players, true)
	return append(lines, sideLines(resp.Road.Players, false)...), nil
}

func sideLines(players []gamePlayerWire, isHome bool) []PlayerLine {
	lines := make([]PlayerLine, 0, len(players))
	for _, p := range players {
		if p.Stats.TimePlayed <= 0 {
			continue
		}
		lines = append(lines, PlayerLine{
			Player:  Player{Code: p.Player.Person.Code, Name: DisplayName(p.Player.Person.Name), ClubCode: p.Player.Club.Code},
			IsHome:  isHome,
			Minutes: int(p.Stats.TimePlayed) / secondsPerMinute,
			Started: p.Stats.StartFive,
			Values: statkey.Values{
				statkey.Points: p.Stats.Points, statkey.Rebounds: p.Stats.TotalRebounds,
				statkey.Assists: p.Stats.Assistances, statkey.Threes: p.Stats.FieldGoalsMade3,
			},
		})
	}
	return lines
}

func clubFrom(c clubWire) Club {
	return Club{Code: c.Code, Name: c.Name, ShortName: c.EditorialName, LogoURL: c.Images.Crest}
}

func DisplayName(surnameFirst string) string {
	surname, given, found := strings.Cut(surnameFirst, ",")
	if !found {
		return cases.Title(language.English).String(strings.ToLower(strings.TrimSpace(surnameFirst)))
	}
	return cases.Title(language.English).String(strings.ToLower(strings.TrimSpace(given) + " " + strings.TrimSpace(surname)))
}
