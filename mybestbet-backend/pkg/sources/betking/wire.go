package betking

import "time"

type menuResponse struct {
	Sports []menuSport `json:"Sports"`
}

type menuSport struct {
	Category []menuCategory `json:"Category"`
}

type menuCategory struct {
	ItemName    string           `json:"ItemName"`
	Tournaments []menuTournament `json:"Tournaments"`
}

type menuTournament struct {
	ItemID         int64  `json:"ItemID"`
	ItemName       string `json:"ItemName"`
	ItemEventCount int    `json:"ItemEventCount"`
}

type feedResponse struct {
	AreaMatches []areaMatch `json:"AreaMatches"`
}

type areaMatch struct {
	SportID sportID     `json:"SportID"`
	Items   []matchItem `json:"Items"`
}

type matchItem struct {
	ItemID         int64            `json:"ItemID"`
	ItemDate       time.Time        `json:"ItemDate"`
	Teams          []team           `json:"Teams"`
	OddsCollection []oddsCollection `json:"OddsCollection"`
	Goalscorer     *goalscorer      `json:"Goalscorer"`
	sport          sportID
}

type team struct {
	Name      string `json:"Name"`
	ItemOrder int    `json:"ItemOrder"`
}

type goalscorer struct {
	GoalscorerTeams []goalscorerTeam `json:"GoalscorerTeams"`
}

type goalscorerTeam struct {
	TeamName          string             `json:"TeamName"`
	GoalscorerPlayers []goalscorerPlayer `json:"GoalscorerPlayers"`
}

type goalscorerPlayer struct {
	PlayerName string `json:"PlayerName"`
}

type oddsCollection struct {
	OddCollectionID int64      `json:"OddCollectionID"`
	OddsType        oddsType   `json:"OddsType"`
	MatchOdds       []matchOdd `json:"MatchOdds"`
}

type oddsType struct {
	OddsTypeID   int    `json:"OddsTypeID"`
	OddsTypeName string `json:"OddsTypeName"`
}

type matchOdd struct {
	MatchOddsID  int64        `json:"MatchOddsID"`
	OddAttribute oddAttribute `json:"OddAttribute"`
	Outcome      outcome      `json:"Outcome"`
}

type oddAttribute struct {
	OddTypeID    int     `json:"OddTypeID"`
	OddName      string  `json:"OddName"`
	SpecialValue float64 `json:"SpecialValue"`
}

type outcome struct {
	OddOutcome     float64 `json:"OddOutcome"`
	ScheduleStatus int     `json:"ScheduleStatus"`
}
