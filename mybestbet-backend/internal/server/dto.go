package server

import (
	"time"

	"mybestbet/pkg/canon"
)

type itemList[T any] struct {
	Items []T `json:"items"`
}

func listOf[T any](items []T) itemList[T] {
	return itemList[T]{Items: emptyIfNil(items)}
}

func emptyIfNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

type League struct {
	Code         string      `json:"code"`
	Name         string      `json:"name"`
	Country      string      `json:"country"`
	Sport        canon.Sport `json:"sport"`
	Group        string      `json:"group"`
	Order        int         `json:"order"`
	FixtureCount int         `json:"fixtureCount"`
	LogoURL      string      `json:"logoUrl,omitempty"`
}

type SportSummary struct {
	Sport        canon.Sport        `json:"sport"`
	FixtureCount int                `json:"fixtureCount"`
	Leagues      []League           `json:"leagues"`
	MatchMarkets []canon.MarketCode `json:"matchMarkets"`
	PlayerProps  []canon.MarketCode `json:"playerProps"`
	Markets      []MarketInfo       `json:"markets"`
}

type MarketInfo struct {
	Code           canon.MarketCode  `json:"code"`
	Label          string            `json:"label"`
	Kind           canon.MarketKind  `json:"kind"`
	Shape          canon.MarketShape `json:"shape"`
	Periods        []string          `json:"periods"`
	StatField      string            `json:"statField,omitempty"`
	RegulationOnly bool              `json:"regulationOnly"`
}

type Team struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl,omitempty"`
}

type TeamDetail struct {
	Team
	Sport       canon.Sport `json:"sport"`
	LeagueCodes []string    `json:"leagueCodes"`
}

type EventInfo struct {
	ID         string            `json:"id"`
	Sport      canon.Sport       `json:"sport"`
	LeagueCode string            `json:"leagueCode"`
	Links      map[string]string `json:"links,omitempty"`
	Score      *canon.EventScore `json:"score,omitempty"`
	Home       Team              `json:"home"`
	Away       Team              `json:"away"`
	Kickoff    time.Time         `json:"kickoff"`
	Status     string            `json:"status"`
}

type BestPrice struct {
	Source   string      `json:"source"`
	Price    canon.Price `json:"price"`
	DeepLink string      `json:"deepLink,omitempty"`
}

type BestOutcomes struct {
	Market canon.MarketCode `json:"market"`
	Line   *float64         `json:"line,omitempty"`
	Period string           `json:"period,omitempty"`
	Home   *BestPrice       `json:"home,omitempty"`
	Draw   *BestPrice       `json:"draw,omitempty"`
	Away   *BestPrice       `json:"away,omitempty"`
	Over   *BestPrice       `json:"over,omitempty"`
	Under  *BestPrice       `json:"under,omitempty"`
	Yes    *BestPrice       `json:"yes,omitempty"`
	No     *BestPrice       `json:"no,omitempty"`
}

type EventBest struct {
	EventID string            `json:"eventId"`
	Best    *BestOutcomes     `json:"best,omitempty"`
	Score   *canon.EventScore `json:"score,omitempty"`
}

type CalendarDay struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type EventCalendar struct {
	Days []CalendarDay `json:"days"`
}

type Badges struct {
	Arbs      int     `json:"arbs"`
	EVs       int     `json:"evs"`
	MaxMargin float64 `json:"maxMargin,omitempty"`
	MaxEV     float64 `json:"maxEv,omitempty"`
}

type EventSummary struct {
	EventInfo
	Sources   []string     `json:"sources"`
	PropCount int          `json:"propCount"`
	Best      BestOutcomes `json:"best"`
	Badges    Badges       `json:"badges"`
}

type SelectionRow struct {
	Side       string                     `json:"side"`
	Label      string                     `json:"label"`
	Cells      map[string]canon.BoardCell `json:"cells"`
	Consensus  *canon.BoardCell           `json:"consensus,omitempty"`
	BestSource string                     `json:"bestSource,omitempty"`
}

type MarketBoard struct {
	Key        canon.MarketKey `json:"key"`
	KeyString  string          `json:"keyString"`
	Selections []SelectionRow  `json:"selections"`
}

type MarketPeriod struct {
	Period string        `json:"period"`
	Boards []MarketBoard `json:"boards"`
}

type MarketTab struct {
	Code    canon.MarketCode  `json:"code"`
	Kind    canon.MarketKind  `json:"kind"`
	Shape   canon.MarketShape `json:"shape"`
	Periods []MarketPeriod    `json:"periods"`
}

type EventDetail struct {
	Event   EventInfo   `json:"event"`
	Sources []string    `json:"sources"`
	Markets []MarketTab `json:"markets"`
}

type PropPlayer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	TeamID      string `json:"teamId,omitempty"`
	Position    string `json:"position,omitempty"`
	TeamName    string `json:"teamName,omitempty"`
	TeamLogoURL string `json:"teamLogoUrl,omitempty" db:"-"`
}

type HitRate struct {
	Window  int `json:"window,omitempty"`
	Matches int `json:"matches,omitempty"`
	Hits    int `json:"hits,omitempty"`
}

type PlayerMatchStat struct {
	EventID          string             `json:"eventId"`
	TeamID           string             `json:"teamId,omitempty"`
	OpponentTeamID   string             `json:"opponentTeamId,omitempty"`
	OpponentTeamName string             `json:"opponentTeamName,omitempty"`
	OpponentLogoURL  string             `json:"opponentLogoUrl,omitempty"`
	IsHome           bool               `json:"isHome"`
	PlayedAt         time.Time          `json:"playedAt"`
	Minutes          int                `json:"minutes"`
	Started          bool               `json:"started"`
	Goals            int                `json:"goals"`
	Assists          int                `json:"assists"`
	Shots            int                `json:"shots"`
	ShotsOnTarget    int                `json:"shotsOnTarget"`
	Fouls            int                `json:"fouls"`
	Tackles          int                `json:"tackles"`
	Yellow           int                `json:"yellow"`
	Red              int                `json:"red"`
	Rating           *float64           `json:"rating,omitempty"`
	Stats            map[string]float64 `json:"stats"`
}

