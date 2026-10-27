package sporty

import (
	"fmt"
	"strings"
)

const bizCodeOK = 10000

type envelope[T any] struct {
	BizCode int    `json:"bizCode"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func (e envelope[T]) err() error {
	if e.BizCode == bizCodeOK {
		return nil
	}
	return fmt.Errorf("bizCode %d: %s", e.BizCode, e.Message)
}

type tournament struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CategoryName string  `json:"categoryName"`
	Events       []event `json:"events"`
}

type sportTree struct {
	Sports []treeSport `json:"sportList"`
}

type treeSport struct {
	ID         string         `json:"id"`
	Categories []treeCategory `json:"categories"`
}

type treeCategory struct {
	Name        string           `json:"name"`
	Tournaments []treeTournament `json:"tournaments"`
}

type treeTournament struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type event struct {
	EventID           string     `json:"eventId"`
	EstimateStartTime int64      `json:"estimateStartTime"`
	HomeTeamName      string     `json:"homeTeamName"`
	AwayTeamName      string     `json:"awayTeamName"`
	Sport             eventSport `json:"sport"`
	Markets           []market   `json:"markets"`
}

type eventSport struct {
	ID string `json:"id"`
}

type market struct {
	ID        string    `json:"id"`
	Desc      string    `json:"desc"`
	Specifier string    `json:"specifier"`
	Status    int       `json:"status"`
	Outcomes  []outcome `json:"outcomes"`
}

const marketOpen = 0

func (m market) specifierValue(key string) (string, bool) {
	for pair := range strings.SplitSeq(m.Specifier, "|") {
		k, v, ok := strings.Cut(pair, "=")
		if ok && k == key {
			return v, true
		}
	}
	return "", false
}

func (m market) sourceID() string {
	if m.Specifier == "" {
		return m.ID
	}
	return m.ID + "@" + m.Specifier
}

type outcome struct {
	ID       string `json:"id"`
	Odds     string `json:"odds"`
	IsActive int    `json:"isActive"`
	Desc     string `json:"desc"`
}

const outcomeActive = 1
