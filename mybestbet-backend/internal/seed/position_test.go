package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"mybestbet/pkg/canon"
)

func TestNormalizePosition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		sport    canon.Sport
		raw      string
		expected string
	}{
		{name: "fotmob squad group", sport: canon.SportFootball, raw: "keepers", expected: "GK"},
		{name: "fotmob attackers", sport: canon.SportFootball, raw: "attackers", expected: "FW"},
		{name: "sofascore letter", sport: canon.SportFootball, raw: "M", expected: "MF"},
		{name: "football goalkeeper letter", sport: canon.SportFootball, raw: "G", expected: "GK"},
		{name: "euroleague position name", sport: canon.SportBasketball, raw: "Center", expected: "C"},
		{name: "espn point guard", sport: canon.SportBasketball, raw: "PG", expected: "G"},
		{name: "combined basketball position keeps primary", sport: canon.SportBasketball, raw: "F-C", expected: "F"},
		{name: "nfl offensive line guard", sport: canon.SportAmericanFootball, raw: "G", expected: "OL"},
		{name: "nfl place kicker", sport: canon.SportAmericanFootball, raw: "PK", expected: "K"},
		{name: "nfl defense", sport: canon.SportAmericanFootball, raw: "CB", expected: "DEF"},
		{name: "already normalized", sport: canon.SportAmericanFootball, raw: "DEF", expected: "DEF"},
		{name: "unknown value", sport: canon.SportBasketball, raw: "NA", expected: ""},
		{name: "sport without positions", sport: canon.SportTennis, raw: "F", expected: ""},
		{name: "missing", sport: canon.SportFootball, raw: "", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, NormalizePosition(tt.sport, tt.raw))
		})
	}
}
