package sofascore

type PlayerLine struct {
	Player        Player
	TeamID        int64
	IsHome        bool
	Started       bool
	HasStatistics bool
	Minutes       int
	Goals         int
	Assists       int
	Shots         int
	ShotsOnTarget int
	Fouls         int
	Tackles       int
	Yellow        int
	Red           int
	Rating        *float64
}

type LineupEntry struct {
	Player     Player
	TeamID     int64
	IsHome     bool
	Substitute bool
	Missing    bool
}

type cardCount struct{ yellow, red int }

const (
	incidentCard     = "card"
	cardYellow       = "yellow"
	cardRed          = "red"
	cardSecondYellow = "yellowRed"
)

func MatchLines(l Lineups, incidents []Incident) []PlayerLine {
	cards := countCards(incidents)
	lines := make([]PlayerLine, 0, len(l.Home.Players)+len(l.Away.Players))
	lines = appendSideLines(lines, l.Home, true, cards)
	return appendSideLines(lines, l.Away, false, cards)
}

func appendSideLines(lines []PlayerLine, side LineupSide, isHome bool, cards map[int64]cardCount) []PlayerLine {
	for _, p := range side.Players {
		line := PlayerLine{Player: p.Player, TeamID: p.TeamID, IsHome: isHome, Started: !p.Substitute}
		if s := p.Statistics; s != nil && s.MinutesPlayed > 0 {
			line.HasStatistics = true
			line.Minutes, line.Goals, line.Assists = s.MinutesPlayed, s.Goals, s.GoalAssist
			line.Shots, line.ShotsOnTarget = s.TotalShots, s.OnTargetScoringAttempt
			line.Fouls, line.Tackles, line.Rating = s.Fouls, s.TotalTackle, s.Rating
		}
		c := cards[p.Player.ID]
		line.Yellow, line.Red = c.yellow, c.red
		lines = append(lines, line)
	}
	return lines
}

func countCards(incidents []Incident) map[int64]cardCount {
	cards := map[int64]cardCount{}
	for _, inc := range incidents {
		if inc.IncidentType != incidentCard || inc.Player == nil || inc.Rescinded {
			continue
		}
		c := cards[inc.Player.ID]
		switch inc.IncidentClass {
		case cardYellow:
			c.yellow++
		case cardRed, cardSecondYellow:
			c.red++
		}
		cards[inc.Player.ID] = c
	}
	return cards
}

func (l Lineups) Entries() []LineupEntry {
	entries := appendSideEntries(nil, l.Home, true)
	return appendSideEntries(entries, l.Away, false)
}

func appendSideEntries(entries []LineupEntry, side LineupSide, isHome bool) []LineupEntry {
	var teamID int64
	for _, p := range side.Players {
		teamID = p.TeamID
		entries = append(entries, LineupEntry{Player: p.Player, TeamID: p.TeamID, IsHome: isHome, Substitute: p.Substitute})
	}
	for _, m := range side.MissingPlayers {
		entries = append(entries, LineupEntry{Player: m.Player, TeamID: teamID, IsHome: isHome, Missing: true})
	}
	return entries
}
