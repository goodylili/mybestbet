package prizepicks

import (
	"strings"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

const (
	boardSoccer      = "SOCCER"
	boardTennis      = "TENNIS"
	eventIDSeparator = "-"
	boardIDExtraKey  = "LeagueId"
)

type leagueBoard struct {
	name             string
	defaultLeagueID  string
	sport            canon.Sport
	canonicalLeague  string
	hasCompetitions  bool
	extraLeagueIDKey string
}

func newBoard(name, defaultLeagueID string, sport canon.Sport, canonicalLeague string) leagueBoard {
	return leagueBoard{
		name:             name,
		defaultLeagueID:  defaultLeagueID,
		sport:            sport,
		canonicalLeague:  canonicalLeague,
		hasCompetitions:  canonicalLeague == "",
		extraLeagueIDKey: strings.ToLower(name) + boardIDExtraKey,
	}
}

var leagueBoards = []leagueBoard{
	newBoard(boardSoccer, "82", canon.SportFootball, ""),
	newBoard(boardTennis, "5", canon.SportTennis, ""),
	newBoard("NBA", "7", canon.SportBasketball, "NBA"),
	newBoard("WNBA", "3", canon.SportBasketball, "WNBA"),
	newBoard("CBB", "20", canon.SportBasketball, "NCAAB"),
	newBoard("NFL", "9", canon.SportAmericanFootball, "NFL"),
	newBoard("CFB", "15", canon.SportAmericanFootball, "NCAAF"),
	newBoard("CS2", "265", canon.SportEsports, "CS2"),
	newBoard("Dota2", "174", canon.SportEsports, "DOTA2"),
	newBoard("VAL", "159", canon.SportEsports, "VALORANT"),
	newBoard("LoL", "121", canon.SportEsports, "LOL"),
}

var boardsByName = func() map[string]leagueBoard {
	m := make(map[string]leagueBoard, len(leagueBoards))
	for _, b := range leagueBoards {
		m[b.name] = b
	}
	return m
}()

func (b leagueBoard) leagueID(cfg sources.Config) string {
	return extraOr(cfg, b.extraLeagueIDKey, b.defaultLeagueID)
}

func (b leagueBoard) leagueOf(game gameAttributes, competitionLeagues map[string]string) string {
	if !b.hasCompetitions {
		return b.canonicalLeague
	}
	league := competitionLeagues[normalize(game.Metadata.GameInfo.Competition)]
	if sportByCompetitionLeague[league] != b.sport {
		return ""
	}
	return league
}

func (b leagueBoard) eventSourceID(gameID string) string {
	if b.name == boardSoccer {
		return gameID
	}
	return b.name + eventIDSeparator + gameID
}

func splitEventSourceID(eventID string) (leagueBoard, string) {
	name, gameID, ok := strings.Cut(eventID, eventIDSeparator)
	if b, known := boardsByName[name]; ok && known {
		return b, gameID
	}
	return boardsByName[boardSoccer], eventID
}

func boardsForLeagues(leagues []string) []leagueBoard {
	var boards []leagueBoard
	for _, b := range leagueBoards {
		for _, league := range leagues {
			if b.carries(league) {
				boards = append(boards, b)
				break
			}
		}
	}
	return boards
}

func (b leagueBoard) carries(league string) bool {
	if b.hasCompetitions {
		return sportByCompetitionLeague[league] == b.sport
	}
	return b.canonicalLeague == league
}
