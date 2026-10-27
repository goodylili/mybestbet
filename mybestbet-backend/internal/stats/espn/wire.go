package espn

import (
	"fmt"
	"strings"
	"time"

	json "github.com/goccy/go-json"
)

var timeLayouts = []string{"2006-01-02T15:04Z07:00", time.RFC3339}

type espnTime struct{ time.Time }

func (t *espnTime) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(string(raw), `"`)
	if text == "" || text == "null" {
		return nil
	}
	for _, layout := range timeLayouts {
		if parsed, err := time.Parse(layout, text); err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return fmt.Errorf("espn time %q: unknown layout", text)
}

type statusWire struct {
	Period       int    `json:"period"`
	DisplayClock string `json:"displayClock"`
	Type         struct {
		Name        string `json:"name"`
		State       string `json:"state"`
		Completed   bool   `json:"completed"`
		ShortDetail string `json:"shortDetail"`
	} `json:"type"`
}

type teamWire struct {
	ID               string     `json:"id"`
	Location         string     `json:"location"`
	Name             string     `json:"name"`
	Abbreviation     string     `json:"abbreviation"`
	DisplayName      string     `json:"displayName"`
	ShortDisplayName string     `json:"shortDisplayName"`
	Logo             string     `json:"logo"`
	Logos            []logoWire `json:"logos"`
}

type logoWire struct {
	Href string `json:"href"`
}

func firstLogo(logos []logoWire) string {
	if len(logos) == 0 {
		return ""
	}
	return logos[0].Href
}

type athleteWire struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName"`
	Position    positionWire `json:"position"`
}

type positionWire struct {
	Abbreviation string
}

func (p *positionWire) UnmarshalJSON(raw []byte) error {
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var object struct {
		Abbreviation string `json:"abbreviation"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return fmt.Errorf("espn position: %w", err)
	}
	p.Abbreviation = object.Abbreviation
	return nil
}

type competitorWire struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	HomeAway string       `json:"homeAway"`
	Score    string       `json:"score"`
	Team     *teamWire    `json:"team"`
	Athlete  *athleteWire `json:"athlete"`
}

type competitionWire struct {
	ID          string           `json:"id"`
	Date        espnTime         `json:"date"`
	Status      statusWire       `json:"status"`
	Competitors []competitorWire `json:"competitors"`
}

type groupingWire struct {
	Grouping struct {
		Slug string `json:"slug"`
	} `json:"grouping"`
	Competitions []competitionWire `json:"competitions"`
}

type eventWire struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Date         espnTime          `json:"date"`
	Status       statusWire        `json:"status"`
	Competitions []competitionWire `json:"competitions"`
	Groupings    []groupingWire    `json:"groupings"`
}

type scoreboardResponse struct {
	Leagues []struct {
		Logos []logoWire `json:"logos"`
	} `json:"leagues"`
	Events []eventWire `json:"events"`
}

type teamsResponse struct {
	Sports []struct {
		Leagues []struct {
			Teams []struct {
				Team teamWire `json:"team"`
			} `json:"teams"`
		} `json:"leagues"`
	} `json:"sports"`
}

type rosterEntryWire struct {
	athleteWire
	Items []athleteWire `json:"items"`
}

type rosterResponse struct {
	Athletes []rosterEntryWire `json:"athletes"`
}

type boxscoreAthleteWire struct {
	Athlete    athleteWire `json:"athlete"`
	Stats      []string    `json:"stats"`
	DidNotPlay bool        `json:"didNotPlay"`
	Starter    bool        `json:"starter"`
}

type boxscoreCategoryWire struct {
	Name     string                `json:"name"`
	Keys     []string              `json:"keys"`
	Athletes []boxscoreAthleteWire `json:"athletes"`
}

type summaryResponse struct {
	Header struct {
		Competitions []struct {
			Status      statusWire       `json:"status"`
			Competitors []competitorWire `json:"competitors"`
		} `json:"competitions"`
	} `json:"header"`
	Boxscore struct {
		Players []struct {
			Team       teamWire               `json:"team"`
			Statistics []boxscoreCategoryWire `json:"statistics"`
		} `json:"players"`
	} `json:"boxscore"`
}
