package espn

import (
	"strconv"

	"mybestbet/pkg/canon"
)

const (
	StateLive     = "in"
	StateFinished = "post"
)

var (
	quarterPeriods = []string{canon.PeriodQuarter1, canon.PeriodQuarter2, canon.PeriodQuarter3, canon.PeriodQuarter4}
	halfPeriods    = []string{canon.PeriodFirstHalf, canon.PeriodSecondHalf}
)

func (g Game) IsLive() bool { return g.State == StateLive }

func (g Game) IsFinished() bool { return g.State == StateFinished }

func scoreOf(status statusWire, league League) canon.EventScore {
	score := canon.EventScore{Detail: status.Type.ShortDetail}
	switch status.Type.State {
	case StateFinished:
		score.Period = canon.PeriodFinished
	case StateLive:
		score.Period, score.Clock = periodLabel(status.Period, league), status.DisplayClock
	}
	return score
}

func periodLabel(period int, league League) string {
	periods := quarterPeriods
	if league.PlaysHalves {
		periods = halfPeriods
	}
	switch {
	case period < 1:
		return ""
	case period <= len(periods):
		return periods[period-1]
	}
	return canon.PeriodOvertime
}

func pointsOf(score string) int {
	points, _ := strconv.Atoi(score)
	return points
}
