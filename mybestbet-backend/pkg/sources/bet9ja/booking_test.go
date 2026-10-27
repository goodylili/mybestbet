package bet9ja

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	json "github.com/goccy/go-json"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/sourcestest"
)

const (
	bookingFixture = "booking_5XFHSN6.json"
	bookedEventID  = "832843044"
	bookingPath    = "/BookABetV2"
)

type bookingServer struct {
	mu       sync.Mutex
	slips    []betslip
	status   int
	response []byte
}

func (b *bookingServer) handler(t *testing.T) http.Handler {
	recorded := recordedFixtures(t)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == bookingPath {
			b.serveBooking(t, w, r)
			return
		}
		name, ok := replayRoute(recorded)(r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(sourcestest.ReadFixture(t, Name, name))
	})
}

func (b *bookingServer) serveBooking(t *testing.T, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		t.Errorf("parse booking form: %v", err)
	}
	var slip betslip
	if err := json.Unmarshal([]byte(r.PostForm.Get(betslipField)), &slip); err != nil {
		t.Errorf("decode posted betslip: %v", err)
	}
	b.mu.Lock()
	b.slips = append(b.slips, slip)
	b.mu.Unlock()
	if b.status != 0 {
		w.WriteHeader(b.status)
		return
	}
	_, _ = w.Write(b.response)
}

func newBookingAdapter(t *testing.T, server *bookingServer) *Adapter {
	t.Helper()
	srv := httptest.NewServer(server.handler(t))
	t.Cleanup(srv.Close)
	cfg := sources.Config{Extra: map[string]string{bookingURLExtra: srv.URL + bookingPath}}
	adapter, ok := New(sourcestest.Deps(srv, cfg)).(*Adapter)
	if !ok {
		t.Fatal("bet9ja adapter has unexpected type")
	}
	return adapter
}

func TestAdapter_Book(t *testing.T) {
	t.Parallel()
	server := &bookingServer{response: sourcestest.ReadFixture(t, Name, bookingFixture)}
	adapter := newBookingAdapter(t, server)
	legs := []sources.BookLeg{
		{SourceEventID: bookedEventID, Code: canon.Code1X2, Side: string(canon.SideAway)},
		{SourceEventID: bookedEventID, Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(9.5)},
		{SourceEventID: "1", Code: canon.Code1X2, Side: string(canon.SideHome)},
		{SourceEventID: bookedEventID, Code: canon.PropAnytime, Side: string(canon.SideYes), PlayerName: "Jonathan Burkardt"},
		{SourceEventID: bookedEventID, Code: canon.Code1X2, Side: string(canon.SideAway)},
	}

	booking, err := adapter.Book(t.Context(), legs)
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	if booking.Code != "5XFHSN6" {
		t.Errorf("code = %q, want 5XFHSN6", booking.Code)
	}
	if want := "https://sports.bet9ja.com/?bookABetCode=5XFHSN6"; booking.WebURL != want {
		t.Errorf("web url = %q, want %q", booking.WebURL, want)
	}
	want := []sources.BookedLeg{
		{Index: 0, Price: 1.7, OK: true},
		{Index: 1, Reason: reasonNotOffered},
		{Index: 2, Reason: reasonEventUnavailable},
		{Index: 3, Reason: reasonPlayerProp},
		{Index: 4, Reason: reasonSameEvent},
	}
	if len(booking.Legs) != len(want) {
		t.Fatalf("legs = %+v, want %+v", booking.Legs, want)
	}
	for i := range want {
		if booking.Legs[i] != want[i] {
			t.Errorf("leg %d = %+v, want %+v", i, booking.Legs[i], want[i])
		}
	}
	if len(server.slips) != 1 {
		t.Fatalf("booking calls = %d, want 1", len(server.slips))
	}
	wantEvent := betslipEvent{EventID: bookedEventID, Market: "S_1X2", Sign: "2", SID: "S_1X2_2", ID: "832843044$S_1X2_2"}
	slip := server.slips[0]
	if got := slip.Events[wantEvent.ID]; got != wantEvent {
		t.Errorf("posted event = %+v, want %+v", got, wantEvent)
	}
	if got := slip.Bets[0].Odds[wantEvent.ID]; got != 1.7 {
		t.Errorf("posted odds = %v, want 1.7", got)
	}
}

func TestAdapter_BookLinedSelection(t *testing.T) {
	t.Parallel()
	server := &bookingServer{response: sourcestest.ReadFixture(t, Name, bookingFixture)}
	adapter := newBookingAdapter(t, server)
	legs := []sources.BookLeg{
		{SourceEventID: bookedEventID, Code: canon.CodeOU, Side: string(canon.SideOver), Line: canon.Line(2.5)},
		{SourceEventID: "852457626", Code: canon.CodeAH, Side: string(canon.SideHome), Line: canon.Line(-0.5)},
	}

	if _, err := adapter.Book(t.Context(), legs); err != nil {
		t.Fatalf("book: %v", err)
	}

	slip := server.slips[0]
	wantEvents := map[string]betslipEvent{
		"832843044$S_OU@2.5_O":  {EventID: bookedEventID, Market: "S_OU", Sign: "O", SID: "S_OU@2.5_O", ID: "832843044$S_OU@2.5_O"},
		"852457626$S_AH@-0.5_1": {EventID: "852457626", Market: "S_AH", Sign: "1", SID: "S_AH@-0.5_1", ID: "852457626$S_AH@-0.5_1"},
	}
	if len(slip.Events) != len(wantEvents) {
		t.Fatalf("posted events = %+v, want %+v", slip.Events, wantEvents)
	}
	for id, want := range wantEvents {
		if got := slip.Events[id]; got != want {
			t.Errorf("posted event %s = %+v, want %+v", id, got, want)
		}
	}
	if got := slip.Bets[0].Type; got != len(wantEvents) {
		t.Errorf("posted type = %d, want %d", got, len(wantEvents))
	}
}

func TestAdapter_BookErrors(t *testing.T) {
	t.Parallel()
	bookable := []sources.BookLeg{{SourceEventID: bookedEventID, Code: canon.Code1X2, Side: string(canon.SideHome)}}
	tests := []struct {
		name    string
		server  *bookingServer
		legs    []sources.BookLeg
		wantErr error
	}{
		{
			name:    "booking endpoint forbidden",
			server:  &bookingServer{status: http.StatusForbidden},
			legs:    bookable,
			wantErr: sources.ErrBookingBlocked,
		},
		{
			name:    "no leg resolves",
			server:  &bookingServer{},
			legs:    []sources.BookLeg{{SourceEventID: "1", Code: canon.Code1X2, Side: string(canon.SideHome)}},
			wantErr: errNothingBookable,
		},
		{
			name:    "booking rejected",
			server:  &bookingServer{response: []byte(`{"status":0,"error":{"code":12,"message":"event closed"},"data":[]}`)},
			legs:    bookable,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := newBookingAdapter(t, tt.server).Book(t.Context(), tt.legs)
			if err == nil {
				t.Fatal("book succeeded, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeBooking_Fixture(t *testing.T) {
	t.Parallel()
	code, prices, err := decodeBooking(sourcestest.ReadFixture(t, Name, bookingFixture))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if code != "5XFHSN6" {
		t.Errorf("code = %q, want 5XFHSN6", code)
	}
	if got := prices["832843044$S_1X2_2"]; got != 1.7 {
		t.Errorf("price = %v, want 1.7", got)
	}
}
