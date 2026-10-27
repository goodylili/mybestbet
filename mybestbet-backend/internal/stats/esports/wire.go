package esports

type parseResponse struct {
	Parse struct {
		Text struct {
			HTML string `json:"*"`
		} `json:"text"`
	} `json:"parse"`
}

type proMatchWire struct {
	MatchID       int64  `json:"match_id"`
	StartTime     int64  `json:"start_time"`
	RadiantTeamID int64  `json:"radiant_team_id"`
	RadiantName   string `json:"radiant_name"`
	DireTeamID    int64  `json:"dire_team_id"`
	DireName      string `json:"dire_name"`
	SeriesID      int64  `json:"series_id"`
	SeriesType    int    `json:"series_type"`
	LeagueName    string `json:"league_name"`
	RadiantWin    bool   `json:"radiant_win"`
	RadiantScore  int    `json:"radiant_score"`
	DireScore     int    `json:"dire_score"`
}

type liveGameWire struct {
	MatchID        int64  `json:"match_id,string"`
	ActivateTime   int64  `json:"activate_time"`
	DeactivateTime int64  `json:"deactivate_time"`
	LeagueID       int64  `json:"league_id"`
	SeriesID       int64  `json:"series_id"`
	RadiantTeamID  int64  `json:"team_id_radiant"`
	RadiantName    string `json:"team_name_radiant"`
	DireTeamID     int64  `json:"team_id_dire"`
	DireName       string `json:"team_name_dire"`
	RadiantScore   int    `json:"radiant_score"`
	DireScore      int    `json:"dire_score"`
}

type teamWire struct {
	TeamID  int64  `json:"team_id"`
	Name    string `json:"name"`
	Tag     string `json:"tag"`
	LogoURL string `json:"logo_url"`
}

type matchWire struct {
	MatchID     int64    `json:"match_id"`
	StartTime   int64    `json:"start_time"`
	RadiantTeam teamWire `json:"radiant_team"`
	DireTeam    teamWire `json:"dire_team"`
	Players     []struct {
		AccountID   int64  `json:"account_id"`
		IsRadiant   bool   `json:"isRadiant"`
		Name        string `json:"name"`
		Personaname string `json:"personaname"`
		Kills       int    `json:"kills"`
	} `json:"players"`
}

type teamPlayerWire struct {
	AccountID           int64  `json:"account_id"`
	Name                string `json:"name"`
	IsCurrentTeamMember bool   `json:"is_current_team_member"`
}
