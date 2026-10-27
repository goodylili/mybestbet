package prizepicks

import (
	"time"

	json "github.com/goccy/go-json"
)

type projectionsDocument struct {
	Data     []projectionResource `json:"data"`
	Included []includedResource   `json:"included"`
}

type projectionResource struct {
	ID            string                  `json:"id"`
	Attributes    projectionAttributes    `json:"attributes"`
	Relationships projectionRelationships `json:"relationships"`
}

type projectionAttributes struct {
	LineScore float64 `json:"line_score"`
	OddsType  string  `json:"odds_type"`
	StatType  string  `json:"stat_type"`
	Status    string  `json:"status"`
}

type projectionRelationships struct {
	NewPlayer relationship `json:"new_player"`
	Game      relationship `json:"game"`
}

type relationship struct {
	Data *resourceIdentifier `json:"data"`
}

type resourceIdentifier struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func (r relationship) id() string {
	if r.Data == nil {
		return ""
	}
	return r.Data.ID
}

type includedResource struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Attributes json.RawMessage `json:"attributes"`
}

type playerAttributes struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	TeamName    string `json:"team_name"`
}

type gameAttributes struct {
	StartTime time.Time    `json:"start_time"`
	Status    string       `json:"status"`
	Metadata  gameMetadata `json:"metadata"`
}

type gameMetadata struct {
	GameInfo gameInfo `json:"game_info"`
}

type gameInfo struct {
	Competition string    `json:"competition"`
	Teams       gameTeams `json:"teams"`
}

type gameTeams struct {
	Home gameTeam `json:"home"`
	Away gameTeam `json:"away"`
}

type gameTeam struct {
	Abbreviation string `json:"abbreviation"`
	Name         string `json:"name"`
}
