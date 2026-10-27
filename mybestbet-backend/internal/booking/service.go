package booking

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"mybestbet/internal/ingest"
	"mybestbet/internal/platform/config"
	"mybestbet/pkg/sources"
)

const (
	sportsbookType     = "sportsbook"
	defaultCallTimeout = 20 * time.Second
)

type Deps struct {
	Config      config.Config
	Pool        *pgxpool.Pool
	Redis       *goredis.Client
	Log         *slog.Logger
	NewBrowser  ingest.BrowserFactory
	Now         func() time.Time
	CallTimeout time.Duration
}

type bookInfo struct {
	name   string
	booker sources.Booker
}

type Service struct {
	cfg         config.Config
	store       store
	rdb         *goredis.Client
	log         *slog.Logger
	books       []bookInfo
	bookers     map[string]sources.Booker
	flight      singleflight.Group
	now         func() time.Time
	callTimeout time.Duration
}

func New(deps Deps) (*Service, error) {
	log := cmp.Or(deps.Log, slog.Default())
	s := &Service{
		cfg:         deps.Config,
		store:       store{pool: deps.Pool, rdb: deps.Redis, log: log},
		rdb:         deps.Redis,
		log:         log,
		bookers:     map[string]sources.Booker{},
		now:         deps.Now,
		callTimeout: cmp.Or(deps.CallTimeout, defaultCallTimeout),
	}
	if s.now == nil {
		s.now = time.Now
	}
	names := make([]string, 0, len(deps.Config.Sources))
	for name, sc := range deps.Config.Sources {
		if sc.Enabled && deps.Config.Profiles[name].Type == sportsbookType {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		adapter, err := ingest.NewAdapter(name, deps.Config.Sources[name], deps.NewBrowser, log)
		if errors.Is(err, sources.ErrNotFound) {
			log.Warn("enabled sportsbook has no registered adapter", "source", name)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("booking: %w", err)
		}
		booker, _ := adapter.(sources.Booker)
		if deps.Config.Profiles[name].NoBooking {
			booker = nil
		}
		s.books = append(s.books, bookInfo{name: name, booker: booker})
		if booker != nil {
			s.bookers[name] = booker
		}
	}
	return s, nil
}

func (s *Service) Bookable() []string {
	names := make([]string, 0, len(s.bookers))
	for _, book := range s.books {
		if book.booker != nil {
			names = append(names, book.name)
		}
	}
	return names
}

func (s *Service) Book(ctx context.Context, userID string, req Request) (Response, error) {
	mode, err := s.validateMode(req)
	if err != nil {
		return Response{}, err
	}
	legs, stake, err := s.validate(ctx, req, bookPerUser, userID)
	if err != nil {
		return Response{}, err
	}
	data, err := s.loadKnown(ctx, legs, true)
	if err != nil {
		return Response{}, err
	}
	plans := s.planSplit(legs, data)
	book := ""
	if mode == ModeSingle {
		book = req.Book
		plans = []bookPlan{s.planSingle(book, legs, data)}
	}
	results := s.execute(ctx, plans, stake)
	return Response{
		Mode:       mode,
		Book:       book,
		Bookings:   results,
		Manual:     manualLegs(legs, results, data),
		Comparison: s.compare(legs, data, stake),
	}, nil
}

func (s *Service) Compare(ctx context.Context, userID string, req Request) (Comparison, error) {
	legs, stake, err := s.validate(ctx, req, comparePerUser, userID)
	if err != nil {
		return Comparison{}, err
	}
	data, err := s.loadKnown(ctx, legs, false)
	if err != nil {
		return Comparison{}, err
	}
	return s.compare(legs, data, stake), nil
}

func (s *Service) validateMode(req Request) (Mode, error) {
	switch req.Mode {
	case "", ModeSplit:
		return ModeSplit, nil
	case ModeSingle:
		if req.Book == "" {
			return "", invalidInput("book", "is required in single mode")
		}
		if s.bookers[req.Book] == nil {
			return "", invalidInput("book", "does not support booking codes")
		}
		return ModeSingle, nil
	default:
		return "", invalidInput("mode", "must be split or single")
	}
}

func (s *Service) validate(ctx context.Context, req Request, rule limitRule, userID string) ([]slipLeg, float64, error) {
	legs, err := s.parseLegs(req.Legs)
	if err != nil {
		return nil, 0, err
	}
	stake, err := normalizeStake(req.Stake)
	if err != nil {
		return nil, 0, err
	}
	if err := s.allow(ctx, rule, userID); err != nil {
		return nil, 0, err
	}
	return legs, stake, nil
}

func (s *Service) loadKnown(ctx context.Context, legs []slipLeg, withLinks bool) (slipData, error) {
	data, err := s.store.load(ctx, legs, withLinks)
	if err != nil {
		return slipData{}, err
	}
	for _, leg := range legs {
		if !data.events[leg.key.EventID] {
			return slipData{}, invalidInput(legField(leg.index, "eventId"), "unknown event")
		}
	}
	return data, nil
}
