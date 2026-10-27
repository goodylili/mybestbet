package seed

import (
	"strings"

	"mybestbet/pkg/canon"
)

var positionsBySport = map[canon.Sport]map[string]string{
	canon.SportFootball: {
		"G": "GK", "GK": "GK", "GOALKEEPER": "GK", "KEEPER": "GK", "KEEPERS": "GK", "GOALKEEPERS": "GK",
		"D": "DF", "DF": "DF", "DEFENDER": "DF", "DEFENDERS": "DF",
		"M": "MF", "MF": "MF", "MIDFIELDER": "MF", "MIDFIELDERS": "MF",
		"F": "FW", "FW": "FW", "FORWARD": "FW", "FORWARDS": "FW", "ATTACKER": "FW", "ATTACKERS": "FW",
	},
	canon.SportBasketball: {
		"G": "G", "PG": "G", "SG": "G", "GUARD": "G", "POINT GUARD": "G", "SHOOTING GUARD": "G",
		"F": "F", "SF": "F", "PF": "F", "FORWARD": "F", "SMALL FORWARD": "F", "POWER FORWARD": "F",
		"C": "C", "CENTER": "C", "CENTRE": "C",
	},
	canon.SportAmericanFootball: {
		"QB": "QB", "RB": "RB", "FB": "RB", "WR": "WR", "TE": "TE",
		"OL": "OL", "OT": "OL", "T": "OL", "OG": "OL", "G": "OL", "C": "OL",
		"DEF": "DEF", "DL": "DEF", "DT": "DEF", "DE": "DEF", "NT": "DEF", "EDGE": "DEF", "LB": "DEF", "ILB": "DEF", "OLB": "DEF", "MLB": "DEF",
		"DB": "DEF", "CB": "DEF", "S": "DEF", "SS": "DEF", "FS": "DEF",
		"K": "K", "PK": "K", "P": "P", "LS": "LS",
	},
}

func NormalizePosition(sport canon.Sport, raw string) string {
	key := strings.ToUpper(strings.TrimSpace(raw))
	if primary, _, combined := strings.Cut(key, "-"); combined {
		key = primary
	}
	return positionsBySport[sport][key]
}
