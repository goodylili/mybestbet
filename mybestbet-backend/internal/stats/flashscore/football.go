package flashscore

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"mybestbet/pkg/canon"
)

const (
	footballDayFeedPath = "/x/feed/f_1_%d_3_en_1"
	keyHomeTeam         = "AE"
	keyAwayTeam         = "AF"
	keyHomeGoals        = "AG"
	keyAwayGoals        = "AH"
	keyPeriodStartUnix  = "AO"
	stageLive           = "2"
	stageFinished       = "3"
	halfTimeClock       = "HT"
	penaltiesDetail     = "Pen"
)

type footballPeriod struct {
	label         string
	clock         string
	detail        string
	minutesBefore int
	length        int
}

var (
	footballLivePeriods = map[string]footballPeriod{
		"12": {label: canon.PeriodFirstHalf, length: 45},
		"13": {label: canon.PeriodSecondHalf, minutesBefore: 45, length: 45},
		"38": {label: canon.PeriodHalfTime, clock: halfTimeClock},
		"6":  {label: canon.PeriodExtraTime},
		"7":  {label: canon.PeriodExtraTime, detail: penaltiesDetail},
	}
	footballFinishedDetails = map[string]string{"3": "FT", "10": "AET", "11": penaltiesDetail}
)

type FootballMatch struct {
	ID       string
	Kickoff  time.Time
	Home     string
	Away     string
	Finished bool
	Score    canon.EventScore
}

func (c *Client) StartedFootballMatches(ctx context.Context, dayOffset int, now time.Time) ([]FootballMatch, error) {
	body, err := c.deps.Get(ctx, c.baseURL+fmt.Sprintf(footballDayFeedPath, dayOffset))
	if err != nil {
		return nil, fmt.Errorf("flashscore football day %d: %w", dayOffset, err)
	}
	var matches []FootballMatch
	for _, rec := range parseRecords(string(body)) {
		if match, ok := startedFootballMatchFrom(rec, now); ok {
			matches = append(matches, match)
		}
	}
	return matches, nil
}

func startedFootballMatchFrom(rec record, now time.Time) (FootballMatch, bool) {
	startUnix, err := strconv.ParseInt(rec[keyStartUnix], 10, 64)
	if rec[keyMatchID] == "" || rec[keyHomeTeam] == "" || rec[keyAwayTeam] == "" || err != nil {
		return FootballMatch{}, false
	}
	score := canon.EventScore{Home: countOf(rec[keyHomeGoals]), Away: countOf(rec[keyAwayGoals])}
	match := FootballMatch{ID: rec[keyMatchID], Kickoff: time.Unix(startUnix, 0).UTC(), Home: rec[keyHomeTeam], Away: rec[keyAwayTeam]}
	switch rec[keyStage] {
	case stageFinished:
		detail, finished := footballFinishedDetails[rec[keyStatusDetail]]
		if !finished {
			return FootballMatch{}, false
		}
		score.Period, score.Detail = canon.PeriodFinished, detail
		match.Finished = true
	case stageLive:
		applyLivePeriod(&score, rec, now)
	default:
		return FootballMatch{}, false
	}
	match.Score = score
	return match, true
}

func applyLivePeriod(score *canon.EventScore, rec record, now time.Time) {
	period, known := footballLivePeriods[rec[keyStatusDetail]]
	if !known {
		return
	}
	score.Period, score.Clock, score.Detail = period.label, period.clock, period.detail
	periodStartUnix, err := strconv.ParseInt(rec[keyPeriodStartUnix], 10, 64)
	if period.length == 0 || err != nil {
		return
	}
	played := max(int(now.Sub(time.Unix(periodStartUnix, 0)).Minutes())+1, 1)
	score.Minute = period.minutesBefore + min(played, period.length)
	score.Clock = strconv.Itoa(score.Minute)
	if played > period.length {
		score.Clock += "+" + strconv.Itoa(played-period.length)
	}
}
