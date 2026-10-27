package polymarket

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	json "github.com/goccy/go-json"
)

type gammaSport struct {
	Sport        string `json:"sport"`
	PrimaryTagID int64  `json:"primaryTagId"`
	Series       string `json:"series"`
}

type gammaTeam struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	Alias        string `json:"alias"`
	Ordering     string `json:"ordering"`
}

type gammaEvent struct {
	Slug      string        `json:"slug"`
	Title     string        `json:"title"`
	StartTime time.Time     `json:"startTime"`
	Teams     []gammaTeam   `json:"teams"`
	Markets   []gammaMarket `json:"markets"`
	Metadata  gammaMetadata `json:"eventMetadata"`
}

type gammaMetadata struct {
	League string `json:"league"`
}

type gammaMarket struct {
	Slug             string     `json:"slug"`
	Question         string     `json:"question"`
	Outcomes         stringList `json:"outcomes"`
	ClobTokenIDs     stringList `json:"clobTokenIds"`
	SportsMarketType string     `json:"sportsMarketType"`
	Line             *float64   `json:"line"`
	Active           bool       `json:"active"`
	Closed           bool       `json:"closed"`
}

func (m gammaMarket) isTradable() bool { return m.Active && !m.Closed && len(m.ClobTokenIDs) == 2 }

type stringList []string

func (l *stringList) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*l = nil
		return nil
	}
	if len(data) > 0 && data[0] == '[' {
		return json.Unmarshal(data, (*[]string)(l))
	}
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return fmt.Errorf("string list: %w", err)
	}
	if encoded == "" {
		*l = nil
		return nil
	}
	return json.Unmarshal([]byte(encoded), (*[]string)(l))
}

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

type bookLevel struct {
	Price decimal `json:"price"`
	Size  decimal `json:"size"`
}

type clobBook struct {
	AssetID string      `json:"asset_id"`
	Bids    []bookLevel `json:"bids"`
	Asks    []bookLevel `json:"asks"`
}

type bookRequest struct {
	TokenID string `json:"token_id"`
}

type wsSubscription struct {
	AssetIDs []string `json:"assets_ids"`
	Type     string   `json:"type"`
}

type wsMessage struct {
	EventType    string          `json:"event_type"`
	AssetID      string          `json:"asset_id"`
	Bids         []bookLevel     `json:"bids"`
	Asks         []bookLevel     `json:"asks"`
	PriceChanges []wsPriceChange `json:"price_changes"`
}

type wsPriceChange struct {
	AssetID string  `json:"asset_id"`
	Price   decimal `json:"price"`
	Size    decimal `json:"size"`
	Side    string  `json:"side"`
}
