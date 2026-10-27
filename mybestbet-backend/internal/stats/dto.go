package stats

import (
	"time"

	"mybestbet/internal/stats/statkey"
	"mybestbet/pkg/canon"
)

type MatchStat struct {
	PlayerID       string
	EventID        string
	Sport          canon.Sport
	TeamID         string
	OpponentTeamID string
	IsHome         bool
	PlayedAt       time.Time
	Minutes        int
	Started        bool
	Goals          int
	Assists        int
	Shots          int
	ShotsOnTarget  int
	Fouls          int
	Tackles        int
	Yellow         int
	Red            int
	Rating         *float64
	Stats          statkey.Values
	ValueSources   map[string]string
}

type HitRate struct {
	Window  int
	Matches int
	Hits    int
	Rate    float64
}

type HitRates struct {
	PlayerID string
	Code     canon.MarketCode
	StatCode canon.MarketCode
	Line     float64
	Windows  []HitRate
	Home     HitRate
	Away     HitRate
}

type MinutesPoint struct {
	EventID  string
	PlayedAt time.Time
	Minutes  int
	Started  bool
}

type LineupStatus string

const (
	LineupUnknown LineupStatus = ""
	LineupStarter LineupStatus = "starter"
	LineupBench   LineupStatus = "bench"
	LineupOut     LineupStatus = "out"
)

type RatingBucket struct {
	From  float64
	To    float64
	Count int
}

type RatingDistribution struct {
	Ratings []float64
	Mean    float64
	Min     float64
	Max     float64
	Buckets []RatingBucket
}

type SeasonStat struct {
	LeagueCode         string
	Season             string
	Source             string
	Team               string
	Matches            int
	Starts             int
	Minutes            int
	Goals              int
	Assists            int
	Shots              int
	ShotsOnTarget      int
	Fouls              int
	Tackles            int
	Yellow             int
	Red                int
	GoalsPer90         float64
	AssistsPer90       float64
	ShotsPer90         float64
	ShotsOnTargetPer90 float64
	UpdatedAt          time.Time
}
