package booking_test

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/booking"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/fake"
)

type bookServer struct {
	mu       sync.Mutex
	calls    map[string]int
	last     map[string][]sources.BookLeg
	blocked  map[string]bool
	delay    time.Duration
	upstream *httptest.Server
}

func newBookServer(t *testing.T) *bookServer {
	t.Helper()
	b := &bookServer{calls: map[string]int{}, last: map[string][]sources.BookLeg{}, blocked: map[string]bool{}}
	b.upstream = httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(b.upstream.Close)
	return b
}

func (b *bookServer) serve(w http.ResponseWriter, r *http.Request) {
	book := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/booking")
	raw, _ := io.ReadAll(r.Body)
	var body struct {
		Selections []sources.BookLeg `json:"selections"`
	}
	_ = json.Unmarshal(raw, &body)
	b.mu.Lock()
	b.calls[book]++
	call, delay, blocked := b.calls[book], b.delay, b.blocked[book]
	b.last[book] = body.Selections
	b.mu.Unlock()
	time.Sleep(delay)
	if blocked {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	_, _ = w.Write([]byte(`{"code":"` + strings.ToUpper(book[:5]) + strconv.Itoa(call) + `"}`))
}

func (b *bookServer) callsOf(book string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls[book]
}

func (b *bookServer) lastOf(book string) []sources.BookLeg {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.last[book]
}

type fixture struct {
	svc         *booking.Service
	deps        booking.Deps
	books       *bookServer
	bookA       string
	bookB       string
	blocked     string
	pred        string
	eventID     string
	otherEvent  string
	oneXTwo     string
	overUnder   string
	otherMarket string
}

func integrationDeps(t *testing.T) (*pgxpool.Pool, *goredis.Client) {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL are required")
	}
	pool, err := postgres.Open(t.Context(), dbURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	rdb, err := redis.Open(t.Context(), redisURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	return pool, rdb
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool, rdb := integrationDeps(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	f := fixture{books: newBookServer(t), bookA: "fakea" + suffix, bookB: "fakeb" + suffix, blocked: "fakex" + suffix, pred: "fakep" + suffix}
	f.books.blocked[f.blocked] = true
	for _, name := range []string{f.bookA, f.bookB, f.blocked, f.pred} {
		sources.Register(name, fake.New)
	}
	f.eventID, f.otherEvent = seedEvents(t, pool, suffix)
	links := [][3]any{{f.bookA, "fake-1", false}, {f.bookB, "fake-2", true}, {f.blocked, "fake-1", false}, {f.pred, "pm-1", false}, {f.bookB, "fake-9", false}}
	for i, link := range links {
		eventID := f.eventID
		if i == len(links)-1 {
			eventID = f.otherEvent
		}
		_, err := pool.Exec(ctx, `INSERT INTO event_links (source, "sourceEventId", "eventId", flipped) VALUES ($1, $2, $3, $4)`, link[0], link[1], eventID, link[2])
		require.NoError(t, err)
	}
	f.oneXTwo = canon.MarketKey{EventID: f.eventID, Code: canon.Code1X2}.String()
	f.overUnder = canon.MarketKey{EventID: f.eventID, Code: canon.CodeOU, Line: canon.Line(2.5)}.String()
	f.otherMarket = canon.MarketKey{EventID: f.otherEvent, Code: canon.Code1X2}.String()
	now := time.Now().UTC()
	cells := []canon.BoardCell{
		decimal(f.oneXTwo, f.bookA, canon.SideHome, 2.1, now),
		decimal(f.oneXTwo, f.bookB, canon.SideHome, 2.3, now),
		decimal(f.oneXTwo, f.bookB, canon.SideAway, 3.2, now),
		decimal(f.oneXTwo, f.blocked, canon.SideHome, 2.0, now),
		decimal(f.overUnder, f.bookA, canon.SideOver, 1.9, now),
		decimal(f.otherMarket, f.bookB, canon.SideHome, 1.5, now),
		{MarketKey: f.oneXTwo, Source: f.pred, Side: canon.SideHome, Price: canon.NewBookPrice(nil, new(0.4), nil, canon.FormShare, now), DeepLink: "https://pred.test/m/1"},
	}
	for _, cell := range cells {
		encoded, err := json.Marshal(cell)
		require.NoError(t, err)
		require.NoError(t, rdb.HSet(ctx, redis.BoardKey(cell.MarketKey), redis.BoardField(cell.Source, string(cell.Side), cell.Label), encoded).Err())
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = rdb.Del(ctx, redis.BoardKey(f.oneXTwo), redis.BoardKey(f.overUnder), redis.BoardKey(f.otherMarket)).Err()
		_, _ = pool.Exec(ctx, `DELETE FROM event_links WHERE "eventId" = ANY($1::uuid[])`, []string{f.eventID, f.otherEvent})
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE id = ANY($1::uuid[])`, []string{f.eventID, f.otherEvent})
		_, _ = pool.Exec(ctx, `DELETE FROM teams WHERE "normalizedName" LIKE $1`, "%"+suffix)
	})
	sportsbook := func(name string) sources.Config {
		return sources.Config{Enabled: true, BaseURL: f.books.upstream.URL + "/" + name}
	}
	cfg := config.Config{
		Sources: map[string]sources.Config{f.bookA: sportsbook(f.bookA), f.bookB: sportsbook(f.bookB), f.blocked: sportsbook(f.blocked), f.pred: {Enabled: true}},
		Profiles: map[string]config.SourceProfile{
			f.bookA: {Type: "sportsbook"}, f.bookB: {Type: "sportsbook"}, f.blocked: {Type: "sportsbook"}, f.pred: {Type: "prediction"},
		},
	}
	f.deps = booking.Deps{Config: cfg, Pool: pool, Redis: rdb, Log: slog.Default(), CallTimeout: 5 * time.Second}
	svc, err := booking.New(f.deps)
	require.NoError(t, err)
	f.svc = svc
	return f
}

func seedEvents(t *testing.T, pool *pgxpool.Pool, suffix string) (string, string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	team := func(name string) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, name+" "+suffix).Scan(&id))
		return id
	}
	event := func(home, away string) string {
		var id string
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3) RETURNING id::text`,
			team(home), team(away), time.Now().Add(2*time.Hour).Truncate(time.Second)).Scan(&id))
		return id
	}
	return event("Home", "Away"), event("Other Home", "Other Away")
}

func decimal(marketKey, source string, side canon.Side, raw float64, at time.Time) canon.BoardCell {
	return canon.BoardCell{MarketKey: marketKey, Source: source, Side: side, Price: canon.NewPrice(raw, canon.FormDecimal, at), DeepLink: "https://" + source + ".test/e"}
}

func (f fixture) leg(marketKey, source string, side canon.Side) booking.Leg {
	key, _ := canon.ParseMarketKey(marketKey)
	return booking.Leg{EventID: key.EventID, MarketKey: marketKey, Source: source, Side: string(side)}
}

func resultOf(t *testing.T, resp booking.Response, book string) booking.BookResult {
	t.Helper()
	for _, result := range resp.Bookings {
		if result.Book == book {
			return result
		}
	}
	require.Failf(t, "no booking result", "book %s in %+v", book, resp.Bookings)
	return booking.BookResult{}
}

func TestBookSplitGroupsLegsByBook(t *testing.T) {
	f := newFixture(t)
	req := booking.Request{Stake: 100, Legs: []booking.Leg{
		f.leg(f.oneXTwo, f.bookA, canon.SideHome),
		f.leg(f.overUnder, f.bookA, canon.SideOver),
		f.leg(f.oneXTwo, f.bookB, canon.SideAway),
		f.leg(f.oneXTwo, f.pred, canon.SideHome),
	}}
	resp, err := f.svc.Book(t.Context(), uuid.NewString(), req)
	require.NoError(t, err)
	assert.Equal(t, booking.ModeSplit, resp.Mode)
	require.Len(t, resp.Bookings, 2)
	first := resultOf(t, resp, f.bookA)
	assert.Equal(t, booking.StatusOK, first.Status)
	assert.NotEmpty(t, first.Code)
	assert.Equal(t, f.books.upstream.URL+"/"+f.bookA+"/?shareCode="+first.Code, first.WebURL)
	assert.Equal(t, "fake://booking/"+first.Code, first.AppURL)
	assert.InDelta(t, 399, first.Return, 1e-9)
	assert.Equal(t, []int{0, 1}, []int{first.Legs[0].Index, first.Legs[1].Index})
	require.NotNil(t, first.ExpiresAt)
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), *first.ExpiresAt, time.Minute)
	second := resultOf(t, resp, f.bookB)
	assert.Equal(t, booking.StatusOK, second.Status)
	sent := f.books.lastOf(f.bookB)
	require.Len(t, sent, 1)
	assert.Equal(t, "home", sent[0].Side)
	assert.Equal(t, "fake-2", sent[0].SourceEventID)
	require.Len(t, resp.Manual, 1)
	assert.Equal(t, booking.ManualLeg{Index: 3, Source: f.pred, DeepLink: "https://pred.test/m/1", Price: 2.5, Reason: "this source has no booking codes, place it manually"}, resp.Manual[0])
}

