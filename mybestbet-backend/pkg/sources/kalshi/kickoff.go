package kalshi

import (
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	tickerKickoffLayout = "06Jan021504"
	exchangeTimeZone    = "America/New_York"
)

func kickoffFromTicker(eventTicker string) (time.Time, bool) {
	stamp := tickerDateAndTime.FindString(gameKey(eventTicker))
	if len(stamp) != len(tickerKickoffLayout) {
		return time.Time{}, false
	}
	eastern, err := time.LoadLocation(exchangeTimeZone)
	if err != nil {
		return time.Time{}, false
	}
	monthInLayoutCase := stamp[:3] + strings.ToLower(stamp[3:5]) + stamp[5:]
	kickoff, err := time.ParseInLocation(tickerKickoffLayout, monthInLayoutCase, eastern)
	if err != nil {
		return time.Time{}, false
	}
	return kickoff.UTC(), true
}
