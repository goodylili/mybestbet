package sporty

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

var brandFixtureDirs = []struct {
	brand string
	dir   string
}{
	{brand: SportyBet, dir: "sportybet"},
	{brand: FootballCom, dir: "footballcom"},
}

var multiSportConfig = sources.Config{LeagueIDs: map[string]string{"ATP_MIAMI": "sr:tournament:2613"}}

func TestAdapter_DiscoverMultiSportGolden(t *testing.T) {
	leagues := []string{"NBA", "NFL", "ATP_MIAMI", "WTA_MIAMI", "CS2", "LOL", "DOTA2", "LEAGUESCUP"}
	for _, bf := range brandFixtureDirs {
		t.Run(bf.brand, func(t *testing.T) {
			snap, err := newReplayAdapterFrom(t, bf.dir, bf.brand, multiSportConfig).Discover(t.Context(), leagues)
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("discover errors: %v", snap.Errors)
			}
			requireLeagueHints(t, snap, "NBA", "NFL", "ATP_MIAMI", "DOTA2")
			sourcestest.AssertGolden(t, bf.dir, bf.brand+"_discover_multisport", snap)
		})
	}
}

func requireLeagueHints(t *testing.T, snap canon.Snapshot, leagues ...string) {
	t.Helper()
	for _, league := range leagues {
		if !slices.ContainsFunc(snap.Events, func(ev canon.SourceEvent) bool { return ev.LeagueHint == league }) {
			t.Fatalf("no event discovered for %s", league)
		}
	}
}

func TestAdapter_DiscoverNormalisesTennisNames(t *testing.T) {
	snap, err := newReplayAdapterFrom(t, fixtureDir, SportyBet, multiSportConfig).Discover(t.Context(), []string{"ATP_MIAMI"})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, ev := range snap.Events {
		if ev.Home == "" || ev.Away == "" || strings.Contains(ev.Home+ev.Away, ",") {
			t.Fatalf("tennis event names not normalised: %+v", ev)
		}
	}
	if snap.Events[0].Home != "Holger Rune" {
		t.Fatalf("first home = %q, want Holger Rune", snap.Events[0].Home)
	}
}

func TestAdapter_PriceMultiSportGolden(t *testing.T) {
	eventIDs := []string{"sr:match:73719372", "sr:match:71488628", "sr:match:75953170", "sr:match:75793260", "sr:match:75957208", "sr:match:74374512"}
	for _, bf := range brandFixtureDirs {
		t.Run(bf.brand, func(t *testing.T) {
			snap, err := newReplayAdapterFrom(t, bf.dir, bf.brand, sources.Config{}).Price(t.Context(), eventIDs)
			if err != nil {
				t.Fatalf("price: %v", err)
			}
			if len(snap.Errors) != 0 {
				t.Fatalf("price errors: %v", snap.Errors)
			}
			sourcestest.RequireMarkets(t, snap, canon.CodeQualify, canon.CodeMoneyline, canon.CodeSpread, canon.CodeTotal, canon.CodeSetHcp, canon.CodeTotalGames)
			sourcestest.RequireMarkets(t, snap, canon.PropPassYards, canon.PropPassTDs, canon.PropRushYards, canon.PropRecYards, canon.PropReceptions, canon.PropAnytimeTD)
			requirePeriods(t, snap, canon.PeriodFirstHalf, canon.PeriodQuarter1, canon.PeriodFirstSet)
			sourcestest.AssertGolden(t, bf.dir, bf.brand+"_price_multisport", snap)
		})
	}
}

func requirePeriods(t *testing.T, snap canon.Snapshot, periods ...string) {
	t.Helper()
	for _, period := range periods {
		if !slices.ContainsFunc(snap.Markets, func(m canon.RawMarket) bool { return m.Period == period }) {
			t.Fatalf("no market priced for period %s", period)
		}
	}
}

