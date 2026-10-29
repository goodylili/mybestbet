package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/auth"
	"mybestbet/internal/booking"
	"mybestbet/internal/logos"
	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/observability"
	"mybestbet/internal/review"
	"mybestbet/internal/stats"
	"mybestbet/pkg/canon"
)

type statsReader interface {
	PlayerStats(ctx context.Context, playerID string, last int) ([]stats.MatchStat, error)
	Histories(ctx context.Context, playerIDs []string) (map[string]stats.History, error)
	LineupStatuses(ctx context.Context, eventID string, playerIDs []string) (map[string]stats.LineupStatus, error)
}

type reviewQueue interface {
	List(ctx context.Context, status string, limit int) ([]review.Item, error)
	Decide(ctx context.Context, decision review.Decision) (review.Item, error)
}

type Deps struct {
	Config config.Config
	Pool   *pgxpool.Pool
	Redis  *goredis.Client
	Stats  statsReader
	Review reviewQueue
	Auth   *auth.Service
	Books  *booking.Service
	Log    *slog.Logger
}

type Server struct {
	cfg           config.Config
	leagueSports  map[string]canon.Sport
	storedLeagues atomic.Pointer[map[string]canon.Sport]
	pool          *pgxpool.Pool
	rdb           *goredis.Client
	stats         statsReader
	review        reviewQueue
	auth          *auth.Service
	bookings      *booking.Service
	logos         *logos.Store
	log           *slog.Logger
	hub           *streamHub
	coverage      coverageCache
	events        snapshot[*eventIndex]
	signals       snapshot[signalSnapshot]
	sources       snapshot[[]SourceHealth]
	leagues       snapshot[[]League]
	settings      settingsCache
	responses     responseCache
	priority      priorityQueue
	heartbeat     time.Duration
	countries     *countryDB
	streams       streamQuota
	links         eventLinks
}

func New(deps Deps) *Server {
	return &Server{
		cfg:          deps.Config,
		leagueSports: sportByLeague(deps.Config.Leagues),
		pool:         deps.Pool,
		rdb:          deps.Redis,
		stats:        deps.Stats,
		review:       deps.Review,
		auth:         deps.Auth,
		bookings:     deps.Books,
		logos:        logos.NewStore(deps.Pool),
		log:          deps.Log,
		hub:          newStreamHub(deps.Redis, deps.Log),
		countries:    openCountryDB(deps.Config.GeoIPDB, deps.Log),
		links:        newEventLinks(deps.Config),
	}
}

func (s *Server) Handler() http.Handler {
	mux := observability.Mux()
	for pattern, handler := range s.routes() {
		var routed http.Handler = handler
		if !uncompressedRoutes[pattern] {
			routed = compressResponses(routed)
		}
		if ttl, ok := sharedResponseTTL[pattern]; ok {
			routed = s.responses.shared(ttl, routed)
		}
		mux.Handle(pattern, s.instrument(pattern, s.guard(pattern, routed.ServeHTTP)))
	}
	return mux
}

var uncompressedRoutes = map[string]bool{"GET /v1/stream": true, "GET /v1/logos/{kind}/{id}": true}

var sharedResponseTTL = map[string]time.Duration{
	"GET /v1/sports":            5 * time.Second,
	"GET /v1/leagues":           5 * time.Second,
	"GET /v1/events":            time.Second,
	"GET /v1/events/calendar":   5 * time.Second,
	"GET /v1/events/{id}":       time.Second,
	"GET /v1/events/{id}/props": 2 * time.Second,
	"GET /v1/signals":           time.Second,
	"GET /v1/sources":           time.Second,
	"GET /v1/baysed":            30 * time.Second,
}

func (s *Server) routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /v1/sports":                s.listSports,
		"GET /v1/meta":                  s.getMeta,
		"GET /v1/search":                s.search,
		"GET /v1/leagues":               s.listLeagues,
		"GET /v1/events":                s.listEvents,
		"GET /v1/events/calendar":       s.eventCalendar,
		"GET /v1/events/{id}":           s.getEvent,
		"GET /v1/events/{id}/props":     s.getEventProps,
		"GET /v1/markets/{key}/history": s.getMarketHistory,
		"GET /v1/signals":               s.listSignals,
		"GET /v1/baysed":                s.getBaysed,
		"GET /v1/players/{id}/stats":    s.getPlayerStats,
		"GET /v1/teams/{id}":            s.getTeam,
		"GET /v1/sources":               s.listSources,
		"GET /v1/logos/{kind}/{id}":     s.getLogo,
		"GET /v1/stream":                s.stream,
		"POST /v1/bookings":             s.createBooking,
		"POST /v1/bookings/compare":     s.compareBookings,
		"GET /v1/me":                    s.getMe,
		"GET /v1/me/settings":           s.getSettings,
		"PUT /v1/me/settings":           s.putSettings,
		"GET /v1/admin/review":          s.listReview,
		"POST /v1/admin/review":         s.decideReview,
		"POST /v1/auth/signup":          s.signup,
		"POST /v1/auth/verify":          s.verifyCode,
		"POST /v1/auth/otp/resend":      s.resendCode,
		"POST /v1/auth/login":           s.login,
		"POST /v1/auth/password/forgot": s.forgotPassword,
		"POST /v1/auth/password/reset":  s.resetPassword,
		"POST /v1/auth/refresh":         s.refresh,
		"POST /v1/auth/logout":          s.logout,
		"GET /v1/auth/google/start":     s.googleStart,
		"GET /v1/auth/google/callback":  s.googleCallback,
	}
}
