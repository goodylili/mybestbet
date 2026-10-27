package betpawa_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betpawa"
	"mybestbet/pkg/sources/sourcestest"
)

const listVersionPath = "/api/sportsbook/v4"

type bookingServer struct {
	mu     sync.Mutex
	bodies []string
}

func (s *bookingServer) posted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.bodies)
}

func newBookingAdapter(t *testing.T) (sources.Booker, *bookingServer) {
	t.Helper()
	server := &bookingServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/sportsbook/v3/booking-number" {
			body, _ := io.ReadAll(r.Body)
			server.mu.Lock()
			server.bodies = append(server.bodies, string(body))
			server.mu.Unlock()
			_, _ = w.Write(sourcestest.ReadFixture(t, betpawa.Name, "booking_NWB75OF.json"))
			return
		}
		eventID, ok := strings.CutPrefix(r.URL.Path, listVersionPath+"/events/")
		if !ok || eventID != "38781890" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(sourcestest.ReadFixture(t, betpawa.Name, "event_"+eventID+".json"))
	}))
	t.Cleanup(srv.Close)
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Config.BaseURL = srv.URL + listVersionPath
	return betpawa.New(deps).(sources.Booker), server
}

func TestBook(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t)
	legs := []sources.BookLeg{
		{SourceEventID: "38781890", Code: canon.Code1X2, Side: string(canon.SideHome)},
		{SourceEventID: "38781890", Code: canon.CodeOU, Side: string(canon.SideUnder), Line: canon.Line(42.5)},
		{SourceEventID: "1", Code: canon.Code1X2, Side: string(canon.SideHome)},
	}
	booking, err := booker.Book(t.Context(), legs)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	want := []string{`{"selections":{"selections":[{"type":"SINGLE","selections":[1573825980]}]}}`}
	if got := server.posted(); !slices.Equal(got, want) {
		t.Fatalf("posted = %q, want %q", got, want)
	}
	if booking.Code != "NWB75OF" || !strings.HasSuffix(booking.WebURL, "/?bookingCode=NWB75OF") {
		t.Errorf("code = %q web url = %q", booking.Code, booking.WebURL)
	}
	wantLegs := []sources.BookedLeg{
		{Index: 0, Price: 1.83, OK: true},
		{Index: 1, Reason: sources.ErrLegNotOffered.Error()},
		{Index: 2, Reason: sources.ErrLegEventMissing.Error()},
	}
	if !slices.Equal(booking.Legs, wantLegs) {
		t.Errorf("legs = %+v, want %+v", booking.Legs, wantLegs)
	}
}

func TestBookNothingBookable(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t)
	_, err := booker.Book(t.Context(), []sources.BookLeg{{SourceEventID: "2", Code: canon.Code1X2, Side: string(canon.SideHome)}})
	if !errors.Is(err, sources.ErrNothingBookable) {
		t.Fatalf("book error = %v, want ErrNothingBookable", err)
	}
	if n := len(server.posted()); n != 0 {
		t.Errorf("booking endpoint called %d times, want 0", n)
	}
}
