package onexbet

import (
	"slices"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var tennisOverride = sources.Config{LeagueIDs: map[string]string{"ATP_MIAMI": "3109522"}}

func TestAdapter_DiscoverMultiSportGolden(t *testing.T) {
	leagues := []string{"NBA", "NFL", "ATP_MIAMI", "WTA_MIAMI", "CS2", "DOTA2", "LOL", "DFBPOKAL"}
	for _, b := range brands {
		t.Run(b.name, func(t *testing.T) {
			snap, err := newConfiguredReplayAdapter(t, b, tennisOverride).Discover(t.Context(), leagues)
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("discover errors: %v", snap.Errors)
			}
			for _, league := range []string{"NBA", "NFL", "ATP_MIAMI", "CS2", "DOTA2", "DFBPOKAL"} {
				if !slices.ContainsFunc(snap.Events, func(ev canon.SourceEvent) bool { return ev.LeagueHint == league }) {
					t.Fatalf("no event discovered for %s", league)
				}
			}
			sourcestest.AssertGolden(t, fixtureDir, b.name+"_discover_multisport", snap)
		})
	}
}

func TestAdapter_PriceMultiSportGolden(t *testing.T) {
	eventIDs := []string{"764452026", "720869371", "764326424", "763697857", "764442228", "757862480"}
	for _, b := range brands {
		t.Run(b.name, func(t *testing.T) {
			snap, err := newReplayAdapter(t, b).Price(t.Context(), eventIDs)
			if err != nil {
				t.Fatalf("price: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("price errors: %v", snap.Errors)
			}
			sourcestest.RequireMarkets(t, snap, canon.CodeQualify, canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal,
				canon.CodeSetHcp, canon.CodeTotalGames, canon.CodeMapHcp, canon.CodeTotalMaps, canon.TeamGoals)
			sourcestest.RequireMarkets(t, snap, canon.PropPassYards, canon.PropPassTDs, canon.PropRushYards, canon.PropRecYards,
				canon.PropReceptions, canon.PropAnytimeTD, canon.PropAces, canon.PropDoubleFlt, canon.PropMapKills)
			for _, period := range []string{canon.PeriodFirstHalf, canon.PeriodQuarter1, canon.PeriodQuarter4, canon.PeriodFirstSet, canon.PeriodFirstMap} {
				if !slices.ContainsFunc(snap.Markets, func(m canon.RawMarket) bool { return m.Period == period }) {
					t.Fatalf("no market priced for period %s", period)
				}
			}
			sourcestest.AssertGolden(t, fixtureDir, b.name+"_price_multisport", snap)
		})
	}
}

func TestAdapter_PriceTennisPropsUseCompetitors(t *testing.T) {
	snap, err := newReplayAdapter(t, brands[1]).Price(t.Context(), []string{"764326424"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	for _, m := range snap.Markets {
		if m.Code != canon.PropAces && m.Code != canon.PropDoubleFlt {
			continue
		}
		if m.Player == nil || (m.Player.Name != "Daniil Medvedev" && m.Player.Name != "Alexander Bublik") || m.Player.Team != m.Player.Name {
			t.Fatalf("tennis prop %s has player %+v, want a competitor", m.SourceMarketID, m.Player)
		}
	}
}

func TestAdapter_PriceFootballTeamTotals(t *testing.T) {
	snap, err := newReplayAdapter(t, brands[0]).Price(t.Context(), []string{"757862480"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	type teamPeriod struct{ team, period string }
	seen := map[teamPeriod]bool{}
	for _, m := range snap.Markets {
		if m.Code != canon.TeamGoals {
			continue
		}
		if m.Player != nil || m.Line == nil || m.Team != "Hamburger" && m.Team != "Eintracht Frankfurt" {
			t.Fatalf("team total %s = %+v, want a team name and a line", m.SourceMarketID, m)
		}
		for _, s := range m.Selections {
			if s.Side != canon.SideOver && s.Side != canon.SideUnder {
				t.Fatalf("team total %s side %q, want over or under", m.SourceMarketID, s.Side)
			}
		}
		seen[teamPeriod{m.Team, m.Period}] = true
	}
	for _, team := range []string{"Hamburger", "Eintracht Frankfurt"} {
		for _, period := range []string{canon.PeriodFullTime, canon.PeriodFirstHalf, canon.PeriodSecondHalf} {
			if !seen[teamPeriod{team, period}] {
				t.Errorf("no %s team total for period %q", team, period)
			}
		}
	}
}

func TestChampionshipName_Matches(t *testing.T) {
	var resp response[[]championshipSummary]
	if err := json.Unmarshal(sourcestest.ReadFixture(t, fixtureDir, "champs_4.json"), &resp); err != nil {
		t.Fatalf("decode champs: %v", err)
	}
	var esports response[[]championshipSummary]
	if err := json.Unmarshal(sourcestest.ReadFixture(t, fixtureDir, "champs_40.json"), &esports); err != nil {
		t.Fatalf("decode champs: %v", err)
	}
	tests := []struct {
		name     string
		listing  []championshipSummary
		match    championshipName
		expected []int64
	}{
		{name: "exact singles name skips doubles and outrights", listing: resp.Value, match: championshipName{exact: "ATP. Almaty"}, expected: []int64{3109522}},
		{name: "challenger by city", listing: resp.Value, match: championshipName{exact: "Challenger. Cali"}, expected: []int64{103297}},
		{name: "absent tournament", listing: resp.Value, match: championshipName{exact: "ATP. Miami"}},
		{name: "game prefix skips outrights", listing: esports.Value, match: championshipName{prefix: "CS 2. "}, expected: []int64{2918839}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int64
			for _, c := range tt.listing {
				if tt.match.matches(c.Name) {
					got = append(got, c.ID)
				}
			}
			if !slices.Equal(got, tt.expected) {
				t.Fatalf("matches = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGroupRule_BasketballPlayerPoints(t *testing.T) {
	rule := groupRules[groupKey{sport: canon.SportBasketball, scope: playerStats, group: 850}]
	g := game{ID: 1, Home: "Charlotte Hornets", Away: "Miami Heat"}
	observedAt := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		bet          bet
		expectedSide canon.Side
		expectedOK   bool
	}{
		{name: "over", bet: bet{Type: 1807, Odds: 1.9, Param: canon.Line(24.5), Player: &linePlayer{Name: "LaMelo Ball", ID: 7, TeamSide: homeTeamSide}}, expectedSide: canon.SideOver, expectedOK: true},
		{name: "under", bet: bet{Type: 1806, Odds: 1.9, Param: canon.Line(24.5), Player: &linePlayer{Name: "LaMelo Ball", ID: 7, TeamSide: homeTeamSide}}, expectedSide: canon.SideUnder, expectedOK: true},
		{name: "three pointers type in the points group is ignored", bet: bet{Type: 4039, Odds: 1.9, Param: canon.Line(2.5), Player: &linePlayer{Name: "LaMelo Ball", ID: 7}}},
		{name: "missing player", bet: bet{Type: 1807, Odds: 1.9, Param: canon.Line(24.5)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			placed, ok := rule.place(tt.bet, g, observedAt)
			if ok != tt.expectedOK {
				t.Fatalf("place ok = %v, want %v", ok, tt.expectedOK)
			}
			if ok && (placed.selection.Side != tt.expectedSide || *placed.line != 24.5 || placed.player.Team != "Charlotte Hornets") {
				t.Fatalf("placed = %+v %+v", placed.selection, placed.player)
			}
		})
	}
}