func TestBookSingleMapsEveryLegToTheChosenBook(t *testing.T) {
	f := newFixture(t)
	req := booking.Request{Mode: booking.ModeSingle, Book: f.bookA, Legs: []booking.Leg{
		f.leg(f.oneXTwo, f.bookB, canon.SideHome),
		f.leg(f.overUnder, f.bookA, canon.SideOver),
		f.leg(f.otherMarket, f.bookB, canon.SideHome),
	}}
	resp, err := f.svc.Book(t.Context(), uuid.NewString(), req)
	require.NoError(t, err)
	assert.Equal(t, f.bookA, resp.Book)
	require.Len(t, resp.Bookings, 1)
	result := resp.Bookings[0]
	assert.Equal(t, booking.StatusPartial, result.Status)
	assert.NotEmpty(t, result.Code)
	assert.Equal(t, []booking.BookedLeg{
		{Index: 0, Price: 2.1, OK: true},
		{Index: 1, Price: 1.9, OK: true},
		{Index: 2, Reason: "event not listed at this book"},
	}, result.Legs)
	assert.Equal(t, 0, f.books.callsOf(f.bookB))
	require.Len(t, resp.Manual, 1)
	assert.Equal(t, 2, resp.Manual[0].Index)
	assert.Equal(t, f.bookB, resp.Manual[0].Source)
}

