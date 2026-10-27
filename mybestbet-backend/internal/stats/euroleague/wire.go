package euroleague

import "time"

type clubWire struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	EditorialName string `json:"editorialName"`
	Images        struct {
		Crest string `json:"crest"`
	} `json:"images"`
}

type gamesResponse struct {
	Data []struct {
		GameCode int       `json:"gameCode"`
		Played   bool      `json:"played"`
		UTCDate  time.Time `json:"utcDate"`
		Local    struct {
			Club clubWire `json:"club"`
		} `json:"local"`
		Road struct {
			Club clubWire `json:"club"`
		} `json:"road"`
	} `json:"data"`
}

type personWire struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type peopleResponse struct {
	Data []struct {
		Person       personWire `json:"person"`
		Type         string     `json:"type"`
		Active       bool       `json:"active"`
		PositionName string     `json:"positionName"`
		Club         clubWire   `json:"club"`
	} `json:"data"`
}

type gamePlayerWire struct {
	Player struct {
		Person personWire `json:"person"`
		Club   clubWire   `json:"club"`
	} `json:"player"`
	Stats struct {
		TimePlayed      float64 `json:"timePlayed"`
		Points          float64 `json:"points"`
		TotalRebounds   float64 `json:"totalRebounds"`
		Assistances     float64 `json:"assistances"`
		FieldGoalsMade3 float64 `json:"fieldGoalsMade3"`
		StartFive       bool    `json:"startFive"`
	} `json:"stats"`
}

type gameStatsResponse struct {
	Local struct {
		Players []gamePlayerWire `json:"players"`
	} `json:"local"`
	Road struct {
		Players []gamePlayerWire `json:"players"`
	} `json:"road"`
}

type headerWire struct {
	Live                 bool   `json:"Live"`
	ScoreA               string `json:"ScoreA"`
	ScoreB               string `json:"ScoreB"`
	Quarter              string `json:"Quarter"`
	RemainingPartialTime string `json:"RemainingPartialTime"`
}

type gameWire struct {
	Played bool `json:"played"`
	Local  struct {
		Score int `json:"score"`
	} `json:"local"`
	Road struct {
		Score int `json:"score"`
	} `json:"road"`
}
