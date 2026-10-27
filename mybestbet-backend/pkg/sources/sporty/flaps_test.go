package sporty

import (
	"testing"
	"time"

	"mybestbet/pkg/canon"
)

func ladderRung(raw float64, at time.Time) canon.RawMarket {
	return canon.RawMarket{
		EventSourceID:  "sr:match:72723282",
		Code:           canon.PropShots,
		Line:           canon.Line(2.5),
		Player:         &canon.SourcePlayer{SourceID: "sr:player:1117889"},
		SourceMarketID: "776@variant=pre:playerprops:72723282:1117889#pre:playerprops:72723282:1117889:3",
		Selections:     []canon.Selection{{Side: canon.SideOver, Price: canon.NewPrice(raw, canon.FormDecimal, at)}},
	}
}

func TestFlapGuard_SuspendFlapping(t *testing.T) {
	start := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		polls     []float64
		step      time.Duration
		suspended []bool
	}{
		{name: "steady quote stays live", polls: []float64{4.6, 4.6, 4.6}, step: time.Minute, suspended: []bool{false, false, false}},
		{name: "one-way move stays live", polls: []float64{4.6, 5.0, 5.5}, step: time.Minute, suspended: []bool{false, false, false}},
		{name: "quote flapping between two ladders is suspended", polls: []float64{4.6, 8.3, 4.6, 8.3, 4.6}, step: time.Minute, suspended: []bool{false, false, true, true, true}},
		{name: "flapped quote that settles stays suspended for the hold", polls: []float64{4.6, 8.3, 4.6, 4.6}, step: time.Minute, suspended: []bool{false, false, true, true}},
		{name: "flapped quote goes live again after the hold", polls: []float64{4.6, 8.3, 4.6, 4.6}, step: flapHold, suspended: []bool{false, false, true, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			guard := newFlapGuard()
			for i, raw := range tt.polls {
				at := start.Add(time.Duration(i) * tt.step)
				markets := []canon.RawMarket{ladderRung(raw, at)}
				guard.suspendFlapping(markets, at)
				if got := markets[0].Selections[0].Price.Suspended; got != tt.suspended[i] {
					t.Fatalf("poll %d (%v): suspended %v, want %v", i, raw, got, tt.suspended[i])
				}
			}
		})
	}
}

func TestFlapGuard_IgnoresMatchMarketsAndOtherRungs(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC)
	guard := newFlapGuard()
	var last []canon.RawMarket
	for i, raw := range []float64{1.9, 2.1, 1.9} {
		at := start.Add(time.Duration(i) * time.Minute)
		match := canon.RawMarket{EventSourceID: "sr:match:72723282", Code: canon.CodeOU, Line: canon.Line(2.5), SourceMarketID: "18@total=2.5",
			Selections: []canon.Selection{{Side: canon.SideOver, Price: canon.NewPrice(raw, canon.FormDecimal, at)}}}
		other := ladderRung(4.6, at)
		other.SourceMarketID += "-other"
		last = []canon.RawMarket{match, other}
		guard.suspendFlapping(last, at)
	}
	for _, m := range last {
		if m.Selections[0].Price.Suspended {
			t.Fatalf("%s suspended, want live", m.SourceMarketID)
		}
	}
}
