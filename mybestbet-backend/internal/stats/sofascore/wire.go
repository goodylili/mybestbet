package sofascore

type Season struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Year string `json:"year"`
}

type Team struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
}

type Player struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	Position  string `json:"position"`
}

type Status struct {
	Code        int    `json:"code"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

type Event struct {
	ID             int64  `json:"id"`
	StartTimestamp int64  `json:"startTimestamp"`
	Status         Status `json:"status"`
	HomeTeam       Team   `json:"homeTeam"`
	AwayTeam       Team   `json:"awayTeam"`
	Tournament     struct {
		UniqueTournament struct {
			ID int64 `json:"id"`
		} `json:"uniqueTournament"`
	} `json:"tournament"`
	Season Season `json:"season"`
}

type PlayerStatistics struct {
	MinutesPlayed          int      `json:"minutesPlayed"`
	Goals                  int      `json:"goals"`
	GoalAssist             int      `json:"goalAssist"`
	TotalShots             int      `json:"totalShots"`
	OnTargetScoringAttempt int      `json:"onTargetScoringAttempt"`
	Fouls                  int      `json:"fouls"`
	TotalTackle            int      `json:"totalTackle"`
	Rating                 *float64 `json:"rating"`
}

type LineupPlayer struct {
	Player     Player            `json:"player"`
	TeamID     int64             `json:"teamId"`
	Substitute bool              `json:"substitute"`
	Statistics *PlayerStatistics `json:"statistics"`
}

type MissingPlayer struct {
	Player      Player `json:"player"`
	Description string `json:"description"`
}

type LineupSide struct {
	Players        []LineupPlayer  `json:"players"`
	MissingPlayers []MissingPlayer `json:"missingPlayers"`
}

type Lineups struct {
	Confirmed bool       `json:"confirmed"`
	Home      LineupSide `json:"home"`
	Away      LineupSide `json:"away"`
}

type Incident struct {
	IncidentType  string  `json:"incidentType"`
	IncidentClass string  `json:"incidentClass"`
	Player        *Player `json:"player"`
	IsHome        bool    `json:"isHome"`
	Rescinded     bool    `json:"rescinded"`
}

type seasonsResponse struct {
	Seasons []Season `json:"seasons"`
}

type teamsResponse struct {
	Teams []Team `json:"teams"`
}

type squadResponse struct {
	Players []struct {
		Player Player `json:"player"`
	} `json:"players"`
}

type eventsPage struct {
	Events      []Event `json:"events"`
	HasNextPage bool    `json:"hasNextPage"`
}

type eventResponse struct {
	Event Event `json:"event"`
}

type incidentsResponse struct {
	Incidents []Incident `json:"incidents"`
}