func TestNoBookingProfileIsNotBookable(t *testing.T) {
	f := newFixture(t)
	deps := f.deps
	deps.Config.Profiles = maps.Clone(deps.Config.Profiles)
	deps.Config.Profiles[f.bookB] = config.SourceProfile{Type: "sportsbook", NoBooking: true}
	svc, err := booking.New(deps)
	require.NoError(t, err)
	assert.Contains(t, svc.Bookable(), f.bookA)
	assert.NotContains(t, svc.Bookable(), f.bookB)
	comparison, err := svc.Compare(t.Context(), uuid.NewString(), booking.Request{Legs: []booking.Leg{f.leg(f.oneXTwo, f.bookB, canon.SideHome)}})
	require.NoError(t, err)
	for _, q := range comparison.Books {
		if q.Book == f.bookB {
			assert.False(t, q.Bookable)
		}
	}
}

func TestCompareUsesCachedPricesOnly(t *testing.T) {
	f := newFixture(t)
	req := booking.Request{Stake: 10, Legs: []booking.Leg{
		f.leg(f.oneXTwo, f.pred, canon.SideHome),
		f.leg(f.overUnder, f.bookA, canon.SideOver),
	}}
	comparison, err := f.svc.Compare(t.Context(), uuid.NewString(), req)
	require.NoError(t, err)
	assert.Equal(t, 2, comparison.TotalLegs)
	assert.InDelta(t, 2.5*1.9, comparison.Picked.Odds, 1e-9)
	assert.InDelta(t, 47.5, comparison.Picked.Return, 1e-9)
	assert.Equal(t, []booking.QuoteLeg{{Index: 0, Source: f.bookB, Price: 2.3}, {Index: 1, Source: f.bookA, Price: 1.9}}, comparison.BestSplit.Legs)
	assert.InDelta(t, 43.7, comparison.BestSplit.Return, 1e-9)
	require.Len(t, comparison.Books, 3)
	assert.Equal(t, booking.BookQuote{
		Book: f.bookA, Bookable: true, Covered: 2, Total: 2, Odds: 3.99, Return: 39.9,
		Legs: []booking.QuoteLeg{{Index: 0, Source: f.bookA, Price: 2.1}, {Index: 1, Source: f.bookA, Price: 1.9}}, Missing: []int{},
	}, comparison.Books[0])
	assert.Equal(t, f.bookB, comparison.Books[1].Book)
	assert.Equal(t, []int{1}, comparison.Books[1].Missing)
	for _, book := range []string{f.bookA, f.bookB, f.blocked} {
		assert.Zero(t, f.books.callsOf(book))
	}
}

