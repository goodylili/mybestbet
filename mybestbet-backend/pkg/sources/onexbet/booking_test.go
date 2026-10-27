package onexbet

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const (
	bookingEventID   = "760398125"
	firstHalfGameID  = 760398126
	playersGameID    = 763696273
	bookingKickoff   = 1792782000
	savedCouponCode  = "F2NPX"
	savedOKFixture   = "savecoupon_ok.json"
	callFailedReplay = "savecoupon_call_failed.json"
)

type couponServer struct {
	mu        sync.Mutex
	replies   []string
	requests  []saveCouponRequest
	forbidden bool
}

func (cs *couponServer) handler(t *testing.T, partner string) http.Handler {
	t.Helper()
	feed := replayRoute(recordedFixtures(t), partner)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cs.forbidden {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		name, ok := feed(r)
		if r.Method == http.MethodPost && r.URL.Path == saveCouponPath {
			name, ok = cs.record(t, r), true
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(sourcestest.ReadFixture(t, fixtureDir, name))
	})
}

func (cs *couponServer) record(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read save coupon body: %v", err)
	}
	var req saveCouponRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Errorf("decode save coupon body: %v", err)
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.requests = append(cs.requests, req)
	reply := savedOKFixture
	if len(cs.replies) > 0 {
		reply, cs.replies = cs.replies[0], cs.replies[1:]
	}
	return reply
}

func newBookingAdapter(t *testing.T, b brand, cs *couponServer) *Adapter {
	t.Helper()
	srv := httptest.NewServer(cs.handler(t, b.defaultPartner))
	t.Cleanup(srv.Close)
	adapter, ok := b.factory()(sourcestest.Deps(srv, sources.Config{})).(*Adapter)
	if !ok {
		t.Fatalf("factory for %s did not build *Adapter", b.name)
	}
	adapter.retryDelay = 0
	return adapter
}

func bookingLegs() []sources.BookLeg {
	return []sources.BookLeg{
		{SourceEventID: bookingEventID, Code: canon.Code1X2, Side: string(canon.SideAway)},
		{SourceEventID: bookingEventID, Code: canon.Code1H1X2, Period: canon.PeriodFirstHalf, Side: string(canon.SideHome)},
		{SourceEventID: bookingEventID, Code: canon.CodeDNB, Side: string(canon.SideHome)},
		{SourceEventID: bookingEventID, Code: canon.CodeCS, Label: "2-1"},
		{SourceEventID: bookingEventID, Code: canon.CodeOU, Side: string(canon.SideUnder), Line: canon.Line(1.5)},
		{SourceEventID: bookingEventID, Code: canon.PropAnytime, Side: string(canon.SideYes), PlayerName: "C. Wood"},
		{SourceEventID: bookingEventID, Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(9.5)},
		{SourceEventID: "1", Code: canon.Code1X2, Side: string(canon.SideHome)},
	}
}

func TestAdapter_Book(t *testing.T) {
	wantEvents := []couponEvent{
		{GameID: 760398125, Type: 3, Coef: 2.232, Kind: prematchEventKind},
		{GameID: firstHalfGameID, Type: 1, Coef: 3.74, Kind: prematchEventKind},
		{GameID: 760398125, Type: 7, Coef: 2.28, Kind: prematchEventKind},
		{GameID: 760398125, Type: 731, Coef: 9.5, Param: 2.001, Kind: prematchEventKind},
		{GameID: 760398125, Type: 10, Coef: 3.925, Param: 1.5, Kind: prematchEventKind},
		{GameID: playersGameID, Type: 5869, Coef: 2.875, Param: 0.5, PlayerID: 2282381, Kind: prematchEventKind},
	}
	wantLegs := []sources.BookedLeg{
		{Index: 0, Price: 2.232, OK: true},
		{Index: 1, Price: 3.74, OK: true},
		{Index: 2, Price: 2.28, OK: true},
		{Index: 3, Price: 9.5, OK: true},
		{Index: 4, Price: 3.925, OK: true},
		{Index: 5, Price: 2.875, OK: true},
		{Index: 6, Reason: reasonSelectionMissing},
		{Index: 7, Reason: reasonEventNotListed},
	}
	for _, b := range brands {
		t.Run(b.name, func(t *testing.T) {
			cs := &couponServer{}
			booking, err := newBookingAdapter(t, b, cs).Book(t.Context(), bookingLegs())
			if err != nil {
				t.Fatalf("book: %v", err)
			}
			if booking.Code != savedCouponCode {
				t.Fatalf("code = %q, want %q", booking.Code, savedCouponCode)
			}
			if want := b.webURL + "/?coupon-code=" + savedCouponCode; booking.WebURL != want {
				t.Fatalf("web url = %q, want %q", booking.WebURL, want)
			}
			if want := time.Unix(bookingKickoff, 0).UTC(); !booking.ExpiresAt.Equal(want) {
				t.Fatalf("expires = %v, want %v", booking.ExpiresAt, want)
			}
			if !slices.Equal(booking.Legs, wantLegs) {
				t.Fatalf("legs = %+v, want %+v", booking.Legs, wantLegs)
			}
			if len(cs.requests) != 1 {
				t.Fatalf("save coupon calls = %d, want 1", len(cs.requests))
			}
			sent := cs.requests[0]
			if sent.Vid != accumulatorCouponVid || sent.Lng != couponLanguage || !sent.NotWait || sent.CheckCf != acceptOddsChanges {
				t.Fatalf("coupon header = %+v", sent)
			}
			if got := sent.Partner; got != mustAtoi(t, b.defaultPartner) {
				t.Fatalf("partner = %d, want %s", got, b.defaultPartner)
			}
			if !slices.Equal(sent.Events, wantEvents) {
				t.Fatalf("events = %+v, want %+v", sent.Events, wantEvents)
			}
		})
	}
}

