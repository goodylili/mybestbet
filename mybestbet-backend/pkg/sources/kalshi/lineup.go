package kalshi

import (
	"regexp"
	"slices"
	"strings"

	"mybestbet/pkg/canon"
)

type competitor struct {
	code   string
	teamID string
	name   string
}

type lineup struct {
	home competitor
	away competitor
}

var (
	tickerDateAndTime = regexp.MustCompile(`^\d{2}[A-Z]{3}\d{2}(\d{4})?`)
	titleTeams        = regexp.MustCompile(`^(.+?) vs\.? (.+)$`)
)

const (
	spreadMarginDigits   = "0123456789"
	regulationTimePrefix = "Reg Time: "
)

func gameKey(eventTicker string) string {
	_, key, _ := strings.Cut(eventTicker, "-")
	return key
}

func concatenatedTeamCodes(eventTicker string) string {
	key := gameKey(eventTicker)
	return strings.TrimPrefix(key, tickerDateAndTime.FindString(key))
}

func lineupOf(game event, awayListedFirst bool) (lineup, bool) {
	competitors := competitorsOf(game)
	if len(competitors) != 2 {
		return lineup{}, false
	}
	first, second, ok := inListedOrder(game, competitors[0], competitors[1])
	if !ok {
		return lineup{}, false
	}
	return orient(first, second, awayListedFirst), true
}

func lineupFromTitle(title string, awayListedFirst bool) (lineup, bool) {
	m := titleTeams.FindStringSubmatch(title)
	if m == nil {
		return lineup{}, false
	}
	return orient(competitor{name: m[1]}, competitor{name: m[2]}, awayListedFirst), true
}

func orient(first, second competitor, awayListedFirst bool) lineup {
	if awayListedFirst {
		return lineup{home: second, away: first}
	}
	return lineup{home: first, away: second}
}

func competitorsOf(game event) []competitor {
	var competitors []competitor
	for _, m := range game.Markets {
		code := marketSuffix(game, m)
		if code == tieSuffix {
			continue
		}
		competitors = append(competitors, competitor{code: code, teamID: m.CustomStrike.team(), name: strings.TrimPrefix(m.YesSubTitle, regulationTimePrefix)})
	}
	return competitors
}

func inListedOrder(game event, a, b competitor) (first, second competitor, ok bool) {
	codes := concatenatedTeamCodes(game.EventTicker)
	switch {
	case codes == a.code+b.code:
		return a, b, true
	case codes == b.code+a.code:
		return b, a, true
	case a.name != "" && strings.HasPrefix(game.Title, a.name):
		return a, b, true
	case b.name != "" && strings.HasPrefix(game.Title, b.name):
		return b, a, true
	}
	return competitor{}, competitor{}, false
}

func (l lineup) sideOfCode(code string) (canon.Side, bool) {
	switch code {
	case tieSuffix:
		return canon.SideDraw, true
	case l.home.code:
		return canon.SideHome, true
	case l.away.code:
		return canon.SideAway, true
	}
	return "", false
}

func (l lineup) sideOfTeam(teamID string) (canon.Side, bool) {
	if teamID == "" {
		return "", false
	}
	switch teamID {
	case l.home.teamID:
		return canon.SideHome, true
	case l.away.teamID:
		return canon.SideAway, true
	}
	return "", false
}

func (l lineup) sideOfMarginMarket(m market, suffix string) (canon.Side, bool) {
	if side, ok := l.sideOfTeam(m.CustomStrike.team()); ok {
		return side, true
	}
	return l.sideOfCode(strings.TrimRight(suffix, spreadMarginDigits))
}

func (l lineup) sideOfPlayerCode(playerCode string) (canon.Side, bool) {
	sides := []canon.Side{canon.SideHome, canon.SideAway}
	if len(l.away.code) > len(l.home.code) {
		slices.Reverse(sides)
	}
	for _, side := range sides {
		code := l.competitor(side).code
		if code != "" && strings.HasPrefix(playerCode, code) {
			return side, true
		}
	}
	return "", false
}

func (l lineup) competitor(side canon.Side) competitor {
	if side == canon.SideAway {
		return l.away
	}
	return l.home
}

func (l lineup) teamName(side canon.Side) string { return l.competitor(side).name }
