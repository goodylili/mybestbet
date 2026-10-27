package fotmob

import (
	"errors"
	"net/http"
	"sort"
	"testing"
	"time"

	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

func newReplayClient(t *testing.T) *Client {
	t.Helper()
	srv := sourcestest.ReplayServer(t, SourceName, func(r *http.Request) (string, bool) {
		switch r.URL.Path {
		case "/api/data/leagues":
			return "league_" + r.URL.Query().Get("id") + ".json", r.URL.Query().Get("id") == "47"
		case "/api/data/matches":
			return "matches_" + r.URL.Query().Get("date") + ".json", r.URL.Query().Get("date") == "20261026"
		case "/api/data/teams":
			id := r.URL.Query().Get("id")
			return "team_" + id + ".json", id == "9902" || id == "9825" || id == "8491"
		case "/api/data/matchDetails":
			id := r.URL.Query().Get("matchId")
			return "match_" + id + ".json", id == "5795484" || id == "5795491" || id == "5749716"
		}
		return "", false
	})
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Config.BaseURL = srv.URL + "/api/data"
	return New(deps)
}

func TestLeagueMatchesAndFindMatch(t *testing.T) {
	t.Parallel()
	matches, err := newReplayClient(t).LeagueMatches(t.Context(), "EPL")
	if err != nil {
		t.Fatalf("LeagueMatches: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3", len(matches))
	}
	kickoff := time.Date(2026, 10, 19, 19, 5, 0, 0, time.UTC)
	got, ok := FindMatch(matches, kickoff, "Tottenham", "Coventry City FC")
	if !ok || got.ID != "5795484" || !got.Finished {
		t.Fatalf("FindMatch = %+v, %v", got, ok)
	}
	if _, ok := FindMatch(matches, kickoff.Add(time.Hour), "Tottenham Hotspur", "Coventry City"); ok {
		t.Fatal("matched outside kickoff tolerance")
	}
}

func TestTeamSquadSkipsCoachAndKeepsTeamIDs(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	matches, err := c.LeagueMatches(t.Context(), "EPL")
	if err != nil {
		t.Fatalf("LeagueMatches: %v", err)
	}
	if matches[2].HomeID != "9902" {
		t.Fatalf("HomeID = %q, want 9902", matches[2].HomeID)
	}
	squad, err := c.TeamSquad(t.Context(), matches[2].HomeID)
	if err != nil {
		t.Fatalf("TeamSquad: %v", err)
	}
	if len(squad) != 12 || squad[0].ID == "" || squad[0].Position != "keepers" {
		t.Fatalf("squad = %+v, want 12 players without the coach, keepers first", squad)
	}
}

func TestLeagueMatchesUnknownLeague(t *testing.T) {
	t.Parallel()
	_, err := newReplayClient(t).LeagueMatches(t.Context(), "WC")
	if !errors.Is(err, ErrUnknownLeague) {
		t.Fatalf("err = %v", err)
	}
}

func TestMatchDetailsPlayerLines(t *testing.T) {
	t.Parallel()
	d, err := newReplayClient(t).MatchDetails(t.Context(), "5795484")
	if err != nil {
		t.Fatalf("MatchDetails: %v", err)
	}
	if !d.IsFinished() || !d.IsLineupConfirmed() {
		t.Fatal("finished confirmed match not reported as such")
	}
	lines := d.PlayerLines()
	sort.Slice(lines, func(i, j int) bool { return lines[i].FotmobID < lines[j].FotmobID })
	byName := map[string]PlayerLine{}
	for _, l := range lines {
		byName[l.Name] = l
	}
	if len(lines) != 13 {
		t.Fatalf("got %d lines, want 13", len(lines))
	}
	solanke := byName["Dominic Solanke"]
	if !solanke.IsHome || solanke.Minutes != 90 || solanke.Assists != 1 || solanke.Shots != 3 || solanke.ShotsOnTarget != 0 {
		t.Errorf("solanke = %+v", solanke)
	}
	thomas := byName["Bobby Thomas"]
	if thomas.IsHome || thomas.Shots != 2 {
		t.Errorf("thomas = %+v", thomas)
	}
	if kinsky := byName["Antonín Kinský"]; kinsky.Shots != 0 || kinsky.Minutes != 90 {
		t.Errorf("kinsky = %+v", kinsky)
	}
}

func TestLineupEntries(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	d, err := c.MatchDetails(t.Context(), "5795484")
	if err != nil {
		t.Fatalf("MatchDetails: %v", err)
	}
	entries := d.LineupEntries()
	if len(entries) != 13 {
		t.Fatalf("got %d entries, want 13", len(entries))
	}
	if e := entries[4]; e.Name != "Ben Davies" || !e.Substitute || !e.IsHome {
		t.Errorf("entry 4 = %+v", e)
	}
	if e := entries[6]; e.Name != "Rodrigo Bentancur" || !e.Missing {
		t.Errorf("entry 6 = %+v", e)
	}

	predicted, err := c.MatchDetails(t.Context(), "5795491")
	if err != nil {
		t.Fatalf("MatchDetails predicted: %v", err)
	}
	if predicted.IsLineupConfirmed() || predicted.IsFinished() || len(predicted.PlayerLines()) != 0 {
		t.Fatal("predicted lineup treated as confirmed")
	}
}

func TestPlayerLinesReadFoulsTacklesAndCards(t *testing.T) {
	t.Parallel()
	d, err := newReplayClient(t).MatchDetails(t.Context(), "5749716")
	if err != nil {
		t.Fatalf("MatchDetails: %v", err)
	}
	byName := map[string]PlayerLine{}
	for _, l := range d.PlayerLines() {
		byName[l.Name] = l
	}
	tests := []struct {
		name string
		want PlayerLine
	}{
		{name: "Kialonda Gaspar", want: PlayerLine{FotmobID: 1206806, Name: "Kialonda Gaspar", IsHome: true, Minutes: 90, Shots: 2, ShotsOnTarget: 1, Fouls: 3, Tackles: 2, Yellow: 1, Red: 1}},
		{name: "Wladimiro Falcone", want: PlayerLine{FotmobID: byName["Wladimiro Falcone"].FotmobID, Name: "Wladimiro Falcone", IsHome: true, Minutes: 90}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := byName[tt.name]; got != tt.want {
				t.Errorf("line = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFinishedFixturesSkipUnplayed(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	fixtures, err := c.FinishedFixtures(t.Context(), "9825")
	if err != nil {
		t.Fatalf("FinishedFixtures: %v", err)
	}
	if len(fixtures) != 3 {
		t.Fatalf("got %d fixtures, want 3 finished", len(fixtures))
	}
	epl := fixtures[1]
	if epl.ID != "5795363" || epl.LeagueID != "47" || epl.Kickoff.IsZero() || epl.HomeID == "" || epl.AwayID == "" {
		t.Errorf("fixture = %+v", epl)
	}
	if code, ok := c.LeagueCodeOf(epl.LeagueID); !ok || code != "EPL" {
		t.Errorf("LeagueCodeOf(47) = %q, %v", code, ok)
	}
	if _, ok := c.LeagueCodeOf(fixtures[0].LeagueID); ok {
		t.Error("club friendlies mapped to a league")
	}
}

func TestTeamSquadKeepsClubOfNationalTeamPlayers(t *testing.T) {
	t.Parallel()
	c := newReplayClient(t)
	national, err := c.TeamSquad(t.Context(), "8491")
	if err != nil {
		t.Fatalf("TeamSquad national: %v", err)
	}
	clubs := map[string]string{}
	for _, p := range national {
		clubs[p.Name] = p.ClubID
	}
	if clubs["Bukayo Saka"] != "9825" || clubs["Jordan Pickford"] != "8668" {
		t.Errorf("national squad clubs = %v", clubs)
	}
	club, err := c.TeamSquad(t.Context(), "9825")
	if err != nil {
		t.Fatalf("TeamSquad club: %v", err)
	}
	for _, p := range club {
		if p.ClubID != "" {
			t.Errorf("club squad player %s has club id %q, want none for a country code", p.Name, p.ClubID)
		}
	}
}
