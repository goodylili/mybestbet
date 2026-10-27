package sporty

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const countryPath = "/api/ng"

type shareServer struct {
	mu     sync.Mutex
	shares [][]shareSelection
}

func (s *shareServer) posted() [][]shareSelection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shares
}

func newBookingAdapter(t *testing.T, dir, brand, shareFixture string) (sources.Booker, *shareServer) {
	t.Helper()
	recorded := recordedFixtures(t, dir)
	server := &shareServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, countryPath)
		name := ""
		switch {
		case path == "/factsCenter/event":
			name = "event_" + strings.TrimPrefix(r.URL.Query().Get("eventId"), "sr:match:") + ".json"
		case path == sharePath && r.Method == http.MethodPost:
			var request shareRequest
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			server.mu.Lock()
			server.shares = append(server.shares, request.Selections)
			server.mu.Unlock()
			name = shareFixture
		}
		if !recorded[name] {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(sourcestest.ReadFixture(t, dir, name))
	}))
	t.Cleanup(srv.Close)
	deps := sourcestest.Deps(srv, sources.Config{})
	deps.Config.BaseURL = srv.URL + countryPath
	return Factory(brand)(deps).(sources.Booker), server
}

func TestAdapter_BookSportyBet(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t, fixtureDir, SportyBet, "share_SNK074.json")
	legs := []sources.BookLeg{
		{SourceEventID: "sr:match:72221358", Code: canon.Code1X2, Side: string(canon.SideHome)},
		{SourceEventID: "sr:match:72221358", Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(9.5)},
		{SourceEventID: "sr:match:1", Code: canon.Code1X2, Side: string(canon.SideAway)},
	}
	booking, err := booker.Book(t.Context(), legs)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	want := [][]shareSelection{{{EventID: "sr:match:72221358", MarketID: "1", OutcomeID: "1"}}}
	if got := server.posted(); !slices.EqualFunc(got, want, slices.Equal[[]shareSelection]) {
		t.Fatalf("posted selections = %+v, want %+v", got, want)
	}
	if booking.Code != "SNK074" {
		t.Errorf("code = %q, want SNK074", booking.Code)
	}
	if !strings.HasSuffix(booking.WebURL, "/ng/?shareCode=SNK074&c=ng") {
		t.Errorf("web url = %q", booking.WebURL)
	}
	if want := time.UnixMilli(1794313800000).UTC(); !booking.ExpiresAt.Equal(want) {
		t.Errorf("expires at = %v, want %v", booking.ExpiresAt, want)
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

func TestAdapter_BookFootballComLineMarket(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t, FootballCom, FootballCom, "share_SLMB7Z.json")
	legs := []sources.BookLeg{{SourceEventID: "sr:match:72478678", Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(2.5)}}
	booking, err := booker.Book(t.Context(), legs)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	want := [][]shareSelection{{{EventID: "sr:match:72478678", MarketID: "18", Specifier: "total=2.5", OutcomeID: "12"}}}
	if got := server.posted(); !slices.EqualFunc(got, want, slices.Equal[[]shareSelection]) {
		t.Fatalf("posted selections = %+v, want %+v", got, want)
	}
	if booking.Code != "SLMB7Z" || !strings.HasSuffix(booking.WebURL, "/ng/m?shareCode=SLMB7Z") {
		t.Errorf("code = %q web url = %q", booking.Code, booking.WebURL)
	}
	if !booking.ExpiresAt.IsZero() {
		t.Errorf("expires at = %v, want zero without a deadline", booking.ExpiresAt)
	}
	if len(booking.Legs) != 1 || !booking.Legs[0].OK || booking.Legs[0].Price != 1.51 {
		t.Errorf("legs = %+v, want one bookable leg at 1.51", booking.Legs)
	}
}

func TestAdapter_BookNothingBookable(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t, fixtureDir, SportyBet, "share_SNK074.json")
	legs := []sources.BookLeg{{SourceEventID: "sr:match:72221358", Code: canon.CodeBTTS, Side: string(canon.SideYes), Period: canon.PeriodFirstHalf}}
	booking, err := booker.Book(t.Context(), legs)
	if !errors.Is(err, sources.ErrNothingBookable) {
		t.Fatalf("book error = %v, want ErrNothingBookable", err)
	}
	if len(server.posted()) != 0 {
		t.Errorf("share endpoint called %d times, want 0", len(server.posted()))
	}
	if len(booking.Legs) != 1 || booking.Legs[0].OK {
		t.Errorf("legs = %+v, want one rejected leg", booking.Legs)
	}
}
