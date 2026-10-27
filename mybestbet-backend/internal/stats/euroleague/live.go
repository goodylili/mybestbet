package euroleague

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	json "github.com/goccy/go-json"
)

const (
	DefaultLiveBaseURL = "https://live.euroleague.net"
	liveBaseURLExtra   = "liveBaseUrl"
)

type GameState struct {
	Started   bool
	Live      bool
	Finished  bool
	HomeScore int
	AwayScore int
	Quarter   int
	Clock     string
}

func (c *Client) GameState(ctx context.Context, seasonCode string, gameCode int) (GameState, error) {
	if seasonCode == "" {
		return GameState{}, fmt.Errorf("game %d: %w", gameCode, ErrUnknownCompetition)
	}
	body, err := c.deps.Get(ctx, fmt.Sprintf("%s/api/Header?gamecode=%d&seasoncode=%s", c.liveBaseURL, gameCode, url.QueryEscape(seasonCode)))
	if err != nil {
		return GameState{}, fmt.Errorf("euroleague %s game %d header: %w", seasonCode, gameCode, err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return GameState{}, nil
	}
	var header headerWire
	if err := json.Unmarshal(body, &header); err != nil {
		return GameState{}, fmt.Errorf("decode euroleague %s game %d header: %w", seasonCode, gameCode, err)
	}
	if header.Live {
		return GameState{
			Started: true, Live: true, HomeScore: numberOf(header.ScoreA), AwayScore: numberOf(header.ScoreB),
			Quarter: numberOf(header.Quarter), Clock: strings.TrimSpace(header.RemainingPartialTime),
		}, nil
	}
	var game gameWire
	if err := c.deps.GetJSON(ctx, fmt.Sprintf("%s/games/%d", c.seasonCodePath(seasonCode), gameCode), &game); err != nil {
		return GameState{}, fmt.Errorf("euroleague %s game %d: %w", seasonCode, gameCode, err)
	}
	if !game.Played {
		return GameState{}, nil
	}
	return GameState{Started: true, Finished: true, HomeScore: game.Local.Score, AwayScore: game.Road.Score}, nil
}

func numberOf(text string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(text))
	return n
}
