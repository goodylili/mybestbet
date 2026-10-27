package espn

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"mybestbet/internal/stats/statkey"
)

const (
	StatusFinal         = "STATUS_FINAL"
	minutesKey          = "minutes"
	basketballSportPath = "basketball/"
)

var statKeysByESPNKey = map[string]statkey.Key{
	"points":   statkey.Points,
	"rebounds": statkey.Rebounds,
	"assists":  statkey.Assists,
	"threePointFieldGoalsMade-threePointFieldGoalsAttempted": statkey.Threes,
	"passingYards":        statkey.PassYards,
	"passingTouchdowns":   statkey.PassTDs,
	"rushingYards":        statkey.RushYards,
	"rushingTouchdowns":   statkey.RushTDs,
	"receptions":          statkey.Receptions,
	"receivingYards":      statkey.RecYards,
	"receivingTouchdowns": statkey.RecTDs,
}

type Boxscore struct {
	Status     string
	HomeTeamID string
	Lines      []PlayerLine
}

func (b Boxscore) IsFinal() bool { return b.Status == StatusFinal }

type PlayerLine struct {
	Athlete Athlete
	TeamID  string
	Minutes int
	Started bool
	Values  statkey.Values
}

func (c *Client) Boxscore(ctx context.Context, leagueCode, eventID string) (Boxscore, error) {
	league, err := leagueOf(leagueCode)
	if err != nil {
		return Boxscore{}, err
	}
	var resp summaryResponse
	if err := c.get(ctx, league.Path+"/summary", url.Values{"event": {eventID}}, &resp); err != nil {
		return Boxscore{}, fmt.Errorf("espn %s summary %s: %w", leagueCode, eventID, err)
	}
	box := Boxscore{}
	if len(resp.Header.Competitions) > 0 {
		header := resp.Header.Competitions[0]
		box.Status = header.Status.Type.Name
		for _, c := range header.Competitors {
			if c.HomeAway == "home" {
				box.HomeTeamID = c.ID
			}
		}
	}
	sportKeys := statkey.Gridiron
	if strings.HasPrefix(league.Path, basketballSportPath) {
		sportKeys = statkey.Basketball
	}
	for _, side := range resp.Boxscore.Players {
		box.Lines = append(box.Lines, teamLines(side.Team.ID, side.Statistics, sportKeys)...)
	}
	return box, nil
}

func teamLines(teamID string, categories []boxscoreCategoryWire, sportKeys []statkey.Key) []PlayerLine {
	byAthlete := map[string]*PlayerLine{}
	var order []string
	for _, category := range categories {
		for _, a := range category.Athletes {
			if a.DidNotPlay || len(a.Stats) == 0 {
				continue
			}
			line, seen := byAthlete[a.Athlete.ID]
			if !seen {
				line = &PlayerLine{Athlete: athleteFrom(a.Athlete), TeamID: teamID, Values: statkey.Zeroed(sportKeys)}
				byAthlete[a.Athlete.ID] = line
				order = append(order, a.Athlete.ID)
			}
			line.Started = line.Started || a.Starter
			applyStats(line, category.Keys, a.Stats)
		}
	}
	lines := make([]PlayerLine, 0, len(order))
	for _, id := range order {
		lines = append(lines, *byAthlete[id])
	}
	return lines
}

func applyStats(line *PlayerLine, keys, values []string) {
	for i, key := range keys {
		if i >= len(values) {
			return
		}
		value, ok := leadingNumber(values[i])
		if !ok {
			continue
		}
		if key == minutesKey {
			line.Minutes = int(value)
			continue
		}
		if stat, known := statKeysByESPNKey[key]; known {
			line.Values[stat] = value
		}
	}
}

func leadingNumber(raw string) (float64, bool) {
	head, _, _ := strings.Cut(raw, "-")
	head, _, _ = strings.Cut(head, "/")
	value, err := strconv.ParseFloat(head, 64)
	return value, err == nil
}
