package fbref

import (
	"strconv"
	"strings"
)

type PlayerSeason struct {
	FBrefID       string
	Name          string
	Team          string
	Season        string
	Matches       int
	Starts        int
	Minutes       int
	Goals         int
	Assists       int
	Shots         int
	ShotsOnTarget int
	Fouls         int
	Tackles       int
	Yellow        int
	Red           int
}

func (p PlayerSeason) Per90(total int) float64 {
	if p.Minutes == 0 {
		return 0
	}
	return float64(total) * 90 / float64(p.Minutes)
}

type statPage struct {
	path    string
	tableID string
	apply   func(*PlayerSeason, map[string]string)
}

var statPages = []statPage{
	{path: "stats", tableID: "stats_standard", apply: func(p *PlayerSeason, c map[string]string) {
		p.Matches += atoi(c["games"])
		p.Starts += atoi(c["games_starts"])
		p.Minutes += atoi(c["minutes"])
		p.Goals += atoi(c["goals"])
		p.Assists += atoi(c["assists"])
		p.Yellow += atoi(c["cards_yellow"])
		p.Red += atoi(c["cards_red"])
	}},
	{path: "shooting", tableID: "stats_shooting", apply: func(p *PlayerSeason, c map[string]string) {
		p.Shots += atoi(c["shots"])
		p.ShotsOnTarget += atoi(c["shots_on_target"])
	}},
	{path: "misc", tableID: "stats_misc", apply: func(p *PlayerSeason, c map[string]string) {
		p.Fouls += atoi(c["fouls"])
		p.Tackles += atoi(c["tackles_won"])
	}},
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.ReplaceAll(s, ",", ""))
	if err != nil {
		return 0
	}
	return n
}
