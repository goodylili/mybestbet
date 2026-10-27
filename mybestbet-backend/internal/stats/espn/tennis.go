package espn

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	singlesAthleteType = "athlete"
	mensSingles        = "mens-singles"
	womensSingles      = "womens-singles"
)

var TennisTours = []string{"tennis/atp", "tennis/wta"}

type tennisTournament struct {
	nameFragment string
	mensCode     string
	womensCode   string
}

var tennisTournaments = []tennisTournament{
	{"miami open", "ATP_MIAMI", "WTA_MIAMI"},
	{"roland garros", "ATP_RG", "WTA_RG"},
	{"french open", "ATP_RG", "WTA_RG"},
	{"wimbledon", "ATP_WIM", "WTA_WIM"},
	{"australian open", "ATP_AO", "WTA_AO"},
	{"us open", "ATP_USO", "WTA_USO"},
}

type TennisMatch struct {
	ID         string
	LeagueCode string
	Kickoff    time.Time
	Status     string
	Home       Athlete
	Away       Athlete
}

func TennisLeagueCode(tournamentName, drawSlug string) (string, bool) {
	name := strings.ToLower(tournamentName)
	for _, t := range tennisTournaments {
		if !strings.Contains(name, t.nameFragment) {
			continue
		}
		switch drawSlug {
		case mensSingles:
			return t.mensCode, true
		case womensSingles:
			return t.womensCode, true
		}
	}
	return "", false
}

func (c *Client) TennisMatches(ctx context.Context, tourPath string, day time.Time) ([]TennisMatch, error) {
	var resp scoreboardResponse
	if err := c.get(ctx, tourPath+"/scoreboard", url.Values{"dates": {day.UTC().Format(scoreboardDate)}}, &resp); err != nil {
		return nil, fmt.Errorf("espn %s scoreboard: %w", tourPath, err)
	}
	var matches []TennisMatch
	for _, e := range resp.Events {
		for _, g := range e.Groupings {
			leagueCode, ok := TennisLeagueCode(e.Name, g.Grouping.Slug)
			if !ok {
				continue
			}
			for _, competition := range g.Competitions {
				if match, ok := tennisMatchFrom(leagueCode, competition); ok {
					matches = append(matches, match)
				}
			}
		}
	}
	return matches, nil
}

func tennisMatchFrom(leagueCode string, c competitionWire) (TennisMatch, bool) {
	match := TennisMatch{ID: c.ID, LeagueCode: leagueCode, Kickoff: c.Date.Time, Status: c.Status.Type.Name}
	for _, competitor := range c.Competitors {
		if competitor.Type != singlesAthleteType || competitor.Athlete == nil || competitor.ID == "" {
			continue
		}
		player := Athlete{ID: competitor.ID, Name: competitor.Athlete.DisplayName}
		switch competitor.HomeAway {
		case "home":
			match.Home = player
		case "away":
			match.Away = player
		}
	}
	return match, match.Home.ID != "" && match.Away.ID != ""
}
