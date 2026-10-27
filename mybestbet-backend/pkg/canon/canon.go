package canon

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type PriceForm string

const (
	FormDecimal  PriceForm = "decimal"
	FormShare    PriceForm = "share"
	FormCents    PriceForm = "cents"
	FormNaira    PriceForm = "naira"
	FormLineOnly PriceForm = "line_only"
)

type Price struct {
	Raw        float64   `json:"raw"`
	Form       PriceForm `json:"form"`
	Prob       float64   `json:"prob"`
	Bid        *float64  `json:"bid,omitempty"`
	Ask        *float64  `json:"ask,omitempty"`
	Liquidity  *float64  `json:"liquidity,omitempty"`
	Suspended  bool      `json:"suspended,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}

func NewPrice(raw float64, form PriceForm, observedAt time.Time) Price {
	return Price{Raw: raw, Form: form, Prob: ImpliedProb(raw, form), ObservedAt: observedAt}
}

func NewBookPrice(bid, ask, liquidity *float64, form PriceForm, observedAt time.Time) Price {
	p := Price{Form: form, Bid: bid, Ask: ask, Liquidity: liquidity, ObservedAt: observedAt}
	if ask != nil {
		p.Raw = *ask
		p.Prob = ImpliedProb(*ask, form)
	}
	return p
}

func ImpliedProb(raw float64, form PriceForm) float64 {
	switch form {
	case FormDecimal:
		if raw <= 1 {
			return 0
		}
		return 1 / raw
	case FormShare:
		return raw
	case FormCents, FormNaira:
		return raw / 100
	case FormLineOnly:
		return 0
	}
	return 0
}

type Side string

const (
	SideHome  Side = "home"
	SideDraw  Side = "draw"
	SideAway  Side = "away"
	SideOver  Side = "over"
	SideUnder Side = "under"
	SideYes   Side = "yes"
	SideNo    Side = "no"
)

func (s Side) Flipped() Side {
	switch s {
	case SideHome:
		return SideAway
	case SideAway:
		return SideHome
	case SideDraw, SideOver, SideUnder, SideYes, SideNo:
		return s
	}
	return s
}

const (
	PeriodFullTime   = ""
	PeriodFirstHalf  = "1H"
	PeriodSecondHalf = "2H"
	PeriodQuarter1   = "Q1"
	PeriodQuarter2   = "Q2"
	PeriodQuarter3   = "Q3"
	PeriodQuarter4   = "Q4"
	PeriodFirstSet   = "S1"
	PeriodFirstMap   = "M1"
	PeriodMaps1To2   = "M1-2"
)

type MarketKey struct {
	EventID  string     `json:"eventId"`
	Code     MarketCode `json:"code"`
	Line     *float64   `json:"line,omitempty"`
	PlayerID string     `json:"playerId,omitempty"`
	Period   string     `json:"period,omitempty"`
	TeamID   string     `json:"teamId,omitempty"`
}

const marketKeySeparator = "|"

func (k MarketKey) String() string {
	line := ""
	if k.Line != nil {
		line = strconv.FormatFloat(*k.Line, 'f', -1, 64)
	}
	parts := []string{k.EventID, string(k.Code), line, k.PlayerID, k.Period}
	if k.TeamID != "" {
		parts = append(parts, k.TeamID)
	}
	return strings.Join(parts, marketKeySeparator)
}

func ParseMarketKey(s string) (MarketKey, error) {
	parts := strings.Split(s, marketKeySeparator)
	if len(parts) != 5 && len(parts) != 6 {
		return MarketKey{}, fmt.Errorf("market key %q: want 5 or 6 parts, got %d", s, len(parts))
	}
	k := MarketKey{EventID: parts[0], Code: MarketCode(parts[1]), PlayerID: parts[3], Period: parts[4]}
	if len(parts) == 6 {
		k.TeamID = parts[5]
	}
	if parts[2] != "" {
		line, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return MarketKey{}, fmt.Errorf("market key %q line: %w", s, err)
		}
		k.Line = &line
	}
	return k, nil
}

func Line(v float64) *float64 { return new(v) }

type Selection struct {
	Side     Side   `json:"side,omitempty"`
	Label    string `json:"label,omitempty"`
	Price    Price  `json:"price"`
	DeepLink string `json:"deepLink,omitempty"`
}

type Market struct {
	Key        MarketKey   `json:"key"`
	SourceKey  string      `json:"sourceKey"`
	Selections []Selection `json:"selections"`
	Rules      SettleRule  `json:"rules"`
}

type SettleRule struct {
	RegulationOnly bool   `json:"regulationOnly"`
	VoidIfNoStart  bool   `json:"voidIfNoStart"`
	Resolver       string `json:"resolver,omitempty"`
}

var StandardRule = SettleRule{RegulationOnly: true}

func (r SettleRule) SettlesLike(other SettleRule) bool {
	return r.RegulationOnly == other.RegulationOnly && r.VoidIfNoStart == other.VoidIfNoStart
}

type SourceEvent struct {
	SourceID       string    `json:"sourceId"`
	LeagueHint     string    `json:"leagueHint"`
	SourceLeagueID string    `json:"sourceLeagueId,omitempty"`
	Home           string    `json:"home"`
	Away           string    `json:"away"`
	Kickoff        time.Time `json:"kickoff"`
}

type SourcePlayer struct {
	SourceID string `json:"sourceId"`
	Name     string `json:"name"`
	Team     string `json:"team"`
}

type RawMarket struct {
	EventSourceID  string        `json:"eventSourceId"`
	Code           MarketCode    `json:"code"`
	Line           *float64      `json:"line,omitempty"`
	Player         *SourcePlayer `json:"player,omitempty"`
	Team           string        `json:"team,omitempty"`
	Period         string        `json:"period,omitempty"`
	Selections     []Selection   `json:"selections"`
	Rules          SettleRule    `json:"rules"`
	SourceMarketID string        `json:"sourceMarketId"`
}

type Snapshot struct {
	Source    string        `json:"source"`
	FetchedAt time.Time     `json:"fetchedAt"`
	Events    []SourceEvent `json:"events"`
	Markets   []RawMarket   `json:"markets"`
	Errors    []error       `json:"-"`
}

func (m RawMarket) Clone() RawMarket {
	m.Selections = slices.Clone(m.Selections)
	return m
}
