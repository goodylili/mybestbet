package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"golang.org/x/time/rate"

	"mybestbet/pkg/canon"
)

type Capabilities struct {
	Markets      []canon.MarketCode
	Leagues      []string
	NeedsBrowser bool
	Push         bool
	Geo          string
	MinInterval  time.Duration
}

type Adapter interface {
	Name() string
	Capabilities() Capabilities
	Discover(ctx context.Context, leagues []string) (canon.Snapshot, error)
	Price(ctx context.Context, eventIDs []string) (canon.Snapshot, error)
}

type Streamer interface {
	Stream(ctx context.Context, eventIDs []string, out chan<- canon.Snapshot) error
}

type BrowserRequest struct {
	Profile     string   `json:"profile"`
	URL         string   `json:"url"`
	CapturePath []string `json:"capturePath"`
	CaptureWS   bool     `json:"captureWs"`
	WaitMillis  int      `json:"waitMillis"`
}

type Captured struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	Body   []byte `json:"body"`
	WS     bool   `json:"ws,omitempty"`
}

type BrowserResult struct {
	Captured []Captured        `json:"captured"`
	Cookies  map[string]string `json:"cookies"`
	Headers  map[string]string `json:"headers"`
}

type Browser interface {
	Fetch(ctx context.Context, req BrowserRequest) (BrowserResult, error)
}

type Config struct {
	BaseURL     string            `yaml:"baseUrl"`
	Headers     map[string]string `yaml:"headers"`
	APIKey      string            `yaml:"-"`
	APISecret   string            `yaml:"-"`
	LeagueIDs   map[string]string `yaml:"leagueIds"`
	Extra       map[string]string `yaml:"extra"`
	MaxRPS      float64           `yaml:"maxRps"`
	Geo         string            `yaml:"geo"`
	Proxy       string            `yaml:"proxy"`
	Enabled     bool              `yaml:"enabled"`
	Weight      float64           `yaml:"weight"`
	FeePerShare float64           `yaml:"feePerShare"`
}

type Deps struct {
	HTTP    *http.Client
	Limiter *rate.Limiter
	Browser Browser
	Config  Config
	Logger  *slog.Logger
	Now     func() time.Time
}

type Factory func(Deps) Adapter

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

func Lookup(name string) (Factory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[name]
	return f, ok
}

func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var (
	ErrThrottled = errors.New("source throttled")
	ErrForbidden = errors.New("source forbidden")
	ErrNotFound  = errors.New("source resource not found")
)

type StatusError struct {
	Method string
	URL    string
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s %s: status %d", e.Method, e.URL, e.Status)
}

func (e *StatusError) Unwrap() error {
	switch e.Status {
	case http.StatusTooManyRequests:
		return ErrThrottled
	case http.StatusForbidden, http.StatusUnauthorized:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	}
	return nil
}

const maxResponseBytes = 32 << 20

func (d Deps) Do(req *http.Request) ([]byte, error) {
	if d.Limiter != nil {
		if err := d.Limiter.Wait(req.Context()); err != nil {
			return nil, fmt.Errorf("rate limit wait: %w", err)
		}
	}
	for k, v := range d.Config.Headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, &StatusError{Method: req.Method, URL: req.URL.Redacted(), Status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", req.URL.Redacted(), err)
	}
	return body, nil
}

func (d Deps) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", url, err)
	}
	return d.Do(req)
}

func (d Deps) GetJSON(ctx context.Context, url string, out any) error {
	body, err := d.Get(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

func (d Deps) Clock() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now().UTC()
}

func (d Deps) Log() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

var reportedUnknownMarkets sync.Map

func (d Deps) DropUnknownMarket(source, sourceMarketID string) {
	if _, seen := reportedUnknownMarkets.LoadOrStore(source+"|"+sourceMarketID, struct{}{}); seen {
		return
	}
	d.Log().Debug("unknown market dropped", "source", source, "source_market_id", sourceMarketID)
}

func NewSnapshot(source string, fetchedAt time.Time) canon.Snapshot {
	return canon.Snapshot{Source: source, FetchedAt: fetchedAt}
}

func FailedEntirely(snap canon.Snapshot) error {
	if len(snap.Errors) == 0 || len(snap.Events) > 0 || len(snap.Markets) > 0 {
		return nil
	}
	return errors.Join(snap.Errors...)
}
