package auth

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/config"
)

const (
	defaultWebOrigin  = "http://localhost:4040"
	mailTimeout       = 10 * time.Second
	defaultHTTPTimout = 10 * time.Second
)

type Deps struct {
	Config        config.Config
	Pool          *pgxpool.Pool
	Redis         *goredis.Client
	Log           *slog.Logger
	HTTPClient    *http.Client
	Mailer        Mailer
	ResendBaseURL string
	Google        GoogleEndpoints
	Now           func() time.Time
}

type Client struct {
	IP        string
	UserAgent string
}

type Session struct {
	User             User
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type CodeSent struct {
	ResendIn time.Duration
	Binding  string
}

type Service struct {
	cfg     config.Auth
	pool    *pgxpool.Pool
	rdb     *goredis.Client
	log     *slog.Logger
	signer  *signer
	keys    keys
	limiter limiter
	mailer  Mailer
	google  *googleClient
	now     func() time.Time
	mail    sync.WaitGroup
	active  activeFamilies
}

func New(deps Deps) (*Service, error) {
	cfg, err := normalizeConfig(deps.Config.Auth, deps.Config.Env)
	if err != nil {
		return nil, err
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	log := cmp.Or(deps.Log, slog.Default())
	seed, err := loadJWTSeed(cfg.JWTPrivateKey, deps.Config.Env, log)
	if err != nil {
		return nil, err
	}
	sig, err := newSigner(seed, cfg.AccessTTL, now)
	if err != nil {
		return nil, err
	}
	pepper, err := loadPepper(cfg.CodePepper, deps.Config.Env, log)
	if err != nil {
		return nil, err
	}
	dummyHash()
	httpClient := cmp.Or(deps.HTTPClient, &http.Client{Timeout: defaultHTTPTimout})
	mailer, err := newMailer(deps, cfg, httpClient, log)
	if err != nil {
		return nil, err
	}
	return &Service{
		cfg:     cfg,
		pool:    deps.Pool,
		rdb:     deps.Redis,
		log:     log,
		signer:  sig,
		keys:    deriveKeys(pepper),
		limiter: limiter{counter: redisCounter{rdb: deps.Redis}, now: now},
		mailer:  mailer,
		google:  newGoogleClient(cfg, deps.Google, httpClient),
		now:     now,
	}, nil
}

func normalizeConfig(cfg config.Auth, env string) (config.Auth, error) {
	if cfg.WebOrigin == "" {
		if !isLocalEnv(env) {
			return cfg, errors.New("auth: WEB_ORIGIN is required outside APP_ENV=dev or test")
		}
		cfg.WebOrigin = defaultWebOrigin
	}
	origins := []string{cfg.WebOrigin}
	for _, origin := range cfg.AllowedOrigins {
		if origin != "" && origin != cfg.WebOrigin {
			origins = append(origins, origin)
		}
	}
	cfg.AllowedOrigins = origins
	cfg.AccessTTL = cmp.Or(cfg.AccessTTL, 15*time.Minute)
	cfg.RefreshTTL = cmp.Or(cfg.RefreshTTL, 720*time.Hour)
	cfg.OTPTTL = cmp.Or(cfg.OTPTTL, 10*time.Minute)
	cfg.OTPCooldown = cmp.Or(cfg.OTPCooldown, time.Minute)
	cfg.OTPMaxAttempts = cmp.Or(cfg.OTPMaxAttempts, 5)
	return cfg, nil
}

func newMailer(deps Deps, cfg config.Auth, client *http.Client, log *slog.Logger) (Mailer, error) {
	switch {
	case deps.Mailer != nil:
		return deps.Mailer, nil
	case cfg.ResendAPIKey != "":
		if cfg.EmailFrom == "" {
			return nil, errors.New("auth: AUTH_EMAIL_FROM is required with RESEND_API_KEY")
		}
		return resendMailer{client: client, baseURL: cmp.Or(deps.ResendBaseURL, DefaultResendBaseURL), apiKey: cfg.ResendAPIKey, from: cfg.EmailFrom}, nil
	case !isLocalEnv(deps.Config.Env):
		return nil, errors.New("auth: RESEND_API_KEY is required outside APP_ENV=dev or test")
	}
	dir, err := devMailDir(cfg.DevMailDir)
	if err != nil {
		return nil, err
	}
	return devMailer{log: log, dir: dir}, nil
}

func (s *Service) Config() config.Auth { return s.cfg }

func (s *Service) GoogleEnabled() bool { return s.google != nil }

func (s *Service) Wait() { s.mail.Wait() }

func (s *Service) SessionActive(ctx context.Context, sessionID string) (bool, error) {
	return s.familyActive(ctx, sessionID)
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (Principal, error) {
	p, err := s.signer.parse(accessToken)
	if err != nil {
		return Principal{}, err
	}
	if s.active.has(p.SessionID, s.now()) {
		return p, nil
	}
	active, err := s.familyActive(ctx, p.SessionID)
	if err != nil {
		return Principal{}, err
	}
	if !active {
		return Principal{}, ErrUnauthorized
	}
	s.active.remember(p.SessionID, s.now())
	return p, nil
}

func (s *Service) Me(ctx context.Context, userID string) (User, error) {
	u, found, err := userByID(ctx, s.pool, userID)
	if err != nil {
		return User{}, err
	}
	if !found {
		return User{}, ErrUnauthorized
	}
	return u.view(), nil
}

func (s *Service) complete(ctx context.Context, issued issuedSession) (Session, error) {
	user, err := s.Me(ctx, issued.userID)
	if err != nil {
		return Session{}, err
	}
	access, expires, err := s.signer.issue(issued.userID, issued.familyID)
	if err != nil {
		return Session{}, err
	}
	return Session{User: user, AccessToken: access, AccessExpiresAt: expires, RefreshToken: issued.refreshToken, RefreshExpiresAt: issued.familyExpiresAt}, nil
}

func (s *Service) limit(ctx context.Context, r rule, subject string) error {
	return s.limiter.allow(ctx, r, s.keys.subject(subject))
}

func (s *Service) limitIP(ctx context.Context, r rule, c Client) error {
	return s.limit(ctx, r, networkOf(c.IP))
}
