package onexbet

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"mybestbet/pkg/canon"
)

type response[T any] struct {
	Success   bool   `json:"Success"`
	Error     string `json:"Error"`
	ErrorCode int    `json:"ErrorCode"`
	Value     T      `json:"Value"`
}

func (r response[T]) err() error {
	if r.Success {
		return nil
	}
	return fmt.Errorf("line feed refused request: %q", r.Error)
}

type championship struct {
	LeagueID int64  `json:"LI"`
	Name     string `json:"L"`
	Games    []game `json:"G"`
}

type championshipSummary struct {
	ID   int64  `json:"LI"`
	Name string `json:"L"`
}

type game struct {
	ID          int64         `json:"I"`
	SportID     int           `json:"SI"`
	LeagueID    int64         `json:"LI"`
	Home        string        `json:"O1"`
	Away        string        `json:"O2"`
	HomeID      int64         `json:"O1I"`
	AwayID      int64         `json:"O2I"`
	StartUnix   int64         `json:"S"`
	Topic       string        `json:"TG"`
	Description string        `json:"DI"`
	Period      *int          `json:"P"`
	Groups      []groupEvents `json:"GE"`
	SubGames    []subGame     `json:"SG"`
}

func (g game) isPlainFixture() bool {
	return g.Topic == "" && g.Description == "" && g.Home != "" && g.Away != ""
}

func (g game) teamName(side int) string {
	switch side {
	case homeTeamSide:
		return g.Home
	case awayTeamSide:
		return g.Away
	}
	return ""
}

func (g game) competitor(side int) *canon.SourcePlayer {
	name, id := g.Home, g.HomeID
	if side == awayTeamSide {
		name, id = g.Away, g.AwayID
	}
	if name == "" || id == 0 {
		return nil
	}
	return &canon.SourcePlayer{SourceID: strconv.FormatInt(id, 10), Name: name, Team: name}
}

func (g *game) inheritCompetitors(main game) {
	g.Home, g.Away, g.HomeID, g.AwayID = main.Home, main.Away, main.HomeID, main.AwayID
}

type subGame struct {
	ID     int64  `json:"I"`
	Topic  string `json:"TG"`
	Period *int   `json:"P"`
}

type groupEvents struct {
	Group   int     `json:"G"`
	Columns [][]bet `json:"E"`
}

type bet struct {
	Type    int         `json:"T"`
	Odds    float64     `json:"C"`
	Param   *float64    `json:"P"`
	Blocked blockedFlag `json:"B"`
	Player  *linePlayer `json:"PL"`
}

type linePlayer struct {
	Name     string `json:"N"`
	ID       int64  `json:"I"`
	TeamSide int    `json:"T"`
}

const (
	homeTeamSide = 1
	awayTeamSide = 2
)

type blockedFlag bool

var errUnexpectedBlockedFlag = errors.New("unexpected blocked flag")

func (f *blockedFlag) UnmarshalJSON(data []byte) error {
	switch string(bytes.TrimSpace(data)) {
	case "true", "1":
		*f = true
	case "false", "0", "null":
		*f = false
	default:
		return fmt.Errorf("%w: %s", errUnexpectedBlockedFlag, data)
	}
	return nil
}
