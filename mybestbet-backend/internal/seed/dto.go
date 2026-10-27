package seed

import (
	"time"

	"mybestbet/pkg/canon"
)

var DefaultUniqueTournaments = map[string]int64{
	"EPL": 17, "LALIGA": 8, "SERIEA": 23, "BUNDES": 35, "LIGUE1": 34,
	"ERED": 37, "PRIMEIRA": 238, "CHAMP": 18, "SPL": 955, "MLS": 242,
	"WC": 16, "FRIENDLY": 851, "UCL": 7, "UEL": 679, "UECL": 17015, "UWCL": 696,
	"FACUP": 19, "EFLCUP": 21, "COPADELREY": 329, "COPPAITALIA": 328, "DFBPOKAL": 217, "CDF": 335,
	"SUPERLIG": 52, "SPFL": 36, "JPL": 38, "BRA1": 325, "BRA2": 390, "ARG1": 155,
	"SSL": 215, "GSL": 185, "UPL": 218, "RPL": 203, "ELITESERIEN": 20, "DSL": 39,
	"J1": 196, "J2": 197, "KLEAGUE": 410, "CSL": 649, "ISL": 1900, "ALEAGUE": 136,
	"LIBERTADORES": 384, "SUDAMERICANA": 480, "SUPERCOPA": 213,
}

var DefaultFotMobLeagues = map[string]string{
	"EPL": "47", "LALIGA": "87", "SERIEA": "55", "BUNDES": "54", "LIGUE1": "53",
	"ERED": "57", "PRIMEIRA": "61", "CHAMP": "48", "SPL": "536", "MLS": "130",
	"WC": "77", "FRIENDLY": "114", "UCL": "42", "UEL": "73", "UECL": "10216", "UWCL": "9375",
	"FACUP": "132", "EFLCUP": "133", "COPADELREY": "138", "COPPAITALIA": "141", "DFBPOKAL": "209", "CDF": "134",
	"SUPERLIG": "71", "SPFL": "64", "JPL": "40", "LIGAMX": "230", "BRA1": "268", "BRA2": "8814", "ARG1": "112",
	"SSL": "69", "GSL": "135", "UPL": "441", "RPL": "63", "ELITESERIEN": "59", "DSL": "46",
	"J1": "223", "J2": "8974", "KLEAGUE": "9080", "CSL": "120", "ISL": "9478", "ALEAGUE": "113",
	"LEAGUESCUP": "10043", "LIBERTADORES": "45", "SUDAMERICANA": "299", "SUPERCOPA": "139",
}

type CompetitionFormat int

const (
	DomesticLeague CompetitionFormat = iota
	ClubCup
	NationalTeams
)

var competitionFormats = map[string]CompetitionFormat{
	"UCL": ClubCup, "UEL": ClubCup, "UECL": ClubCup, "UWCL": ClubCup,
	"FACUP": ClubCup, "EFLCUP": ClubCup, "COPADELREY": ClubCup, "COPPAITALIA": ClubCup,
	"DFBPOKAL": ClubCup, "CDF": ClubCup, "SUPERCOPA": ClubCup, "LEAGUESCUP": ClubCup,
	"LIBERTADORES": ClubCup, "SUDAMERICANA": ClubCup,
	"WC": NationalTeams, "FRIENDLY": NationalTeams,
}

func FormatOf(leagueCode string) CompetitionFormat { return competitionFormats[leagueCode] }

type League struct {
	Code               string
	Name               string
	Country            string
	Sport              canon.Sport
	UniqueTournamentID int64
}

func (l League) Format() CompetitionFormat { return FormatOf(l.Code) }

type Team struct {
	SofascoreID int64
	LeagueCode  string
	Name        string
	ShortName   string
}

type Player struct {
	SofascoreID int64
	TeamID      string
	Name        string
	Position    string
}

type SourceTeam struct {
	Source     string
	SourceID   string
	LeagueCode string
	Name       string
	Aliases    []string
	LogoURL    string
}

type SourcePlayer struct {
	Source       string
	SourceID     string
	TeamID       string
	Name         string
	Position     string
	ClubSourceID string
}

type EventStatus string

const (
	EventScheduled EventStatus = "scheduled"
	EventLive      EventStatus = "live"
	EventFinished  EventStatus = "finished"
	EventPostponed EventStatus = "postponed"
	EventCancelled EventStatus = "cancelled"
)

type Event struct {
	SofascoreID int64
	LeagueCode  string
	HomeTeamID  string
	AwayTeamID  string
	Kickoff     time.Time
	Status      EventStatus
}

type SourceEvent struct {
	Source     string
	SourceID   string
	LeagueCode string
	HomeTeamID string
	AwayTeamID string
	Kickoff    time.Time
	Status     EventStatus
}

func StatusFromSofascore(statusType string) EventStatus {
	switch statusType {
	case "inprogress":
		return EventLive
	case "finished":
		return EventFinished
	case "postponed":
		return EventPostponed
	case "canceled", "cancelled":
		return EventCancelled
	}
	return EventScheduled
}
