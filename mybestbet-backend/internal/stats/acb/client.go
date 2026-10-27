package acb

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/sources"
)

const (
	SourceName           = "acb"
	LeagueCode           = "ACB"
	DefaultBaseURL       = "https://api2.acb.com"
	DefaultKeyPageURL    = "https://acb.com/es/liga"
	keyPageURLExtra      = "keyPageUrl"
	ligaEndesaID         = 1
	boxscoreTotalsPeriod = 0
)

type MatchStatus string

const (
	StatusFinalized  MatchStatus = "FINALIZED"
	StatusStarted    MatchStatus = "STARTED"
	StatusPostponed  MatchStatus = "POSTPONED"
	StatusSuspended  MatchStatus = "SUSPENDED"
	StatusNotStarted MatchStatus = "NOT_STARTED"
)

var ErrEditionNotFound = errors.New("acb: current edition not listed")

type Client struct {
	deps       sources.Deps
	baseURL    string
	keyPageURL string

	apiKeyMu sync.Mutex
	apiKey   string
}

type Week struct {
	ID    int
	Start time.Time
}

type Team struct {
	ClubID       string
	Name         string
	ShortName    string
	Abbreviation string
	LogoURL      string
}

type Match struct {
	ID        int64
	Start     time.Time
	Status    MatchStatus
	Home      Team
	Away      Team
	HomeScore int
	AwayScore int
	Quarter   int
	HalfTime  bool
}

type Player struct {
	ID   string
	Name string
}

type PlayerLine struct {
	Player  Player
	Club    Team
	Played  bool
	Minutes int
	Started bool
	Values  statkey.Values
}

type Boxscore struct {
	Finished bool
	Lines    []PlayerLine
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	keyPageURL := deps.Config.Extra[keyPageURLExtra]
	if keyPageURL == "" {
		keyPageURL = DefaultKeyPageURL
	}
	return &Client{deps: deps, baseURL: baseURL, keyPageURL: keyPageURL}
}

func (c *Client) CurrentWeeks(ctx context.Context) ([]Week, error) {
	var resp competitionDataWire
	if _, err := c.getAPI(ctx, "/api/matchdata/Menu/competition-data?competitionIds="+strconv.Itoa(ligaEndesaID), &resp); err != nil {
		return nil, err
	}
	for _, competition := range resp.Competitions {
		for _, edition := range competition.Editions {
			if competition.ID != ligaEndesaID || edition.ID != resp.CurrentEditionID {
				continue
			}
			weeks := make([]Week, 0, len(edition.Weeks))
			for _, w := range edition.Weeks {
				start, err := time.Parse(time.DateOnly, w.StartDate)
				if err != nil {
					return nil, fmt.Errorf("acb week %d start: %w", w.ID, err)
				}
				weeks = append(weeks, Week{ID: w.ID, Start: start})
			}
			return weeks, nil
		}
	}
	return nil, fmt.Errorf("edition %d: %w", resp.CurrentEditionID, ErrEditionNotFound)
}

func WeekOf(weeks []Week, at time.Time) (Week, bool) {
	var found Week
	for _, w := range weeks {
		if !w.Start.After(at) && w.Start.After(found.Start) {
			found = w
		}
	}
	return found, found.ID != 0
}

func (c *Client) WeekMatches(ctx context.Context, weekID int) ([]Match, error) {
	var resp matchListWire
	if _, err := c.getAPI(ctx, "/api/matchdata/Menu/matchlist?weekId="+strconv.Itoa(weekID), &resp); err != nil {
		return nil, err
	}
	matches := make([]Match, 0, len(resp.Matches))
	for _, m := range resp.Matches {
		matches = append(matches, Match{
			ID: m.ID, Start: m.StartDateTime.UTC(), Status: MatchStatus(m.MatchStatus),
			Home: teamFrom(m.HomeTeam), Away: teamFrom(m.AwayTeam),
			HomeScore: m.HomeScore, AwayScore: m.AwayScore, Quarter: m.Quarter, HalfTime: m.IsOnHalfTime,
		})
	}
	return matches, nil
}

func (c *Client) Boxscore(ctx context.Context, matchID int64) (Boxscore, error) {
	var resp boxscoresWire
	hasContent, err := c.getAPI(ctx, "/api/matchdata/Result/boxscores?matchId="+strconv.FormatInt(matchID, 10), &resp)
	if err != nil || !hasContent {
		return Boxscore{}, err
	}
	box := Boxscore{Finished: resp.MatchFinished}
	for _, side := range resp.TeamBoxscores {
		club := teamFrom(side.Team)
		for _, period := range side.StatsByPeriods {
			if period.Quarter != boxscoreTotalsPeriod {
				continue
			}
			for _, p := range period.Stats.Players {
				line, err := lineFrom(club, p)
				if err != nil {
					return Boxscore{}, fmt.Errorf("acb match %d: %w", matchID, err)
				}
				box.Lines = append(box.Lines, line)
			}
		}
	}
	return box, nil
}

func lineFrom(club Team, p boxscorePlayerWire) (PlayerLine, error) {
	seconds, err := playTimeSeconds(p.PlayTime)
	if err != nil {
		return PlayerLine{}, err
	}
	return PlayerLine{
		Player: Player{ID: strconv.FormatInt(p.Player.ID, 10), Name: displayName(p)}, Club: club,
		Played: seconds > 0, Minutes: seconds / 60, Started: p.IsStarted,
		Values: statkey.Values{statkey.Points: p.Points, statkey.Rebounds: p.TotalRebounds, statkey.Assists: p.Assists, statkey.Threes: p.ThreePointersMade},
	}, nil
}

func playTimeSeconds(playTime string) (int, error) {
	minutesText, secondsText, found := strings.Cut(playTime, ":")
	minutes, minutesErr := strconv.Atoi(minutesText)
	seconds, secondsErr := strconv.Atoi(secondsText)
	if !found || minutesErr != nil || secondsErr != nil {
		return 0, fmt.Errorf("acb play time %q: %w", playTime, strconv.ErrSyntax)
	}
	return minutes*60 + seconds, nil
}

func displayName(p boxscorePlayerWire) string {
	if name := strings.TrimSpace(p.Player.Nickname); name != "" {
		return name
	}
	return strings.TrimSpace(p.Player.FirstName + " " + p.Player.LastName)
}

func teamFrom(t teamWire) Team {
	return Team{ClubID: strconv.FormatInt(t.ClubID, 10), Name: t.FullName, ShortName: t.ShortName, Abbreviation: t.AbbreviatedName, LogoURL: t.Logo}
}