func TestAdapter_PriceDropsRegulationOnlyThreeWay(t *testing.T) {
	snap, err := newReplayAdapter(t, SportyBet).Price(t.Context(), []string{"sr:match:73719372"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	for _, m := range snap.Markets {
		if m.Code == canon.Code1X2 || m.Code == canon.CodeOU || m.Code == canon.Code1H1X2 {
			t.Fatalf("basketball market %s leaked a football code %s", m.SourceMarketID, m.Code)
		}
		if m.Rules.RegulationOnly {
			t.Fatalf("basketball market %s tagged regulation only", m.SourceMarketID)
		}
	}
}

func TestSportTree_TournamentIDsMatching(t *testing.T) {
	var resp envelope[sportTree]
	if err := json.Unmarshal(sourcestest.ReadFixture(t, fixtureDir, "tree_5.json"), &resp); err != nil {
		t.Fatalf("decode tree: %v", err)
	}
	tests := []struct {
		name     string
		match    tournamentName
		expected []string
	}{
		{name: "atp singles only", match: tournamentName{category: "ATP", contains: []string{"ATP Lyon", "Singles"}}, expected: []string{"sr:tournament:3097"}},
		{name: "challenger by city", match: tournamentName{category: "Challenger", contains: []string{"Cali", "Singles"}}, expected: []string{"sr:tournament:3861"}},
		{name: "wrong category", match: tournamentName{category: "WTA", contains: []string{"ATP Lyon"}}},
		{name: "absent tournament", match: tournamentName{category: "ATP", contains: []string{"ATP Miami", "Singles"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resp.Data.tournamentIDsMatching(tennisSportID, &tt.match)
			if !slices.Equal(got, tt.expected) {
				t.Fatalf("tournamentIDsMatching = %v, want %v", got, tt.expected)
			}
		})
	}
}

type codePeriod struct {
	code   canon.MarketCode
	period string
}

func pricedCodePeriods(t *testing.T, eventID string) (canon.Snapshot, map[codePeriod]bool) {
	t.Helper()
	snap, err := newReplayAdapter(t, SportyBet).Price(t.Context(), []string{eventID})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	got := map[codePeriod]bool{}
	for _, m := range snap.Markets {
		got[codePeriod{code: m.Code, period: m.Period}] = true
	}
	return snap, got
}

func TestAdapter_PriceFootballHalves(t *testing.T) {
	_, got := pricedCodePeriods(t, "sr:match:72723274")
	for _, period := range []string{canon.PeriodFirstHalf, canon.PeriodSecondHalf} {
		for _, code := range []canon.MarketCode{canon.CodeOU, canon.CodeAH, canon.CodeDC, canon.CodeDNB, canon.CodeBTTS, canon.CodeCS} {
			if !got[codePeriod{code: code, period: period}] {
				t.Fatalf("no %s market for period %s", code, period)
			}
		}
	}
	if !got[codePeriod{code: canon.CodeCornOU, period: canon.PeriodFirstHalf}] {
		t.Fatalf("no first half corners market")
	}
}

func TestAdapter_PriceBasketballPropsAndSecondHalf(t *testing.T) {
	snap, got := pricedCodePeriods(t, "sr:match:73719408")
	for _, want := range []codePeriod{
		{code: canon.PropPoints},
		{code: canon.PropBkAssists},
		{code: canon.CodeMoneyline, period: canon.PeriodSecondHalf},
		{code: canon.CodeSpread, period: canon.PeriodSecondHalf},
		{code: canon.CodeTotal, period: canon.PeriodSecondHalf},
	} {
		if !got[want] {
			t.Fatalf("no %s market for period %q", want.code, want.period)
		}
	}
	assertPlayerLinesUnique(t, snap.Markets)
}

func assertPlayerLinesUnique(t *testing.T, markets []canon.RawMarket) {
	t.Helper()
	seen := map[playerLineKey]string{}
	for _, m := range markets {
		key, ok := playerLineKeyOf(m)
		if !ok {
			continue
		}
		if first, dup := seen[key]; dup {
			t.Fatalf("player line %+v priced twice: %s and %s", key, first, m.SourceMarketID)
		}
		seen[key] = m.SourceMarketID
	}
}

func fixtureOutcomeOdds(t *testing.T, name string) map[string]float64 {
	t.Helper()
	var resp envelope[event]
	if err := json.Unmarshal(sourcestest.ReadFixture(t, fixtureDir, name), &resp); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	odds := map[string]float64{}
	for _, m := range resp.Data.Markets {
		for _, o := range m.Outcomes {
			price, err := strconv.ParseFloat(o.Odds, 64)
			if err != nil {
				t.Fatalf("odds %q of %s: %v", o.Odds, o.ID, err)
			}
			odds[m.sourceID()+"#"+o.ID] = price
		}
	}
	return odds
}

func TestAdapter_PricePlayerLaddersKeepEachPlayersRungs(t *testing.T) {
	snap, err := newReplayAdapter(t, SportyBet).Price(t.Context(), []string{"sr:match:66887112"})
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	assertPlayerLinesUnique(t, snap.Markets)
	odds := fixtureOutcomeOdds(t, "event_66887112.json")
	rungs := map[playerLineKey]float64{}
	for _, m := range snap.Markets {
		_, outcomeID, isRung := strings.Cut(m.SourceMarketID, "#")
		key, ok := playerLineKeyOf(m)
		if !isRung || !ok {
			continue
		}
		ref, ok := parsePlayerOutcomeID(outcomeID)
		if !ok || ref.playerID != key.playerID || key.line != float64(ref.atLeast)-0.5 {
			t.Fatalf("%s keyed as %+v, want player %s line %v", outcomeID, key, ref.playerID, float64(ref.atLeast)-0.5)
		}
		if got, want := m.Selections[0].Price.Raw, odds[m.SourceMarketID]; got != want {
			t.Fatalf("%s priced %v, want %v", m.SourceMarketID, got, want)
		}
		rungs[key] = m.Selections[0].Price.Raw
	}
	tests := []struct {
		name   string
		code   canon.MarketCode
		player string
		line   float64
		want   float64
	}{
		{name: "caio paulista 3+ shots", code: canon.PropShots, player: "sr:player:1672627", line: 2.5, want: 4.20},
		{name: "caio paulista 1+ shots on target", code: canon.PropSOT, player: "sr:player:1672627", line: 0.5, want: 2.70},
		{name: "bustos 2+ shots", code: canon.PropShots, player: "sr:player:1038623", line: 1.5, want: 4.25},
		{name: "villasanti 2+ shots with an identical ladder", code: canon.PropShots, player: "sr:player:1108419", line: 1.5, want: 4.25},
		{name: "vinicius ladder starting at 2+", code: canon.PropShots, player: "sr:player:1233356", line: 2.5, want: 1.35},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := rungs[playerLineKey{code: tt.code, playerID: tt.player, line: tt.line}]
			if !ok || got != tt.want {
				t.Fatalf("got %v (present %v), want %v", got, ok, tt.want)
			}
		})
	}
}
