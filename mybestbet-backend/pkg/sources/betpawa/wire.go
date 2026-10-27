package betpawa

import "time"

type listQuery struct {
	Queries []eventQuery `json:"queries"`
}

type eventQuery struct {
	Query eventFilter `json:"query"`
	View  queryView   `json:"view"`
	Skip  int         `json:"skip"`
	Take  int         `json:"take"`
}

type eventFilter struct {
	EventType  string       `json:"eventType"`
	Categories []categoryID `json:"categories"`
	Zones      queryZones   `json:"zones"`
	HasOdds    bool         `json:"hasOdds"`
}

type queryZones struct {
	Competitions []string `json:"competitions"`
}

type queryView struct {
	MarketTypes []string `json:"marketTypes"`
}

type listResponse struct {
	Responses []listPage `json:"responses"`
}

type listPage struct {
	Responses []event `json:"responses"`
}

type event struct {
	ID           string        `json:"id"`
	StartTime    time.Time     `json:"startTime"`
	Participants []participant `json:"participants"`
	Category     category      `json:"category"`
	Competition  competition   `json:"competition"`
	Markets      []market      `json:"markets"`
}

type category struct {
	ID categoryID `json:"id"`
}

type participant struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
}

type competition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type categoryTreeResponse struct {
	WithRegions []categoryTree `json:"withRegions"`
}

type categoryTree struct {
	Regions []regionTree `json:"regions"`
}

type regionTree struct {
	Region       named             `json:"region"`
	Competitions []competitionNode `json:"competitions"`
}

type named struct {
	Name string `json:"name"`
}

type competitionNode struct {
	Competition competition `json:"competition"`
	EventCounts eventCounts `json:"eventCounts"`
}

type eventCounts struct {
	Upcoming string `json:"upcoming"`
}

type market struct {
	MarketType marketType `json:"marketType"`
	Row        []row      `json:"row"`
}

type marketType struct {
	ID string `json:"id"`
}

type row struct {
	Specifier specifier `json:"specifier"`
	Prices    []price   `json:"prices"`
}

type specifier struct {
	Total  string `json:"total"`
	Hcp    string `json:"hcp"`
	Number string `json:"number"`
}

type price struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	TypeID    string  `json:"typeId"`
	Odds      float64 `json:"odds"`
	Suspended bool    `json:"suspended"`
	Team      int     `json:"team"`
}