func TestBookIsIdempotent(t *testing.T) {
	f := newFixture(t)
	legs := []booking.Leg{f.leg(f.oneXTwo, f.bookA, canon.SideHome), f.leg(f.overUnder, f.bookA, canon.SideOver)}
	user := uuid.NewString()
	first, err := f.svc.Book(t.Context(), user, booking.Request{Legs: legs})
	require.NoError(t, err)
	reordered := []booking.Leg{legs[1], legs[0]}
	again, err := f.svc.Book(t.Context(), user, booking.Request{Legs: reordered, Stake: 5})
	require.NoError(t, err)
	assert.Equal(t, 1, f.books.callsOf(f.bookA))
	assert.False(t, first.Bookings[0].Cached)
	assert.True(t, again.Bookings[0].Cached)
	assert.Equal(t, first.Bookings[0].Code, again.Bookings[0].Code)
	assert.Equal(t, first.Bookings[0].ExpiresAt, again.Bookings[0].ExpiresAt)
	assert.Equal(t, []int{0, 1}, []int{again.Bookings[0].Legs[0].Index, again.Bookings[0].Legs[1].Index})
}

func TestConcurrentDuplicateBookingsShareOneCall(t *testing.T) {
	f := newFixture(t)
	f.books.delay = 200 * time.Millisecond
	req := booking.Request{Legs: []booking.Leg{f.leg(f.oneXTwo, f.bookB, canon.SideHome)}}
	codes := make([]string, 4)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Go(func() {
			resp, err := f.svc.Book(context.Background(), uuid.NewString(), req)
			if assert.NoError(t, err) {
				codes[i] = resp.Bookings[0].Code
			}
		})
	}
	wg.Wait()
	assert.Equal(t, 1, f.books.callsOf(f.bookB))
	for _, code := range codes {
		assert.Equal(t, codes[0], code)
	}
}

func TestBookReportsEachBookFailureSeparately(t *testing.T) {
	f := newFixture(t)
	req := booking.Request{Legs: []booking.Leg{
		f.leg(f.oneXTwo, f.blocked, canon.SideHome),
		f.leg(f.overUnder, f.bookA, canon.SideOver),
		f.leg(f.oneXTwo, f.bookB, canon.SideHome),
		f.leg(f.otherMarket, f.bookB, canon.SideHome),
	}}
	resp, err := f.svc.Book(t.Context(), uuid.NewString(), req)
	require.NoError(t, err)
	blocked := resultOf(t, resp, f.blocked)
	assert.Equal(t, booking.StatusBlocked, blocked.Status)
	assert.Empty(t, blocked.Code)
	assert.Equal(t, "this book blocks booking from our region", blocked.Message)
	assert.Equal(t, booking.StatusOK, resultOf(t, resp, f.bookA).Status)
	partial := resultOf(t, resp, f.bookB)
	assert.Equal(t, booking.StatusPartial, partial.Status)
	assert.NotEmpty(t, partial.Code)
	assert.Equal(t, booking.BookedLeg{Index: 3, Price: 1.5, Reason: "event not found"}, partial.Legs[1])
	indexes := make([]int, len(resp.Manual))
	for i, manual := range resp.Manual {
		indexes[i] = manual.Index
	}
	assert.Equal(t, []int{0, 3}, indexes)

	again, err := f.svc.Book(t.Context(), uuid.NewString(), req)
	require.NoError(t, err)
	assert.Equal(t, booking.StatusBlocked, resultOf(t, again, f.blocked).Status)
	assert.Equal(t, 2, f.books.callsOf(f.blocked))
	assert.Equal(t, 1, f.books.callsOf(f.bookA))
}

