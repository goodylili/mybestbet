package flashscore

import (
	"strconv"
	"strings"

	"mybestbet/pkg/canon"
)

const (
	keyHomeSets   = "AG"
	keyAwaySets   = "AH"
	keyHomePoints = "WA"
	keyAwayPoints = "WB"
	setPrefix     = "S"
)

var setGameKeys = [][2]string{{"BA", "BB"}, {"BC", "BD"}, {"BE", "BF"}, {"BG", "BH"}, {"BI", "BJ"}}

func IsOver(status MatchStatus) bool {
	return status == StatusFinished || status == StatusRetired || status == StatusWalkover
}

func scoreOf(rec record, isLive bool) canon.EventScore {
	score := canon.EventScore{Home: countOf(rec[keyHomeSets]), Away: countOf(rec[keyAwaySets])}
	var sets []string
	for _, keys := range setGameKeys {
		home, hasHome := rec[keys[0]]
		away, hasAway := rec[keys[1]]
		if !hasHome || !hasAway {
			break
		}
		sets = append(sets, home+"-"+away)
	}
	score.Detail = strings.Join(sets, " ")
	if !isLive {
		score.Period = canon.PeriodFinished
		return score
	}
	if len(sets) > 0 {
		score.Period = setPrefix + strconv.Itoa(len(sets))
	}
	if home, away := rec[keyHomePoints], rec[keyAwayPoints]; home != "" && away != "" {
		score.Clock = home + "-" + away
	}
	return score
}

func countOf(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}
