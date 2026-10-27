package acb

import "time"

type competitionDataWire struct {
	CurrentEditionID int `json:"currentEditionId"`
	Competitions     []struct {
		ID       int `json:"id"`
		Editions []struct {
			ID    int `json:"id"`
			Weeks []struct {
				ID        int    `json:"id"`
				StartDate string `json:"startDate"`
			} `json:"weeks"`
		} `json:"editions"`
	} `json:"competitions"`
}

type teamWire struct {
	ClubID          int64  `json:"clubId"`
	FullName        string `json:"fullName"`
	ShortName       string `json:"shortName"`
	AbbreviatedName string `json:"abbreviatedName"`
	Logo            string `json:"logo"`
}

type matchListWire struct {
	Matches []struct {
		ID            int64     `json:"id"`
		MatchStatus   string    `json:"matchStatus"`
		IsOnHalfTime  bool      `json:"isOnHalfTime"`
		Quarter       int       `json:"currentQuarter"`
		HomeScore     int       `json:"homeScore"`
		AwayScore     int       `json:"awayScore"`
		StartDateTime time.Time `json:"startDateTime"`
		HomeTeam      teamWire  `json:"homeTeam"`
		AwayTeam      teamWire  `json:"awayTeam"`
	} `json:"matches"`
}

type boxscorePlayerWire struct {
	Player struct {
		ID        int64  `json:"id"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Nickname  string `json:"nickname"`
	} `json:"player"`
	PlayTime          string  `json:"playTime"`
	IsStarted         bool    `json:"isStarted"`
	Points            float64 `json:"points"`
	ThreePointersMade float64 `json:"threePointersMade"`
	Assists           float64 `json:"assists"`
	TotalRebounds     float64 `json:"totalRebounds"`
}

type boxscoresWire struct {
	MatchFinished bool `json:"matchFinished"`
	TeamBoxscores []struct {
		Team           teamWire `json:"team"`
		StatsByPeriods []struct {
			Quarter int `json:"quarter"`
			Stats   struct {
				Players []boxscorePlayerWire `json:"players"`
			} `json:"stats"`
		} `json:"statsByPeriods"`
	} `json:"teamBoxscores"`
}
