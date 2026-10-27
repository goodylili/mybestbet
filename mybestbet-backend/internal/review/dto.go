package review

import (
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
)

const (
	ActionLink   = "link"
	ActionCreate = "create"
	ActionIgnore = "ignore"
)

const (
	StatusPending = "pending"
	StatusLinked  = "linked"
	StatusCreated = "created"
	StatusIgnored = "ignored"
)

type Candidate struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

type Item struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Source     string          `json:"source"`
	SourceKey  string          `json:"sourceKey"`
	Payload    json.RawMessage `json:"payload"`
	Candidates []Candidate     `json:"candidates"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type Decision struct {
	ReviewID string `json:"reviewId"`
	Action   string `json:"action"`
	TargetID string `json:"targetId"`
}

type NewItem struct {
	Kind       string
	Source     string
	SourceKey  string
	Payload    any
	Candidates []Candidate
}

type TeamPayload struct {
	League        string `json:"league"`
	Name          string `json:"name"`
	SourceEventID string `json:"sourceEventId,omitempty"`
}

type EventPayload struct {
	Event      canon.SourceEvent `json:"event"`
	League     string            `json:"league"`
	HomeTeamID string            `json:"homeTeamId"`
	AwayTeamID string            `json:"awayTeamId"`
}

type PlayerPayload struct {
	Player  canon.SourcePlayer `json:"player"`
	EventID string             `json:"eventId"`
	TeamID  string             `json:"teamId,omitempty"`
}
