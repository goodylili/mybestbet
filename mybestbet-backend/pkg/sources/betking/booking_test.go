package betking_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betking"
	"mybestbet/pkg/sources/sourcestest"
)

const bettingPath = "/api/sports/v1/bet"

type bettingServer struct {
	mu       sync.Mutex
	created  []string
	bookings []string
	coupons  []string
}

func (s *bettingServer) record(target *[]string, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*target = append(*target, value)
}

func (s *bettingServer) snapshot() (created, bookings, coupons []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.created), slices.Clone(s.bookings), slices.Clone(s.coupons)
}

func newBookingAdapter(t *testing.T) (sources.Booker, *bettingServer) {
	t.Helper()
	server := &bettingServer{}
	feeds := replayRoute(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, betting := strings.CutPrefix(r.URL.Path, bettingPath)
		if !betting {
			name, ok := feeds(r)
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(sourcestest.ReadFixture(t, betking.Name, name))
			return
		}
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPost && path == "/create/language/en":
			server.record(&server.created, string(body))
			_, _ = w.Write(sourcestest.ReadFixture(t, betking.Name, "coupon_create_2510403257.json"))
		case r.Method == http.MethodPost && strings.HasPrefix(path, "/Book/"):
			server.record(&server.bookings, strings.TrimPrefix(path, "/Book/"))
			server.record(&server.coupons, string(body))
			_, _ = w.Write(sourcestest.ReadFixture(t, betking.Name, "coupon_book_RN28X5.json"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := sources.Config{Extra: map[string]string{"bettingUrl": srv.URL + bettingPath}}
	return betking.New(sourcestest.Deps(srv, cfg)).(sources.Booker), server
}

func TestBook(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t)
	legs := []sources.BookLeg{
		{SourceEventID: "1005767843", Code: canon.Code1X2, Side: string(canon.SideHome)},
		{SourceEventID: "1005767843", Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(42.5)},
	}
	booking, err := booker.Book(t.Context(), legs)
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	created, bookings, coupons := server.snapshot()
	wantCreate := []string{`{"Stake":null,"Groupings":null,"Selections":[{"SelectionId":"2510403257"}]}`}
	if !slices.Equal(created, wantCreate) {
		t.Fatalf("create bodies = %q, want %q", created, wantCreate)
	}
	if len(bookings) != 1 || len(bookings[0]) != 20 || len(coupons) != 1 {
		t.Fatalf("bookings = %q coupons = %d, want one booking with a 20 character transaction id", bookings, len(coupons))
	}
	var coupon struct {
		Odds []struct {
			SelectionID int64 `json:"SelectionId"`
		} `json:"Odds"`
	}
	if err := json.Unmarshal([]byte(coupons[0]), &coupon); err != nil || len(coupon.Odds) != 1 || coupon.Odds[0].SelectionID != 2510403257 {
		t.Fatalf("booked coupon = %s (err %v), want the created coupon forwarded", coupons[0], err)
	}
	if booking.Code != "RN28X5" || booking.WebURL != "https://m.betking.com/en-ng/sports/book-bet/RN28X5" {
		t.Errorf("code = %q web url = %q", booking.Code, booking.WebURL)
	}
	wantLegs := []sources.BookedLeg{
		{Index: 0, Price: 1.79, OK: true},
		{Index: 1, Reason: sources.ErrLegNotOffered.Error()},
	}
	if !slices.Equal(booking.Legs, wantLegs) {
		t.Errorf("legs = %+v, want %+v", booking.Legs, wantLegs)
	}
}

func TestBookReusesTransactionIDOnReplay(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t)
	legs := []sources.BookLeg{{SourceEventID: "1005767843", Code: canon.Code1X2, Side: string(canon.SideHome)}}
	for range 2 {
		if _, err := booker.Book(t.Context(), legs); err != nil {
			t.Fatalf("book: %v", err)
		}
	}
	_, bookings, _ := server.snapshot()
	if len(bookings) != 2 || bookings[0] != bookings[1] {
		t.Fatalf("transaction ids = %q, want the same id twice", bookings)
	}
}

func TestBookNothingBookable(t *testing.T) {
	t.Parallel()
	booker, server := newBookingAdapter(t)
	_, err := booker.Book(t.Context(), []sources.BookLeg{{SourceEventID: "1005767843", Code: canon.CodeCornOU, Side: string(canon.SideOver), Line: canon.Line(99.5)}})
	if !errors.Is(err, sources.ErrNothingBookable) {
		t.Fatalf("book error = %v, want ErrNothingBookable", err)
	}
	if created, _, _ := server.snapshot(); len(created) != 0 {
		t.Errorf("create endpoint called %d times, want 0", len(created))
	}
}
