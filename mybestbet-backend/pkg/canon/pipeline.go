package canon

import "time"

type Delta struct {
	Source     string    `json:"source"`
	Market     Market    `json:"market"`
	FetchedAt  time.Time `json:"fetchedAt"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

type BoardCell struct {
	MarketKey string   `json:"marketKey"`
	Source    string   `json:"source"`
	Side      Side     `json:"side,omitempty"`
	Label     string   `json:"label,omitempty"`
	Price     Price    `json:"price"`
	Fair      *float64 `json:"fair,omitempty"`
	Derived   bool     `json:"derived,omitempty"`
	DeepLink  string   `json:"deepLink,omitempty"`
}

type BoardUpdate struct {
	EventID string      `json:"eventId"`
	Cells   []BoardCell `json:"cells,omitempty"`
	Signals []Signal    `json:"signals,omitempty"`
	Score   *EventScore `json:"score,omitempty"`
	SentAt  time.Time   `json:"sentAt"`
}

type SignalType string

const (
	SignalArb  SignalType = "arb"
	SignalEV   SignalType = "ev"
	SignalLine SignalType = "line"
	SignalMove SignalType = "move"
)

type SignalStatus string

const (
	SignalOpen   SignalStatus = "open"
	SignalClosed SignalStatus = "closed"
)

type Leg struct {
	Source     string    `json:"source"`
	MarketKey  string    `json:"marketKey,omitempty"`
	Line       *float64  `json:"line,omitempty"`
	Side       Side      `json:"side,omitempty"`
	Label      string    `json:"label,omitempty"`
	Price      Price     `json:"price"`
	Stake      float64   `json:"stake,omitempty"`
	DeepLink   string    `json:"deepLink,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}

type Signal struct {
	ID           string        `json:"id"`
	DedupeKey    string        `json:"dedupeKey"`
	Type         SignalType    `json:"type"`
	Status       SignalStatus  `json:"status"`
	EventID      string        `json:"eventId"`
	LeagueCode   string        `json:"leagueCode"`
	Market       MarketKey     `json:"market"`
	Legs         []Leg         `json:"legs"`
	Margin       float64       `json:"margin,omitempty"`
	FromPrice    *Price        `json:"fromPrice,omitempty"`
	ToPrice      *Price        `json:"toPrice,omitempty"`
	Change       float64       `json:"change,omitempty"`
	EV           float64       `json:"ev,omitempty"`
	FairProb     float64       `json:"fairProb,omitempty"`
	OldestLegAge time.Duration `json:"oldestLegAge"`
	DetectedAt   time.Time     `json:"detectedAt"`
	ClosedAt     *time.Time    `json:"closedAt,omitempty"`
}

type AliasUpdate struct {
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
	Flipped  bool   `json:"flipped,omitempty"`
}

const (
	AliasTeam        = "team"
	AliasPlayer      = "player"
	AliasEvent       = "event"
	AliasPlayerMerge = "player_merge"
)

type Freshness struct {
	Source     string    `json:"source"`
	EventID    string    `json:"eventId"`
	ObservedAt time.Time `json:"observedAt"`
	SourceKeys []string  `json:"sourceKeys,omitempty"`
	Complete   bool      `json:"complete,omitempty"`
}
