package kalshi

import (
	"bytes"
	"cmp"
	"fmt"
	"strconv"
	"time"

	json "github.com/goccy/go-json"
)

type eventList struct {
	Cursor     string      `json:"cursor"`
	Events     []event     `json:"events"`
	Milestones []milestone `json:"milestones"`
}

type event struct {
	EventTicker     string          `json:"event_ticker"`
	Title           string          `json:"title"`
	ProductMetadata productMetadata `json:"product_metadata"`
	Markets         []market        `json:"markets"`
}

type productMetadata struct {
	Competition string `json:"competition"`
}

type milestone struct {
	StartDate           time.Time        `json:"start_date"`
	Details             milestoneDetails `json:"details"`
	PrimaryEventTickers []string         `json:"primary_event_tickers"`
}

type milestoneDetails struct {
	MainGameEventTicker string `json:"main_game_event_ticker"`
}

type market struct {
	Ticker       string       `json:"ticker"`
	Status       string       `json:"status"`
	FloorStrike  *float64     `json:"floor_strike"`
	StrikeType   string       `json:"strike_type"`
	CustomStrike customStrike `json:"custom_strike"`
	YesSubTitle  string       `json:"yes_sub_title"`
	YesBid       decimal      `json:"yes_bid_dollars"`
	YesAsk       decimal      `json:"yes_ask_dollars"`
	YesBidSize   decimal      `json:"yes_bid_size_fp"`
	YesAskSize   decimal      `json:"yes_ask_size_fp"`
}

func (m market) quote() quote {
	return quote{yesBid: float64(m.YesBid), yesAsk: float64(m.YesAsk), yesBidSize: float64(m.YesBidSize), yesAskSize: float64(m.YesAskSize)}
}

type customStrike struct {
	HomeScore         string `json:"home_score"`
	AwayScore         string `json:"away_score"`
	SoccerTeam        string `json:"soccer_team"`
	BasketballTeam    string `json:"basketball_team"`
	FootballTeam      string `json:"football_team"`
	TennisCompetitor  string `json:"tennis_competitor"`
	EsportsCompetitor string `json:"esports_competitor"`
	BasketballPlayer  string `json:"basketball_player"`
	FootballPlayer    string `json:"football_player"`
}

func (s customStrike) team() string {
	return cmp.Or(s.SoccerTeam, s.BasketballTeam, s.FootballTeam, s.TennisCompetitor, s.EsportsCompetitor)
}

func (s customStrike) player() string { return cmp.Or(s.BasketballPlayer, s.FootballPlayer) }

type decimal float64

func (d *decimal) UnmarshalJSON(data []byte) error {
	text := string(bytes.Trim(data, `"`))
	if text == "" || text == "null" {
		*d = 0
		return nil
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("decimal %q: %w", text, err)
	}
	*d = decimal(v)
	return nil
}

type wsCommand struct {
	ID     int      `json:"id"`
	Cmd    string   `json:"cmd"`
	Params wsParams `json:"params"`
}

type wsParams struct {
	Channels      []string `json:"channels"`
	MarketTickers []string `json:"market_tickers"`
}

type wsEnvelope struct {
	Type string          `json:"type"`
	Msg  json.RawMessage `json:"msg"`
}

type wsError struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}

type tickerMsg struct {
	MarketTicker string   `json:"market_ticker"`
	YesBid       *decimal `json:"yes_bid_dollars"`
	YesAsk       *decimal `json:"yes_ask_dollars"`
	YesBidSize   *decimal `json:"yes_bid_size_fp"`
	YesAskSize   *decimal `json:"yes_ask_size_fp"`
}
