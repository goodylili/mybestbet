package bo3

import "time"

type teamWire struct {
	ID       int64  `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

type matchWire struct {
	ID        int64     `json:"id"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	Tier      string    `json:"tier"`
	StartDate time.Time `json:"start_date"`
	Team1     *teamWire `json:"team1"`
	Team2     *teamWire `json:"team2"`
	Team1Maps int       `json:"team1_score"`
	Team2Maps int       `json:"team2_score"`
}

type matchPageWire struct {
	Total struct {
		Count int `json:"count"`
	} `json:"total"`
	Results []matchWire `json:"results"`
}

type playerWire struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	IsCoach  bool   `json:"is_coach"`
}

type teamRosterWire struct {
	Players []playerWire `json:"players"`
}

type csPlayerStatWire struct {
	Kills    int `json:"kills"`
	TeamClan struct {
		TeamID int64 `json:"team_id"`
	} `json:"team_clan"`
	SteamProfile struct {
		ID                int64       `json:"id"`
		Nickname          string      `json:"nickname"`
		Player            *playerWire `json:"player"`
		GameSteamProfiles []struct {
			GameID int64 `json:"game_id"`
		} `json:"game_steam_profiles"`
	} `json:"steam_profile"`
}

type gamePlayerWire struct {
	VlrPlayerID int64 `json:"vlr_player_id"`
	PlayerID    int64 `json:"player_id"`
	TeamID      int64 `json:"team_id"`
	Kills       int   `json:"kills"`
}

func (g gamePlayerWire) lineupKey() int64 {
	if g.VlrPlayerID != 0 {
		return g.VlrPlayerID
	}
	return g.PlayerID
}

type gameWire struct {
	Status     string           `json:"status"`
	VlrPlayers []gamePlayerWire `json:"vlr_game_players"`
	LolPlayers []gamePlayerWire `json:"lol_game_players"`
}

type lineupWire struct {
	VlrPlayerID int64 `json:"vlr_player_id"`
	PlayerID    int64 `json:"player_id"`
	VlrPlayer   *struct {
		Nickname string      `json:"nickname"`
		Player   *playerWire `json:"player"`
	} `json:"vlr_player"`
	Player *playerWire `json:"player"`
}

func (l lineupWire) lineupKey() int64 {
	if l.VlrPlayerID != 0 {
		return l.VlrPlayerID
	}
	return l.PlayerID
}