func TestBookRejectsInvalidRequests(t *testing.T) {
	f := newFixture(t)
	valid := f.leg(f.oneXTwo, f.bookA, canon.SideHome)
	tooMany := make([]booking.Leg, booking.MaxLegs+1)
	for i := range tooMany {
		tooMany[i] = valid
	}
	mismatched := valid
	mismatched.EventID = f.otherEvent
	unknownSource := valid
	unknownSource.Source = "nowhere"
	badSide := valid
	badSide.Side = "sideways"
	unknownEvent := f.leg(canon.MarketKey{EventID: uuid.NewString(), Code: canon.Code1X2}.String(), f.bookA, canon.SideHome)
	tests := []struct {
		name  string
		req   booking.Request
		field string
	}{
		{name: "no legs", req: booking.Request{}, field: "legs"},
		{name: "too many legs", req: booking.Request{Legs: tooMany}, field: "legs"},
		{name: "duplicate legs", req: booking.Request{Legs: []booking.Leg{valid, valid}}, field: "legs[1].marketKey"},
		{name: "unknown mode", req: booking.Request{Mode: "all", Legs: []booking.Leg{valid}}, field: "mode"},
		{name: "single without book", req: booking.Request{Mode: booking.ModeSingle, Legs: []booking.Leg{valid}}, field: "book"},
		{name: "single on a prediction source", req: booking.Request{Mode: booking.ModeSingle, Book: f.pred, Legs: []booking.Leg{valid}}, field: "book"},
		{name: "market of another event", req: booking.Request{Legs: []booking.Leg{mismatched}}, field: "legs[0].marketKey"},
		{name: "unknown source", req: booking.Request{Legs: []booking.Leg{unknownSource}}, field: "legs[0].source"},
		{name: "unknown side", req: booking.Request{Legs: []booking.Leg{badSide}}, field: "legs[0].side"},
		{name: "negative stake", req: booking.Request{Stake: -1, Legs: []booking.Leg{valid}}, field: "stake"},
		{name: "unknown event", req: booking.Request{Legs: []booking.Leg{unknownEvent}}, field: "legs[0].eventId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.svc.Book(t.Context(), uuid.NewString(), tt.req)
			var input *booking.InputError
			require.ErrorAs(t, err, &input)
			assert.Equal(t, tt.field, input.Field)
		})
	}
	assert.Zero(t, f.books.callsOf(f.bookA))
}

func TestBookIsRateLimitedPerUser(t *testing.T) {
	f := newFixture(t)
	windowStart := time.Now().Truncate(time.Minute).Add(time.Minute)
	f.deps.Now = func() time.Time { return windowStart.Add(time.Second) }
	svc, err := booking.New(f.deps)
	require.NoError(t, err)
	user := uuid.NewString()
	req := booking.Request{Legs: []booking.Leg{f.leg(f.oneXTwo, f.bookA, canon.SideHome)}}
	for range 20 {
		_, err := svc.Book(t.Context(), user, req)
		require.NoError(t, err)
	}
	_, err = svc.Book(t.Context(), user, req)
	var limited *booking.RateLimitError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, 59*time.Second, limited.RetryAfter)
	_, err = svc.Book(t.Context(), uuid.NewString(), req)
	require.NoError(t, err)
	assert.Equal(t, 1, f.books.callsOf(f.bookA))
}
