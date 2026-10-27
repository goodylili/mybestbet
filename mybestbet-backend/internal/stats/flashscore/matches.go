package flashscore

import (
	"strconv"
	"time"

	"mybestbet/pkg/canon"
)

type MatchStatus int

const (
	StatusUnknown MatchStatus = iota
	StatusScheduled
	StatusLive
	StatusFinished
	StatusRetired
	StatusWalkover
	StatusCancelled
)

var statusByDetail = map[string]MatchStatus{
	"3": StatusFinished,
	"5": StatusCancelled,
	"8": StatusRetired,
	"9": StatusWalkover,
}

var statusByStage = map[string]MatchStatus{
	"1": StatusScheduled,
	"2": StatusLive,
}

type Player struct {
	ID        string
	Name      string
	ShortName string
}

type Match struct {
	ID         string
	LeagueCode string
	Tournament Tournament
	Kickoff    time.Time
	Status     MatchStatus
	Home       Player
	Away       Player
	Score      canon.EventScore
}

func matchesFrom(records []record) []Match {
	var matches []Match
	tournament, isTourSingles := Tournament{}, false
	for _, rec := range records {
		if header, isSection := rec[keySectionHeader]; isSection {
			tournament, isTourSingles = tournamentOfSection(header, rec[keySectionPath])
			continue
		}
		if !isTourSingles {
			continue
		}
		if match, ok := matchFrom(rec, tournament); ok {
			matches = append(matches, match)
		}
	}
	return matches
}

func matchFrom(rec record, tournament Tournament) (Match, bool) {
	startUnix, err := strconv.ParseInt(rec[keyStartUnix], 10, 64)
	if rec[keyMatchID] == "" || err != nil {
		return Match{}, false
	}
	match := Match{
		ID:         rec[keyMatchID],
		LeagueCode: tournament.Code,
		Tournament: tournament,
		Kickoff:    time.Unix(startUnix, 0).UTC(),
		Status:     statusOf(rec),
		Home:       playerFrom(rec[keyHomeID], rec[keyHomeShortName], rec[keyHomeSlug]),
		Away:       playerFrom(rec[keyAwayID], rec[keyAwayShortName], rec[keyAwaySlug]),
	}
	if match.Status == StatusLive || IsOver(match.Status) {
		match.Score = scoreOf(rec, match.Status == StatusLive)
	}
	return match, match.Home.ID != "" && match.Away.ID != ""
}

func playerFrom(id, shortName, slug string) Player {
	return Player{ID: id, Name: fullName(shortName, slug), ShortName: shortName}
}

func statusOf(rec record) MatchStatus {
	if status, ok := statusByDetail[rec[keyStatusDetail]]; ok {
		return status
	}
	return statusByStage[rec[keyStage]]
}
