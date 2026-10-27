package fotmob

const (
	confirmedLineupType = "standard"
	keyMinutes          = "minutes_played"
	keyGoals            = "goals"
	keyAssists          = "assists"
	keyShots            = "total_shots"
	keyShotsOnTarget    = "ShotsOnTarget"
	keyFouls            = "fouls"
	keyTackles          = "matchstats.headers.tackles"
	cardEventType       = "Card"
	cardYellow          = "Yellow"
	cardRed             = "Red"
	cardSecondYellow    = "YellowRed"
)

type Details struct {
	raw matchResponse
}

type LineupEntry struct {
	FotmobID   int64
	Name       string
	IsHome     bool
	Substitute bool
	Missing    bool
}

type PlayerLine struct {
	FotmobID      int64
	Name          string
	IsHome        bool
	Minutes       int
	Goals         int
	Assists       int
	Shots         int
	ShotsOnTarget int
	Fouls         int
	Tackles       int
	Yellow        int
	Red           int
}

func (d Details) IsFinished() bool { return d.raw.General.Finished }

func (d Details) IsLineupConfirmed() bool {
	return d.raw.Content.Lineup.LineupType == confirmedLineupType
}

func (d Details) LineupEntries() []LineupEntry {
	l := d.raw.Content.Lineup
	entries := appendTeamEntries(nil, l.HomeTeam, true)
	return appendTeamEntries(entries, l.AwayTeam, false)
}

func appendTeamEntries(entries []LineupEntry, team lineupTeam, isHome bool) []LineupEntry {
	for _, p := range team.Starters {
		entries = append(entries, LineupEntry{FotmobID: p.ID, Name: p.Name, IsHome: isHome})
	}
	for _, p := range team.Subs {
		entries = append(entries, LineupEntry{FotmobID: p.ID, Name: p.Name, IsHome: isHome, Substitute: true})
	}
	for _, p := range team.Unavailable {
		entries = append(entries, LineupEntry{FotmobID: p.ID, Name: p.Name, IsHome: isHome, Missing: true})
	}
	return entries
}

func (d Details) PlayerLines() []PlayerLine {
	homeTeamID := d.raw.General.HomeTeam.ID
	yellow, red := d.cardsByPlayer()
	lines := make([]PlayerLine, 0, len(d.raw.Content.PlayerStats))
	for _, p := range d.raw.Content.PlayerStats {
		values := statValues(p)
		if values[keyMinutes] <= 0 {
			continue
		}
		lines = append(lines, PlayerLine{
			FotmobID:      p.ID,
			Name:          p.Name,
			IsHome:        p.TeamID == homeTeamID,
			Minutes:       values[keyMinutes],
			Goals:         values[keyGoals],
			Assists:       values[keyAssists],
			Shots:         values[keyShots],
			ShotsOnTarget: values[keyShotsOnTarget],
			Fouls:         values[keyFouls],
			Tackles:       values[keyTackles],
			Yellow:        yellow[p.ID],
			Red:           red[p.ID],
		})
	}
	return lines
}

func (d Details) cardsByPlayer() (yellow, red map[int64]int) {
	yellow, red = map[int64]int{}, map[int64]int{}
	for _, e := range d.raw.Content.MatchFacts.Events.Events {
		if e.Type != cardEventType {
			continue
		}
		switch e.Card {
		case cardYellow:
			yellow[e.Player.ID]++
		case cardRed, cardSecondYellow:
			red[e.Player.ID]++
		}
	}
	return yellow, red
}

func statValues(p playerStats) map[string]int {
	values := map[string]int{}
	for _, group := range p.Stats {
		for _, entry := range group.Stats {
			if _, seen := values[entry.Key]; !seen && entry.Stat.Value != nil {
				values[entry.Key] = int(*entry.Stat.Value)
			}
		}
	}
	return values
}
