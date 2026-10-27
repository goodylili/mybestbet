package sources_test

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"testing"
	"time"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

var observedAt = time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)

func decimal(raw float64, suspended bool) canon.Price {
	p := canon.NewPrice(raw, canon.FormDecimal, observedAt)
	p.Suspended = suspended
	return p
}

var outcomes = map[string][]canon.RawMarket{
	"ou-over-2.5": {{Code: canon.CodeOU, Line: canon.Line(2.5), Selections: []canon.Selection{{Side: canon.SideOver, Price: decimal(1.9, false)}}}},
	"btts-yes":    {{Code: canon.CodeBTTS, Selections: []canon.Selection{{Side: canon.SideYes, Price: decimal(1.7, true)}}}},
	"dc-1x-1h":    {{Code: canon.CodeDC, Period: canon.PeriodFirstHalf, Selections: []canon.Selection{{Label: "1X", Price: decimal(1.3, false)}}}},
	"anytime-saka": {{
		Code:       canon.PropAnytime,
		Player:     &canon.SourcePlayer{SourceID: "7", Name: "Bukayo Saka"},
		Selections: []canon.Selection{{Side: canon.SideYes, Price: decimal(3.1, false)}},
	}},
}

func TestPickLegOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		leg     sources.BookLeg
		wantRef string
		wantErr error
	}{
		{name: "line within tolerance", leg: sources.BookLeg{Code: canon.CodeOU, Side: "over", Line: canon.Line(2.5000001)}, wantRef: "ou-over-2.5"},
		{name: "label with period", leg: sources.BookLeg{Code: canon.CodeDC, Period: canon.PeriodFirstHalf, Label: "1X"}, wantRef: "dc-1x-1h"},
		{name: "player name ignores case", leg: sources.BookLeg{Code: canon.PropAnytime, Side: "yes", PlayerName: " bukayo saka"}, wantRef: "anytime-saka"},
		{name: "suspended", leg: sources.BookLeg{Code: canon.CodeBTTS, Side: "yes"}, wantErr: sources.ErrLegSuspended},
		{name: "wrong line", leg: sources.BookLeg{Code: canon.CodeOU, Side: "over", Line: canon.Line(3.5)}, wantErr: sources.ErrLegNotOffered},
		{name: "line required", leg: sources.BookLeg{Code: canon.CodeOU, Side: "over"}, wantErr: sources.ErrLegNotOffered},
		{name: "wrong period", leg: sources.BookLeg{Code: canon.CodeDC, Label: "1X"}, wantErr: sources.ErrLegNotOffered},
		{name: "player missing", leg: sources.BookLeg{Code: canon.PropAnytime, Side: "yes"}, wantErr: sources.ErrLegNotOffered},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ref, _, err := sources.PickLegOutcome(maps.All(outcomes), tt.leg)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if ref != tt.wantRef {
				t.Fatalf("ref = %q, want %q", ref, tt.wantRef)
			}
		})
	}
}

func TestResolveLegs(t *testing.T) {
	t.Parallel()
	fetches := map[string]int{}
	fetch := func(_ context.Context, id string) (map[string][]canon.RawMarket, error) {
		fetches[id]++
		switch id {
		case "gone":
			return nil, &sources.StatusError{Method: http.MethodGet, URL: "/e/gone", Status: http.StatusNotFound}
		case "down":
			return nil, &sources.StatusError{Method: http.MethodGet, URL: "/e/down", Status: http.StatusBadGateway}
		}
		return outcomes, nil
	}
	pick := func(event map[string][]canon.RawMarket, leg sources.BookLeg) (string, float64, error) {
		return sources.PickLegOutcome(maps.All(event), leg)
	}
	legs := []sources.BookLeg{
		{SourceEventID: "e1", Code: canon.CodeOU, Side: "over", Line: canon.Line(2.5)},
		{SourceEventID: "e1", Code: canon.CodeBTTS, Side: "yes"},
		{SourceEventID: "gone", Code: canon.CodeOU, Side: "over", Line: canon.Line(2.5)},
		{SourceEventID: "gone", Code: canon.CodeBTTS, Side: "yes"},
		{SourceEventID: "down", Code: canon.CodeOU, Side: "over", Line: canon.Line(2.5)},
		{SourceEventID: "e1", Code: canon.CodeDC, Period: canon.PeriodFirstHalf, Label: "1X"},
	}
	resolved, err := sources.ResolveLegs(t.Context(), legs, fetch, pick)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantRefs := []string{"ou-over-2.5", "dc-1x-1h"}
	if len(resolved.Refs) != 2 || resolved.Refs[0] != wantRefs[0] || resolved.Refs[1] != wantRefs[1] {
		t.Fatalf("refs = %v, want %v", resolved.Refs, wantRefs)
	}
	wantLegs := []sources.BookedLeg{
		{Index: 0, Price: 1.9, OK: true},
		{Index: 1, Reason: "selection suspended"},
		{Index: 2, Reason: "event not found"},
		{Index: 3, Reason: "event not found"},
		{Index: 4, Reason: "event unavailable"},
		{Index: 5, Price: 1.3, OK: true},
	}
	for i, want := range wantLegs {
		if resolved.Legs[i] != want {
			t.Errorf("leg %d = %+v, want %+v", i, resolved.Legs[i], want)
		}
	}
	if fetches["e1"] != 1 || fetches["gone"] != 1 {
		t.Errorf("fetches = %v, want each event fetched once", fetches)
	}
	resolved.Reject(1, "selection suspended")
	if resolved.Legs[5].OK || resolved.Legs[5].Reason != "selection suspended" {
		t.Errorf("rejected leg = %+v, want leg 5 rejected", resolved.Legs[5])
	}
}

func TestResolveLegsNothingBookable(t *testing.T) {
	t.Parallel()
	fetch := func(context.Context, string) (map[string][]canon.RawMarket, error) { return outcomes, nil }
	pick := func(event map[string][]canon.RawMarket, leg sources.BookLeg) (string, float64, error) {
		return sources.PickLegOutcome(maps.All(event), leg)
	}
	resolved, err := sources.ResolveLegs(t.Context(), []sources.BookLeg{{Code: canon.CodeBTTS, Side: "yes"}}, fetch, pick)
	if !errors.Is(err, sources.ErrNothingBookable) || !errors.Is(err, sources.ErrLegSuspended) {
		t.Fatalf("err = %v, want ErrNothingBookable wrapping ErrLegSuspended", err)
	}
	if len(resolved.Legs) != 1 || resolved.Legs[0].OK {
		t.Fatalf("legs = %+v, want one rejected leg", resolved.Legs)
	}
}

func TestBookingFailure(t *testing.T) {
	t.Parallel()
	forbidden := &sources.StatusError{Method: http.MethodPost, URL: "/share", Status: http.StatusForbidden}
	if err := sources.BookingFailure(forbidden); !errors.Is(err, sources.ErrBookingBlocked) || !errors.Is(err, sources.ErrForbidden) {
		t.Errorf("forbidden = %v, want ErrBookingBlocked", err)
	}
	throttled := &sources.StatusError{Method: http.MethodPost, URL: "/share", Status: http.StatusTooManyRequests}
	if err := sources.BookingFailure(throttled); errors.Is(err, sources.ErrBookingBlocked) {
		t.Errorf("throttled = %v, want not blocked", err)
	}
}
