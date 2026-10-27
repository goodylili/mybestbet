package stats

import "testing"

func TestMatchPlayerByName(t *testing.T) {
	t.Parallel()
	squad := []NamedPlayer{
		{ID: "kinsky", Name: "Antonín Kinský", TeamName: "Tottenham Hotspur"},
		{ID: "davies-spurs", Name: "Ben Davies", TeamName: "Tottenham Hotspur"},
		{ID: "davies-other", Name: "Ben Davies", TeamName: "Coventry City"},
		{ID: "vanhecke", Name: "Jan Paul van Hecke", TeamName: "Tottenham Hotspur"},
	}
	tests := []struct {
		name      string
		input     string
		teamHint  string
		expected  nameMatch
		isMatched bool
	}{
		{"accent-insensitive exact", "Antonin Kinsky", "", nameMatch{playerID: "kinsky", isExact: true}, true},
		{"duplicate name resolved by team", "Ben Davies", "Tottenham", nameMatch{playerID: "davies-spurs", isExact: true}, true},
		{"duplicate name without team is ambiguous", "Ben Davies", "", nameMatch{}, false},
		{"surname plus initial", "J. van Hecke", "", nameMatch{playerID: "vanhecke"}, true},
		{"unknown player", "Harry Kane", "", nameMatch{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := matchPlayerByName(squad, tt.input, tt.teamHint)
			if ok != tt.isMatched || got != tt.expected {
				t.Errorf("matchPlayerByName(%q) = %+v, %v", tt.input, got, ok)
			}
		})
	}
}
