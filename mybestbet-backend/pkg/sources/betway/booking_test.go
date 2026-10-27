package betway_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betway"
	"mybestbet/pkg/sources/sourcestest"
)

const (
	bookingPath    = "/Betting/BookABet"
	bookedEventID  = "72478646"
	bookingFixture = "booking_response.json"
	brandIDHeader  = "X-Brand-Id"
	nigeriaBrandID = "f8a8d16a-d619-4b49-aa8c-f21211403c92"
)

type postedOutcome struct {
	OutcomeID string  `json:"outcomeId"`
	EventID   int64   `json:"eventId"`
	MarketID  string  `json:"marketId"`
	Payment   int     `json:"payment"`
	Value     float64 `json:"value"`
	Selected  bool    `json:"selected"`
}

type postedBooking struct {
	CultureCode string          `json:"cultureCode"`
	CountryCode string          `json:"countryCode"`
	IsSingleBet bool            `json:"isSingleBet"`
	IsBuildABet bool            `json:"isBuildABet"`
	Outcomes    []postedOutcome `json:"outcomes"`
}

type bookingServer struct {
	mu       sync.Mutex
	posted   []postedBooking
	brandIDs []string
	status   int
}

func (b *bookingServer) handler(t *testing.T) http.Handler {
	replay := replayRoute(t)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != bookingPath {
			name, ok := replay(r)
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(sourcestest.ReadFixture(t, betway.Name, name))
			return
		}
		var posted postedBooking
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Errorf("decode posted booking: %v", err)
		}
		b.mu.Lock()
		b.posted = append(b.posted, posted)
		b.brandIDs = append(b.brandIDs, r.Header.Get(brandIDHeader))
		b.mu.Unlock()
		if b.status != 0 {
			w.WriteHeader(b.status)
			return
		}
		_, _ = w.Write(sourcestest.ReadFixture(t, betway.Name, bookingFixture))
	})
}

func newBooker(t *testing.T, server *bookingServer) sources.Booker {
	t.Helper()
	srv := httptest.NewServer(server.handler(t))
	t.Cleanup(srv.Close)
	cfg := sources.Config{Extra: map[string]string{"bookingUrl": srv.URL + bookingPath}}
	booker, ok := betway.New(sourcestest.Deps(srv, cfg)).(sources.Booker)
	if !ok {
		t.Fatal("betway adapter does not implement sources.Booker")
	}
	return booker
}

func TestBook(t *testing.T) {
	t.Parallel()
	server := &bookingServer{}
	legs := []sources.BookLeg{
		{SourceEventID: bookedEventID, Code: canon.Code1X2, Side: string(canon.SideHome)},
		{SourceEventID: bookedEventID, Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(2.5)},
		{SourceEventID: bookedEventID, Code: canon.PropAnytime, Side: string(canon.SideYes), PlayerName: "Lucas Boye"},
		{SourceEventID: bookedEventID, Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(11.5)},
		{SourceEventID: "1", Code: canon.Code1X2, Side: string(canon.SideHome)},
	}

	booking, err := newBooker(t, server).Book(t.Context(), legs)
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	if booking.Code != "2213045C" {
		t.Errorf("code = %q, want 2213045C", booking.Code)
	}
	if want := "https://www.betway.com.ng/?bookingCode=2213045C"; booking.WebURL != want {
		t.Errorf("web url = %q, want %q", booking.WebURL, want)
	}
	wantLegs := []sources.BookedLeg{
		{Index: 0, Price: 1.54, OK: true},
		{Index: 1, Price: 1.86, OK: true},
		{Index: 2, Price: 2.17, OK: true},
		{Index: 3, Reason: "selection not offered at betway"},
		{Index: 4, Reason: "event not available at betway"},
	}
	if len(booking.Legs) != len(wantLegs) {
		t.Fatalf("legs = %+v, want %+v", booking.Legs, wantLegs)
	}
	for i := range wantLegs {
		if booking.Legs[i] != wantLegs[i] {
			t.Errorf("leg %d = %+v, want %+v", i, booking.Legs[i], wantLegs[i])
		}
	}
	if len(server.posted) != 1 {
		t.Fatalf("booking calls = %d, want 1", len(server.posted))
	}
	if server.brandIDs[0] != nigeriaBrandID {
		t.Errorf("brand id = %q, want %q", server.brandIDs[0], nigeriaBrandID)
	}
	posted := server.posted[0]
	if posted.CountryCode != "NG" || posted.CultureCode != "en-US" || posted.IsSingleBet || posted.IsBuildABet {
		t.Errorf("posted envelope = %+v", posted)
	}
	wantOutcomes := []postedOutcome{
		{OutcomeID: "7247864611", EventID: 72478646, MarketID: "724786461", Payment: 1, Value: 100, Selected: true},
		{OutcomeID: "7247864618total=2.5~12", EventID: 72478646, MarketID: "7247864618", Payment: 1, Value: 100, Selected: true},
		{OutcomeID: "7247864640type=prematch~584066", EventID: 72478646, MarketID: "7247864640type=prematch~", Payment: 1, Value: 100, Selected: true},
	}
	if len(posted.Outcomes) != len(wantOutcomes) {
		t.Fatalf("posted outcomes = %+v, want %+v", posted.Outcomes, wantOutcomes)
	}
	for i := range wantOutcomes {
		if posted.Outcomes[i] != wantOutcomes[i] {
			t.Errorf("posted outcome %d = %+v, want %+v", i, posted.Outcomes[i], wantOutcomes[i])
		}
	}
}

func TestBookErrors(t *testing.T) {
	t.Parallel()
	bookable := []sources.BookLeg{{SourceEventID: bookedEventID, Code: canon.Code1X2, Side: string(canon.SideHome)}}
	tests := []struct {
		name    string
		status  int
		legs    []sources.BookLeg
		wantErr error
	}{
		{name: "login required", status: http.StatusUnauthorized, legs: bookable, wantErr: sources.ErrBookingUnsupported},
		{name: "forbidden", status: http.StatusForbidden, legs: bookable, wantErr: sources.ErrBookingBlocked},
		{name: "legal block", status: http.StatusUnavailableForLegalReasons, legs: bookable, wantErr: sources.ErrBookingBlocked},
		{
			name: "nothing bookable",
			legs: []sources.BookLeg{{SourceEventID: bookedEventID, Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(11.5)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := &bookingServer{status: tt.status}
			_, err := newBooker(t, server).Book(t.Context(), tt.legs)
			if err == nil {
				t.Fatal("book succeeded, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
