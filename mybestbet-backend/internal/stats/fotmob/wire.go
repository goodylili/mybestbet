package fotmob

import (
	"strings"
	"time"
)

type leagueResponse struct {
	Fixtures struct {
		AllMatches []leagueMatch `json:"allMatches"`
	} `json:"fixtures"`
}

type leagueMatch struct {
	ID     string   `json:"id"`
	Home   teamName `json:"home"`
	Away   teamName `json:"away"`
	Status struct {
		UTCTime   time.Time `json:"utcTime"`
		Finished  bool      `json:"finished"`
		Cancelled bool      `json:"cancelled"`
	} `json:"status"`
}

type teamName struct {
	ID   flexibleID `json:"id"`
	Name string     `json:"name"`
}

type flexibleID string

func (id *flexibleID) UnmarshalJSON(raw []byte) error {
	*id = flexibleID(strings.Trim(string(raw), `"`))
	return nil
}

type teamResponse struct {
	Squad struct {
		Squad []squadGroup `json:"squad"`
	} `json:"squad"`
	Fixtures struct {
		AllFixtures struct {
			Fixtures []teamFixture `json:"fixtures"`
		} `json:"allFixtures"`
	} `json:"fixtures"`
}

type teamFixture struct {
	ID         flexibleID `json:"id"`
	Home       teamName   `json:"home"`
	Away       teamName   `json:"away"`
	Tournament struct {
		LeagueID flexibleID `json:"leagueId"`
	} `json:"tournament"`
	Status struct {
		UTCTime   time.Time `json:"utcTime"`
		Finished  bool      `json:"finished"`
		Cancelled bool      `json:"cancelled"`
	} `json:"status"`
}

type squadGroup struct {
	Title   string        `json:"title"`
	Members []squadMember `json:"members"`
}

type squadMember struct {
	ID       flexibleID `json:"id"`
	Name     string     `json:"name"`
	ClubID   flexibleID `json:"ccode"`
	ClubName string     `json:"cname"`
}

type matchResponse struct {
	General struct {
		MatchID  string `json:"matchId"`
		HomeTeam struct {
			ID int64 `json:"id"`
		} `json:"homeTeam"`
		Finished bool `json:"finished"`
	} `json:"general"`
	Content struct {
		Lineup      lineup                 `json:"lineup"`
		PlayerStats map[string]playerStats `json:"playerStats"`
		MatchFacts  struct {
			Events struct {
				Events []matchEvent `json:"events"`
			} `json:"events"`
		} `json:"matchFacts"`
	} `json:"content"`
}

type matchEvent struct {
	Type   string `json:"type"`
	Card   string `json:"card"`
	Player struct {
		ID int64 `json:"id"`
	} `json:"player"`
}

type lineup struct {
	LineupType string     `json:"lineupType"`
	HomeTeam   lineupTeam `json:"homeTeam"`
	AwayTeam   lineupTeam `json:"awayTeam"`
}

type lineupTeam struct {
	ID          int64          `json:"id"`
	Starters    []lineupPlayer `json:"starters"`
	Subs        []lineupPlayer `json:"subs"`
	Unavailable []lineupPlayer `json:"unavailable"`
}

type lineupPlayer struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type playerStats struct {
	ID     int64       `json:"id"`
	Name   string      `json:"name"`
	TeamID int64       `json:"teamId"`
	Stats  []statGroup `json:"stats"`
}

type statGroup struct {
	Key   string               `json:"key"`
	Stats map[string]statEntry `json:"stats"`
}

type statEntry struct {
	Key  string `json:"key"`
	Stat struct {
		Value *float64 `json:"value"`
	} `json:"stat"`
}

type dayResponse struct {
	Leagues []struct {
		Matches []dayMatch `json:"matches"`
	} `json:"leagues"`
}

type dayMatch struct {
	ID     flexibleID `json:"id"`
	Home   teamScore  `json:"home"`
	Away   teamScore  `json:"away"`
	Status struct {
		Started   bool          `json:"started"`
		Finished  bool          `json:"finished"`
		Cancelled bool          `json:"cancelled"`
		LiveTime  *liveTimeWire `json:"liveTime"`
		Reason    struct {
			Short string `json:"short"`
		} `json:"reason"`
	} `json:"status"`
}

type teamScore struct {
	Score int `json:"score"`
}

type liveTimeWire struct {
	Short string `json:"short"`
}