type MinutesPoint struct {
	EventID  string    `json:"eventId"`
	PlayedAt time.Time `json:"playedAt"`
	Minutes  int       `json:"minutes"`
	Started  bool      `json:"started"`
}

type RatingBucket struct {
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Count int     `json:"count"`
}

type RatingDistribution struct {
	Ratings []float64      `json:"ratings"`
	Mean    float64        `json:"mean"`
	Min     float64        `json:"min"`
	Max     float64        `json:"max"`
	Buckets []RatingBucket `json:"buckets"`
}

type PropResearch struct {
	Windows           []HitRate         `json:"windows"`
	Home              HitRate           `json:"home"`
	Away              HitRate           `json:"away"`
	VsOpponent        HitRate           `json:"vsOpponent"`
	VsOpponentMatches []PlayerMatchStat `json:"vsOpponentMatches,omitempty"`
}

type PropCell struct {
	Raw       float64         `json:"raw"`
	Form      canon.PriceForm `json:"form,omitempty"`
	Prob      *float64        `json:"prob,omitempty"`
	Bid       *float64        `json:"bid,omitempty"`
	Ask       *float64        `json:"ask,omitempty"`
	Liquidity *float64        `json:"liquidity,omitempty"`
	Suspended bool            `json:"suspended,omitempty"`
	Fair      *float64        `json:"fair,omitempty"`
	Derived   bool            `json:"derived,omitempty"`
	DeepLink  string          `json:"deepLink,omitempty"`
}

type PropSelection struct {
	Side       string              `json:"side"`
	Label      string              `json:"label"`
	BestSource string              `json:"bestSource,omitempty"`
	Consensus  *PropCell           `json:"consensus,omitempty"`
	Cells      map[string]PropCell `json:"cells"`
}

type PropRow struct {
	KeyString  string          `json:"keyString"`
	Selections []PropSelection `json:"selections"`
	Research   *PropResearch   `json:"research,omitempty"`
}

type PropPlayerInfo struct {
	PropPlayer
	MinutesTrend []MinutesPoint      `json:"minutesTrend"`
	LineupStatus string              `json:"lineupStatus"`
	BenchWarning bool                `json:"benchWarning"`
	Ratings      *RatingDistribution `json:"ratings,omitempty"`
}

type PropBoard struct {
	Event   EventInfo        `json:"event"`
	Sources []string         `json:"sources"`
	Players []PropPlayerInfo `json:"players"`
	Rows    []PropRow        `json:"rows"`
}

type HistoryPoint struct {
	Source     string          `json:"source"`
	Side       string          `json:"side"`
	Label      string          `json:"label"`
	Raw        float64         `json:"raw"`
	Form       canon.PriceForm `json:"form"`
	Prob       float64         `json:"prob"`
	Suspended  bool            `json:"suspended"`
	ObservedAt time.Time       `json:"observedAt"`
}

type MarketHistory struct {
	MarketKey string         `json:"marketKey"`
	Points    []HistoryPoint `json:"points"`
}

type SignalView struct {
	canon.Signal
	Event  EventInfo   `json:"event"`
	Player *PropPlayer `json:"player,omitempty"`
}

type PlayerStats struct {
	Player PropPlayer        `json:"player"`
	Items  []PlayerMatchStat `json:"items"`
}

type SourceHealth struct {
	Name                string         `json:"name"`
	Enabled             bool           `json:"enabled"`
	Degraded            bool           `json:"degraded"`
	LastSuccess         *time.Time     `json:"lastSuccess,omitempty"`
	LastError           string         `json:"lastError"`
	ConsecutiveFailures int64          `json:"consecutiveFailures"`
	Requests            int64          `json:"requests"`
	Errors              int64          `json:"errors"`
	ErrorRate           float64        `json:"errorRate"`
	LastFetchMs         int64          `json:"lastFetchMs"`
	FreshnessMs         int64          `json:"freshnessMs"`
	LogoURL             string         `json:"logoUrl,omitempty"`
	Type                string         `json:"type"`
	Feed                string         `json:"feed"`
	FeedGroup           string         `json:"feedGroup"`
	Note                string         `json:"note,omitempty"`
	Countries           []string       `json:"countries"`
	BlockedCountries    []string       `json:"blockedCountries"`
	Coverage            SourceCoverage `json:"coverage"`
}

type SourceCoverage struct {
	Leagues []string           `json:"leagues"`
	Markets []canon.MarketCode `json:"markets"`
}

type Meta struct {
	Version        string   `json:"version"`
	TelegramBotURL string   `json:"telegramBotUrl,omitempty"`
	Auth           MetaAuth `json:"auth"`
	Country        string   `json:"country"`
}

type MetaAuth struct {
	Google bool `json:"google"`
}

type SearchKind string

const (
	SearchTeam   SearchKind = "team"
	SearchPlayer SearchKind = "player"
	SearchLeague SearchKind = "league"
	SearchEvent  SearchKind = "event"
)

type SearchItem struct {
	Kind        SearchKind  `json:"kind"`
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Sport       canon.Sport `json:"sport"`
	LeagueCode  string      `json:"leagueCode,omitempty"`
	LogoURL     string      `json:"logoUrl,omitempty"`
	AwayLogoURL string      `json:"awayLogoUrl,omitempty"`
	Kickoff     *time.Time  `json:"kickoff,omitempty"`
}
