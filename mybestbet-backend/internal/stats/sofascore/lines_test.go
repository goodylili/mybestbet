package sofascore

import (
	"testing"
)

func TestMatchLinesMergesStatisticsAndCards(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	lineups, err := c.Lineups(t.Context(), 16363890)
	if err != nil {
		t.Fatalf("Lineups: %v", err)
	}
	incidents, err := c.Incidents(t.Context(), 16363890)
	if err != nil {
		t.Fatalf("Incidents: %v", err)
	}
	lines := MatchLines(lineups, incidents)
	if len(lines) != 16 {
		t.Fatalf("got %d lines, want 16", len(lines))
	}
	byID := map[int64]PlayerLine{}
	for _, l := range lines {
		byID[l.Player.ID] = l
	}

	davies := byID[94758]
	if davies.Started || !davies.IsHome || davies.TeamID != 33 || davies.Minutes != 21 || davies.Goals != 1 || davies.Shots != 1 || davies.ShotsOnTarget != 1 {
		t.Errorf("davies = %+v", davies)
	}
	if davies.Rating == nil || *davies.Rating != 7.9 {
		t.Errorf("davies rating = %v", davies.Rating)
	}

	dasilva := byID[791985]
	if !dasilva.Started || dasilva.IsHome || dasilva.Yellow != 1 || dasilva.Red != 0 || dasilva.Fouls != 2 || dasilva.Tackles != 2 {
		t.Errorf("dasilva = %+v", dasilva)
	}

	vanHecke := byID[962012]
	if vanHecke.Yellow != 0 || vanHecke.Tackles != 3 || vanHecke.Minutes != 90 {
		t.Errorf("van hecke = %+v", vanHecke)
	}
}

func TestCountCardsSecondYellowIsRedAndRescindedIgnored(t *testing.T) {
	t.Parallel()
	p := &Player{ID: 7}
	cards := countCards([]Incident{
		{IncidentType: "card", IncidentClass: "yellow", Player: p},
		{IncidentType: "card", IncidentClass: "yellowRed", Player: p},
		{IncidentType: "card", IncidentClass: "red", Player: &Player{ID: 8}, Rescinded: true},
		{IncidentType: "goal", Player: p},
	})
	if cards[7] != (cardCount{yellow: 1, red: 1}) || cards[8] != (cardCount{}) {
		t.Fatalf("cards = %+v", cards)
	}
}

func TestEntriesMarkBenchAndOut(t *testing.T) {
	t.Parallel()
	lineups, err := newReplayClient(t).Lineups(t.Context(), 16363897)
	if err != nil {
		t.Fatalf("Lineups: %v", err)
	}
	if lineups.Confirmed {
		t.Fatal("predicted lineup reported as confirmed")
	}
	entries := lineups.Entries()
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(entries))
	}
	out := entries[3]
	if !out.Missing || out.Player.ID != 371222 || out.TeamID != 32 || !out.IsHome {
		t.Fatalf("missing entry = %+v", out)
	}
}
