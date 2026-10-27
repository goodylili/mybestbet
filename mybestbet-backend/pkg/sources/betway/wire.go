package betway

type regionsResponse struct {
	Regions []region `json:"regions"`
}

type region struct {
	RegionID string         `json:"regionId"`
	Leagues  []regionLeague `json:"leagues"`
}

type regionLeague struct {
	LeagueID string `json:"leagueId"`
}

type catalogResponse struct {
	Events      []event `json:"events"`
	IsFinalPage bool    `json:"isFinalPage"`
}

type event struct {
	EventID            int64  `json:"eventId"`
	HomeTeam           string `json:"homeTeam"`
	AwayTeam           string `json:"awayTeam"`
	ExpectedStartEpoch int64  `json:"expectedStartEpoch"`
	RegionID           string `json:"regionId"`
	LeagueID           string `json:"leagueId"`
	IsLive             bool   `json:"isLive"`
}

type eventMarketsResponse struct {
	MarketsInGroup []market  `json:"marketsInGroup"`
	Outcomes       []outcome `json:"outcomes"`
	Prices         []price   `json:"prices"`
}

type market struct {
	MarketID    string `json:"marketId"`
	IsSuspended bool   `json:"isSuspended"`
	DefaultLine string `json:"defaultLine"`
	DisplayName string `json:"displayName"`
}

type outcome struct {
	OutcomeID        string  `json:"outcomeId"`
	MarketID         string  `json:"marketId"`
	OriginalMarketID string  `json:"originalMarketId"`
	Name             string  `json:"name"`
	Handicap         float64 `json:"handicap"`
	IsTradingActive  bool    `json:"isTradingActive"`
}

type price struct {
	OutcomeID    string  `json:"outcomeId"`
	PriceDecimal float64 `json:"priceDecimal"`
}
