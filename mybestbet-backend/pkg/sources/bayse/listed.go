package bayse

import (
	"strings"
	"time"
)

const (
	gamesHashtag               = "Games"
	matchupSeparator           = " vs "
	statSuffixSeparator        = ": "
	listedGameExpiryAfterStart = 3 * time.Hour
)

type listedGame struct {
	game    event
	away    string
	home    string
	kickoff time.Time
	props   []event
}

func (g listedGame) events() []event {
	return append([]event{g.game}, g.props...)
}

func (g listedGame) codes() matchCodes {
	return matchCodes{home: g.home, away: g.away}
}

type matchupKey struct {
	matchup     string
	closingDate string
}

func listedGamesOf(events []event) []listedGame {
	var games []listedGame
	index := map[matchupKey]int{}
	for _, ev := range events {
		g, ok := listedGameOf(ev)
		if !ok {
			continue
		}
		index[matchupKey{ev.Title, ev.ClosingDate}] = len(games)
		games = append(games, g)
	}
	for _, ev := range events {
		matchup, _, isStat := strings.Cut(ev.Title, statSuffixSeparator)
		if i, found := index[matchupKey{matchup, ev.ClosingDate}]; isStat && found {
			games[i].props = append(games[i].props, ev)
		}
	}
	return games
}

func listedGameOf(ev event) (listedGame, bool) {
	if !hasHashtag(ev, gamesHashtag) || strings.Contains(ev.Title, statSuffixSeparator) {
		return listedGame{}, false
	}
	away, home, found := strings.Cut(ev.Title, matchupSeparator)
	closing, err := time.Parse(time.RFC3339, ev.ClosingDate)
	if !found || err != nil {
		return listedGame{}, false
	}
	return listedGame{
		game:    ev,
		away:    strings.TrimSpace(away),
		home:    strings.TrimSpace(home),
		kickoff: closing.Add(-listedGameExpiryAfterStart).UTC(),
	}, true
}

func listedGamesBySlug(games []listedGame) map[string]listedGame {
	bySlug := make(map[string]listedGame, len(games))
	for _, g := range games {
		bySlug[g.game.Slug] = g
	}
	return bySlug
}

func hasHashtag(ev event, tag string) bool {
	for _, t := range ev.Hashtags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}
