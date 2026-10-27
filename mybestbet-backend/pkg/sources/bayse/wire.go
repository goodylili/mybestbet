package bayse

import (
	"time"

	json "github.com/goccy/go-json"
)

type pagination struct {
	Page     int `json:"page"`
	LastPage int `json:"lastPage"`
}

type gameList struct {
	Games      []game     `json:"games"`
	Pagination pagination `json:"pagination"`
}

type game struct {
	Slug      string    `json:"slug"`
	StartDate time.Time `json:"startDate"`
	Status    string    `json:"status"`
	HomeTeam  gameTeam  `json:"homeTeam"`
	AwayTeam  gameTeam  `json:"awayTeam"`
}

type gameTeam struct {
	Name      string `json:"name"`
	ShortCode string `json:"shortCode"`
}

type eventList struct {
	Events     []event    `json:"events"`
	Pagination pagination `json:"pagination"`
}

type event struct {
	ID              string   `json:"id"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Hashtags        []string `json:"hashtags"`
	ClosingDate     string   `json:"closingDate"`
	Engine          string   `json:"engine"`
	Status          string   `json:"status"`
	SportMarketType string   `json:"sportMarketType"`
	Liquidity       float64  `json:"liquidity"`
	Markets         []market `json:"markets"`
}

type market struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	Outcome1ID    string   `json:"outcome1Id"`
	Outcome1Label string   `json:"outcome1Label"`
	Outcome1Price float64  `json:"outcome1Price"`
	Outcome2ID    string   `json:"outcome2Id"`
	Outcome2Label string   `json:"outcome2Label"`
	Outcome2Price float64  `json:"outcome2Price"`
	PropLine      *float64 `json:"propLine"`
	Rules         string   `json:"rules"`
	TotalOrders   int      `json:"totalOrders"`
}

type outcomeBook struct {
	MarketID  string      `json:"marketId"`
	OutcomeID string      `json:"outcomeId"`
	Bids      []bookLevel `json:"bids"`
	Asks      []bookLevel `json:"asks"`
}

type bookLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

type wsSubscribe struct {
	Type      string   `json:"type"`
	Channel   string   `json:"channel"`
	EventID   string   `json:"eventId,omitempty"`
	MarketIDs []string `json:"marketIds,omitempty"`
}

type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type priceUpdate struct {
	Markets []priceUpdateMarket `json:"markets"`
}

type priceUpdateMarket struct {
	ID     string             `json:"id"`
	Prices map[string]float64 `json:"prices"`
}

type orderbookUpdate struct {
	Orderbook outcomeBook `json:"orderbook"`
}