func TestAdapter_BookSingleLegIsSingleCoupon(t *testing.T) {
	cs := &couponServer{}
	legs := []sources.BookLeg{{SourceEventID: bookingEventID, Code: canon.CodeBTTS, Side: string(canon.SideNo)}}
	if _, err := newBookingAdapter(t, brands[1], cs).Book(t.Context(), legs); err != nil {
		t.Fatalf("book: %v", err)
	}
	want := []couponEvent{{GameID: 760398125, Type: 181, Coef: 2.22, Kind: prematchEventKind}}
	if sent := cs.requests[0]; sent.Vid != singleCouponVid || !slices.Equal(sent.Events, want) {
		t.Fatalf("coupon = %+v, want single with %+v", sent, want)
	}
}

func TestAdapter_BookRetriesCallFailed(t *testing.T) {
	tests := []struct {
		name      string
		replies   []string
		wantCalls int
		wantErr   error
	}{
		{name: "recovers on retry", replies: []string{callFailedReplay}, wantCalls: 2},
		{name: "gives up after attempts", replies: []string{callFailedReplay, callFailedReplay, callFailedReplay}, wantCalls: saveCouponAttempts, wantErr: errCallFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &couponServer{replies: tt.replies}
			booking, err := newBookingAdapter(t, brands[1], cs).Book(t.Context(), bookingLegs()[:1])
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("book error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && booking.Code != savedCouponCode {
				t.Fatalf("code = %q, want %q", booking.Code, savedCouponCode)
			}
			if len(cs.requests) != tt.wantCalls {
				t.Fatalf("save coupon calls = %d, want %d", len(cs.requests), tt.wantCalls)
			}
		})
	}
}

func TestAdapter_BookNothingBookable(t *testing.T) {
	tests := []struct {
		name      string
		forbidden bool
		legs      []sources.BookLeg
		wantErr   error
	}{
		{name: "no matching selection", legs: bookingLegs()[6:7], wantErr: errNoBookableLegs},
		{name: "unknown event", legs: bookingLegs()[7:], wantErr: errNoBookableLegs},
		{name: "blocked by the book", forbidden: true, legs: bookingLegs()[:2], wantErr: sources.ErrBookingBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := &couponServer{forbidden: tt.forbidden}
			_, err := newBookingAdapter(t, brands[0], cs).Book(t.Context(), tt.legs)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("book error = %v, want %v", err, tt.wantErr)
			}
			if len(cs.requests) != 0 {
				t.Fatalf("save coupon called %d times for an unbookable slip", len(cs.requests))
			}
		})
	}
}

func TestSamePersonName(t *testing.T) {
	tests := []struct {
		offered, wanted string
		expected        bool
	}{
		{offered: "Chris Wood", wanted: "Chris Wood", expected: true},
		{offered: "Chris Wood", wanted: "C. Wood", expected: true},
		{offered: "Sinner J.", wanted: "Jannik Sinner", expected: true},
		{offered: "Erling Haaland", wanted: "Haaland", expected: true},
		{offered: "Chris Wood", wanted: "Callum Wilson", expected: false},
		{offered: "Chris Wood", wanted: "D. Wood", expected: false},
		{offered: "J.", wanted: "Jannik Sinner", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.offered+" vs "+tt.wanted, func(t *testing.T) {
			if got := samePersonName(tt.offered, tt.wanted); got != tt.expected {
				t.Fatalf("samePersonName(%q, %q) = %v, want %v", tt.offered, tt.wanted, got, tt.expected)
			}
		})
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return n
}
