package predictfun

import (
	"time"

	json "github.com/goccy/go-json"
)

type categoryList struct {
	Cursor string     `json:"cursor"`
	Data   []category `json:"data"`
}

type category struct {
	Slug           string          `json:"slug"`
	MarketVariant  string          `json:"marketVariant"`
	Title          string          `json:"title"`
	StartsAt       time.Time       `json:"startsAt"`
	EndsAt         *time.Time      `json:"endsAt"`
	VariantDetails categoryDetails `json:"variantDetails"`
	Teams          []team          `json:"teams"`
	Markets        []market        `json:"markets"`
}

type categoryDetails struct {
	Sports sportsCategory `json:"sports"`
}

type sportsCategory struct {
	Teams []team `json:"teams"`
}

type team struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	IsHome       bool   `json:"isHome"`
}

type market struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Question       string        `json:"question"`
	TradingStatus  string        `json:"tradingStatus"`
	MarketType     string        `json:"marketType"`
	Outcomes       []outcome     `json:"outcomes"`
	VariantDetails marketDetails `json:"variantDetails"`
}

type outcome struct {
	Name     string     `json:"name"`
	Team     *team      `json:"team"`
	IndexSet int        `json:"indexSet"`
	BestBid  *bookLevel `json:"bestBid"`
	BestAsk  *bookLevel `json:"bestAsk"`
}

type bookLevel struct {
	Price float64 `json:"price"`
	Size  float64 `json:"size"`
}

type marketDetails struct {
	Sports sportsMarket `json:"sports"`
}

type sportsMarket struct {
	Line       *float64 `json:"line"`
	Team       *team    `json:"team"`
	GameNumber *int     `json:"gameNumber"`
}

type orderbookEnvelope struct {
	Data orderbook `json:"data"`
}

type orderbook struct {
	MarketID int64        `json:"marketId"`
	Asks     [][2]float64 `json:"asks"`
	Bids     [][2]float64 `json:"bids"`
}

type subscribeRequest struct {
	Method    string   `json:"method"`
	RequestID int      `json:"requestId"`
	Params    []string `json:"params"`
}

type heartbeatReply struct {
	Method string          `json:"method"`
	Data   json.RawMessage `json:"data"`
}

type wsMessage struct {
	Type      string          `json:"type"`
	Topic     string          `json:"topic"`
	Data      json.RawMessage `json:"data"`
	RequestID int             `json:"requestId"`
	Success   bool            `json:"success"`
	Error     *wsError        `json:"error"`
}

type wsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
