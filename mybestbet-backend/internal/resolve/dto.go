package resolve

import (
	"time"

	"mybestbet/pkg/canon"
)

type Team struct {
	ID     string
	League string
	Name   string
}

type Event struct {
	ID         string
	League     string
	HomeTeamID string
	AwayTeamID string
	Kickoff    time.Time
}

func (e Event) involves(teamID string) bool { return teamID == e.HomeTeamID || teamID == e.AwayTeamID }

func (e Event) opponentOf(teamID string) string {
	if teamID == e.HomeTeamID {
		return e.AwayTeamID
	}
	return e.HomeTeamID
}

type Player struct {
	ID     string
	Name   string
	TeamID string
}

type PolledEvent struct {
	SourceEventID string
	SourceKeys    []string
	Complete      bool
}

type EventLink struct {
	EventID string
	Flipped bool
}

type PendingMarket struct {
	Kind      string          `json:"kind"`
	Source    string          `json:"source"`
	SourceKey string          `json:"sourceKey"`
	Market    canon.RawMarket `json:"market"`
	FetchedAt time.Time       `json:"fetchedAt"`
}
