package resolve

import (
	"fmt"
	"time"
)

const (
	reviewKickoffFormat   = "2006-01-02 15:04 MST"
	eventCandidateNameFmt = "%s v %s %s"
	maxClockShiftHours    = 3
)

type eventCandidate struct {
	event   Event
	flipped bool
}

func (c *Cache) eventCandidates(league, homeTeamID, awayTeamID string, kickoff time.Time) []eventCandidate {
	tolerance := c.kickoffToleranceIn(league)
	return c.eventsMatching(league, homeTeamID, awayTeamID, func(e Event) bool {
		return absDuration(e.Kickoff.Sub(kickoff)) <= tolerance
	})
}

func (c *Cache) fixtureCandidates(league, homeTeamID, awayTeamID string, kickoff time.Time) []eventCandidate {
	if candidates := c.eventCandidates(league, homeTeamID, awayTeamID, kickoff); len(candidates) > 0 {
		return candidates
	}
	tolerance := c.kickoffToleranceIn(league)
	return c.eventsMatching(league, homeTeamID, awayTeamID, func(e Event) bool {
		return isClockShift(e.Kickoff.Sub(kickoff), tolerance)
	})
}

func (c *Cache) eventsMatching(league, homeTeamID, awayTeamID string, keep func(Event) bool) []eventCandidate {
	var candidates []eventCandidate
	for _, e := range c.eventsBetween(homeTeamID, awayTeamID) {
		if e.League != league || !keep(e) {
			continue
		}
		candidates = append(candidates, eventCandidate{event: e, flipped: e.HomeTeamID != homeTeamID})
	}
	return candidates
}

func isClockShift(offset, tolerance time.Duration) bool {
	offset = absDuration(offset)
	for hours := 1; hours <= maxClockShiftHours; hours++ {
		if absDuration(offset-time.Duration(hours)*time.Hour) <= tolerance {
			return true
		}
	}
	return false
}

func (c *Cache) eventCandidateScores(league string, candidates []eventCandidate, kickoff time.Time) []scoredCandidate {
	tolerance := c.kickoffToleranceIn(league)
	ranked := make([]scoredCandidate, len(candidates))
	for i, cand := range candidates {
		home, _ := c.team(cand.event.HomeTeamID)
		away, _ := c.team(cand.event.AwayTeamID)
		ranked[i] = scoredCandidate{
			id:    cand.event.ID,
			name:  fmt.Sprintf(eventCandidateNameFmt, home.Name, away.Name, cand.event.Kickoff.UTC().Format(reviewKickoffFormat)),
			score: 1 - float64(absDuration(cand.event.Kickoff.Sub(kickoff)))/float64(tolerance),
		}
	}
	sortByScoreDesc(ranked)
	return ranked
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
