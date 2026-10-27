package sofascore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mybestbet/internal/stats/fetch"
	"mybestbet/pkg/sources"
)

const (
	SourceName     = "sofascore"
	DefaultBaseURL = "https://www.sofascore.com/api/v1"
	maxEventPages  = 10
)

var ErrNoSeason = errors.New("sofascore: unique tournament has no seasons")

type Client struct {
	fetcher fetch.Fetcher
	baseURL string
}

func New(deps sources.Deps) *Client {
	baseURL := strings.TrimRight(deps.Config.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{fetcher: fetch.Fetcher{Deps: deps, Profile: SourceName}, baseURL: baseURL}
}

func (c *Client) CurrentSeason(ctx context.Context, uniqueTournamentID int64) (Season, error) {
	var resp seasonsResponse
	if err := c.get(ctx, fmt.Sprintf("/unique-tournament/%d/seasons", uniqueTournamentID), &resp); err != nil {
		return Season{}, err
	}
	if len(resp.Seasons) == 0 {
		return Season{}, fmt.Errorf("tournament %d: %w", uniqueTournamentID, ErrNoSeason)
	}
	return resp.Seasons[0], nil
}

func (c *Client) SeasonTeams(ctx context.Context, uniqueTournamentID, seasonID int64) ([]Team, error) {
	var resp teamsResponse
	err := c.get(ctx, fmt.Sprintf("/unique-tournament/%d/season/%d/teams", uniqueTournamentID, seasonID), &resp)
	return resp.Teams, err
}

func (c *Client) TeamPlayers(ctx context.Context, teamID int64) ([]Player, error) {
	var resp squadResponse
	if err := c.get(ctx, fmt.Sprintf("/team/%d/players", teamID), &resp); err != nil {
		return nil, err
	}
	players := make([]Player, 0, len(resp.Players))
	for _, p := range resp.Players {
		players = append(players, p.Player)
	}
	return players, nil
}

func (c *Client) UpcomingEvents(ctx context.Context, uniqueTournamentID, seasonID int64, until time.Time) ([]Event, error) {
	var events []Event
	for page := range maxEventPages {
		var resp eventsPage
		err := c.get(ctx, fmt.Sprintf("/unique-tournament/%d/season/%d/events/next/%d", uniqueTournamentID, seasonID, page), &resp)
		if errors.Is(err, sources.ErrNotFound) {
			return events, nil
		}
		if err != nil {
			return events, err
		}
		pastHorizon := false
		for _, e := range resp.Events {
			if e.Kickoff().After(until) {
				pastHorizon = true
				continue
			}
			events = append(events, e)
		}
		if pastHorizon || !resp.HasNextPage {
			return events, nil
		}
	}
	return events, nil
}

func (c *Client) Event(ctx context.Context, eventID int64) (Event, error) {
	var resp eventResponse
	err := c.get(ctx, fmt.Sprintf("/event/%d", eventID), &resp)
	return resp.Event, err
}

func (c *Client) Lineups(ctx context.Context, eventID int64) (Lineups, error) {
	var resp Lineups
	err := c.get(ctx, fmt.Sprintf("/event/%d/lineups", eventID), &resp)
	return resp, err
}

func (c *Client) Incidents(ctx context.Context, eventID int64) ([]Incident, error) {
	var resp incidentsResponse
	err := c.get(ctx, fmt.Sprintf("/event/%d/incidents", eventID), &resp)
	return resp.Incidents, err
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	if err := c.fetcher.JSON(ctx, c.baseURL+path, out); err != nil {
		return fmt.Errorf("sofascore %s: %w", path, err)
	}
	return nil
}

func (e Event) Kickoff() time.Time { return time.Unix(e.StartTimestamp, 0).UTC() }

func (e Event) IsFinished() bool { return e.Status.Type == "finished" }
