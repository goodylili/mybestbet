package bo3

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

const unlinkedSteamProfilePrefix = "steam:"

var gameStatsPaths = map[string]string{"VALORANT": "vlr", "LOL": "lol"}

type PlayerLine struct {
	Player Player
	TeamID string
	Kills  int
	Maps   int
}

func (c *Client) PlayerLines(ctx context.Context, leagueCode string, m Match) ([]PlayerLine, error) {
	if leagueCode == "CS2" {
		return c.counterStrikeLines(ctx, m)
	}
	statsPath, ok := gameStatsPaths[leagueCode]
	if !ok {
		return nil, fmt.Errorf("%s: %w", leagueCode, ErrUnknownDiscipline)
	}
	return c.perGameLines(ctx, statsPath, m)
}

func (c *Client) counterStrikeLines(ctx context.Context, m Match) ([]PlayerLine, error) {
	var resp []csPlayerStatWire
	if err := c.deps.GetJSON(ctx, c.baseURL+"/matches/"+url.PathEscape(m.Slug)+"/players_stats", &resp); err != nil {
		return nil, fmt.Errorf("bo3 match %s players stats: %w", m.Slug, err)
	}
	lines := make([]PlayerLine, 0, len(resp))
	for _, r := range resp {
		maps := len(r.SteamProfile.GameSteamProfiles)
		if maps == 0 {
			continue
		}
		player := Player{ID: unlinkedSteamProfilePrefix + idText(r.SteamProfile.ID), Name: r.SteamProfile.Nickname}
		if r.SteamProfile.Player != nil {
			player = playerFrom(*r.SteamProfile.Player)
		}
		lines = append(lines, PlayerLine{Player: player, TeamID: idText(r.TeamClan.TeamID), Kills: r.Kills, Maps: maps})
	}
	return lines, nil
}

func (c *Client) perGameLines(ctx context.Context, statsPath string, m Match) ([]PlayerLine, error) {
	matchPath := c.baseURL + "/" + statsPath + "/stats/matches/" + strconv.FormatInt(m.ID, 10)
	var games []gameWire
	if err := c.deps.GetJSON(ctx, matchPath+"/players_stats", &games); err != nil {
		return nil, fmt.Errorf("bo3 %s match %d players stats: %w", statsPath, m.ID, err)
	}
	var lineup []lineupWire
	if err := c.deps.GetJSON(ctx, matchPath+"/lineup", &lineup); err != nil {
		return nil, fmt.Errorf("bo3 %s match %d lineup: %w", statsPath, m.ID, err)
	}
	people := playersByLineupKey(lineup)
	byKey := map[int64]*PlayerLine{}
	var order []int64
	for _, g := range games {
		if g.Status != statusFinished {
			continue
		}
		for _, p := range append(g.VlrPlayers, g.LolPlayers...) {
			player, known := people[p.lineupKey()]
			if !known {
				continue
			}
			line, seen := byKey[p.lineupKey()]
			if !seen {
				line = &PlayerLine{Player: player, TeamID: idText(p.TeamID)}
				byKey[p.lineupKey()] = line
				order = append(order, p.lineupKey())
			}
			line.Kills += p.Kills
			line.Maps++
		}
	}
	lines := make([]PlayerLine, 0, len(order))
	for _, key := range order {
		lines = append(lines, *byKey[key])
	}
	return lines, nil
}

func playersByLineupKey(lineup []lineupWire) map[int64]Player {
	people := make(map[int64]Player, len(lineup))
	for _, l := range lineup {
		switch {
		case l.Player != nil:
			people[l.lineupKey()] = playerFrom(*l.Player)
		case l.VlrPlayer != nil && l.VlrPlayer.Player != nil:
			people[l.lineupKey()] = playerFrom(*l.VlrPlayer.Player)
		}
	}
	return people
}
