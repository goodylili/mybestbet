package booking

import "time"

type Mode string

const (
	ModeSplit  Mode = "split"
	ModeSingle Mode = "single"
)

type Status string

const (
	StatusOK          Status = "ok"
	StatusPartial     Status = "partial"
	StatusFailed      Status = "failed"
	StatusBlocked     Status = "blocked"
	StatusUnsupported Status = "unsupported"
)

type Leg struct {
	EventID   string `json:"eventId"`
	MarketKey string `json:"marketKey"`
	Source    string `json:"source"`
	Side      string `json:"side"`
	Label     string `json:"label"`
}

type Request struct {
	Legs  []Leg   `json:"legs"`
	Mode  Mode    `json:"mode"`
	Book  string  `json:"book"`
	Stake float64 `json:"stake"`
}

type BookedLeg struct {
	Index  int     `json:"index"`
	Price  float64 `json:"price,omitempty"`
	OK     bool    `json:"ok"`
	Reason string  `json:"reason,omitempty"`
}

type BookResult struct {
	Book      string      `json:"book"`
	Status    Status      `json:"status"`
	Code      string      `json:"code,omitempty"`
	WebURL    string      `json:"webUrl,omitempty"`
	AppURL    string      `json:"appUrl,omitempty"`
	ExpiresAt *time.Time  `json:"expiresAt,omitempty"`
	Odds      float64     `json:"odds,omitempty"`
	Return    float64     `json:"return,omitempty"`
	Legs      []BookedLeg `json:"legs"`
	Message   string      `json:"message,omitempty"`
	Cached    bool        `json:"cached"`
}

type ManualLeg struct {
	Index    int     `json:"index"`
	Source   string  `json:"source"`
	DeepLink string  `json:"deepLink,omitempty"`
	Price    float64 `json:"price,omitempty"`
	Reason   string  `json:"reason"`
}

type QuoteLeg struct {
	Index  int     `json:"index"`
	Source string  `json:"source"`
	Price  float64 `json:"price"`
}

type SplitQuote struct {
	Odds    float64    `json:"odds"`
	Return  float64    `json:"return"`
	Covered int        `json:"covered"`
	Legs    []QuoteLeg `json:"legs"`
}

type BookQuote struct {
	Book     string     `json:"book"`
	Bookable bool       `json:"bookable"`
	Covered  int        `json:"covered"`
	Total    int        `json:"total"`
	Odds     float64    `json:"odds"`
	Return   float64    `json:"return"`
	Legs     []QuoteLeg `json:"legs"`
	Missing  []int      `json:"missing"`
}

type Comparison struct {
	Stake     float64     `json:"stake"`
	TotalLegs int         `json:"totalLegs"`
	Picked    SplitQuote  `json:"picked"`
	BestSplit SplitQuote  `json:"bestSplit"`
	Books     []BookQuote `json:"books"`
}

type Response struct {
	Mode       Mode         `json:"mode"`
	Book       string       `json:"book,omitempty"`
	Bookings   []BookResult `json:"bookings"`
	Manual     []ManualLeg  `json:"manual"`
	Comparison Comparison   `json:"comparison"`
}
