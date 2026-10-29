package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
)

type League struct {
	Code    string      `yaml:"code"`
	Name    string      `yaml:"name"`
	Country string      `yaml:"country"`
	Sport   canon.Sport `yaml:"sport"`
	Group   string      `yaml:"group"`
	Order   int         `yaml:"order"`
}

type SourceProfile struct {
	Feed             string   `yaml:"feed"`
	Type             string   `yaml:"type"`
	Note             string   `yaml:"note"`
	Countries        []string `yaml:"countries"`
	BlockedCountries []string `yaml:"blockedCountries"`
	NoBooking        bool     `yaml:"noBooking"`
}

type Tier struct {
	Name     string        `yaml:"name"`
	Within   time.Duration `yaml:"within"`
	Interval time.Duration `yaml:"interval"`
	Jitter   float64       `yaml:"jitter"`
}

type Pricing struct {
	DevigMethod       map[canon.MarketCode]string `yaml:"devigMethod"`
	DefaultDevig      string                      `yaml:"defaultDevig"`
	MinFairSources    int                         `yaml:"minFairSources"`
	ArbMinMargin      float64                     `yaml:"arbMinMargin"`
	EVMin             float64                     `yaml:"evMin"`
	MaxLegSkew        time.Duration               `yaml:"maxLegSkew"`
	MinTouchLiquidity float64                     `yaml:"minTouchLiquidity"`
	DeepBookLiquidity float64                     `yaml:"deepBookLiquidity"`
	DeepBookWeight    float64                     `yaml:"deepBookWeight"`
	MaxOverround      float64                     `yaml:"maxOverround"`
	EVProbRange       ProbRange                   `yaml:"evProbRange"`
	MinMoveHold       time.Duration               `yaml:"minMoveHold"`
	QuoteMaxAge       QuoteMaxAge                 `yaml:"quoteMaxAge"`
}

type QuoteMaxAge struct {
	Live      time.Duration            `yaml:"live"`
	PreMatch  []AgeTier                `yaml:"preMatch"`
	MinByType map[string]time.Duration `yaml:"minByType"`
}

type AgeTier struct {
	Within time.Duration `yaml:"within"`
	MaxAge time.Duration `yaml:"maxAge"`
}

func (a QuoteMaxAge) For(sourceType string, untilKickoff time.Duration) time.Duration {
	limit := a.Live
	if untilKickoff > 0 {
		limit = a.preMatch(untilKickoff)
	}
	if limit <= 0 {
		return 0
	}
	return max(limit, a.MinByType[sourceType])
}

func (a QuoteMaxAge) preMatch(untilKickoff time.Duration) time.Duration {
	for _, tier := range a.PreMatch {
		if untilKickoff <= tier.Within {
			return tier.MaxAge
		}
	}
	if len(a.PreMatch) == 0 {
		return 0
	}
	return a.PreMatch[len(a.PreMatch)-1].MaxAge
}

type ProbRange struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

func (r ProbRange) Contains(prob float64) bool {
	if r == (ProbRange{}) {
		return true
	}
	return prob >= r.Min && prob <= r.Max
}

type PlayerMerge struct {
	Interval time.Duration `yaml:"interval"`
}

type Telegram struct {
	Token            string        `yaml:"-"`
	MaxAlertsPerHour int           `yaml:"maxAlertsPerHour"`
	RealertGrowth    float64       `yaml:"realertGrowth"`
	MoveWindow       time.Duration `yaml:"moveWindow"`
	BotUsername      string        `yaml:"botUsername"`
}

type Auth struct {
	AccessTTL          time.Duration `yaml:"accessTtl"`
	RefreshTTL         time.Duration `yaml:"refreshTtl"`
	OTPTTL             time.Duration `yaml:"otpTtl"`
	OTPCooldown        time.Duration `yaml:"otpCooldown"`
	OTPMaxAttempts     int           `yaml:"otpMaxAttempts"`
	SecureCookies      bool          `yaml:"secureCookies"`
	TrustProxy         bool          `yaml:"trustProxy"`
	WebOrigin          string        `yaml:"webOrigin"`
	AllowedOrigins     []string      `yaml:"allowedOrigins"`
	EmailFrom          string        `yaml:"emailFrom"`
	GoogleRedirectURL  string        `yaml:"googleRedirectUrl"`
	JWTPrivateKey      string        `yaml:"-"`
	CodePepper         string        `yaml:"-"`
	GoogleClientID     string        `yaml:"-"`
	GoogleClientSecret string        `yaml:"-"`
	ResendAPIKey       string        `yaml:"-"`
	DevMailDir         string        `yaml:"-"`
}

type Config struct {
	Env          string                    `yaml:"env"`
	DatabaseURL  string                    `yaml:"-"`
	RedisURL     string                    `yaml:"-"`
	AdminToken   string                    `yaml:"-"`
	BrowserdURL  string                    `yaml:"browserdUrl"`
	HTTPAddr     string                    `yaml:"-"`
	GeoIPDB      string                    `yaml:"-"`
	Leagues      []League                  `yaml:"leagues"`
	Tiers        []Tier                    `yaml:"tiers"`
	Discovery    time.Duration             `yaml:"discovery"`
	Sources      map[string]sources.Config `yaml:"sources"`
	Feeds        map[string]string         `yaml:"-"`
	Profiles     map[string]SourceProfile  `yaml:"-"`
	Pricing      Pricing                   `yaml:"pricing"`
	Telegram     Telegram                  `yaml:"telegram"`
	EngineShards int                       `yaml:"engineShards"`
	FlushEvery   time.Duration             `yaml:"flushEvery"`
	FlushRows    int                       `yaml:"flushRows"`
	Auth         Auth                      `yaml:"auth"`
	PlayerMerge  PlayerMerge               `yaml:"playerMerge"`
}

const defaultPath = "config.yaml"

func Load() (Config, error) {
	path := envOr("CONFIG_PATH", defaultPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	expanded := []byte(os.ExpandEnv(string(raw)))
	var cfg Config
	if err := yaml.Unmarshal(expanded, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Profiles, err = sourceProfiles(expanded); err != nil {
		return Config{}, fmt.Errorf("parse source profiles %s: %w", path, err)
	}
	cfg.Feeds = feedsOf(cfg.Profiles)
	cfg.applyEnv()
	return cfg, nil
}

func sourceProfiles(raw []byte) (map[string]SourceProfile, error) {
	var file struct {
		Sources map[string]SourceProfile `yaml:"sources"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	return file.Sources, nil
}

func feedsOf(profiles map[string]SourceProfile) map[string]string {
	feeds := make(map[string]string, len(profiles))
	for name, profile := range profiles {
		if profile.Feed != "" {
			feeds[name] = profile.Feed
		}
	}
	return feeds
}

func (c *Config) applyEnv() {
	c.DatabaseURL = envOr("DATABASE_URL", "postgres://mybestbet:mybestbet@localhost:5433/mybestbet?sslmode=disable")
	c.RedisURL = envOr("REDIS_URL", "redis://localhost:6379/0")
	c.AdminToken = os.Getenv("ADMIN_TOKEN")
	c.HTTPAddr = envOr("HTTP_ADDR", ":8080")
	c.GeoIPDB = os.Getenv("GEOIP_DB")
	c.Telegram.Token = os.Getenv("TELEGRAM_BOT_TOKEN")
	c.applyAuthEnv()
	if v := os.Getenv("BROWSERD_URL"); v != "" {
		c.BrowserdURL = v
	}
	disabled := strings.Split(os.Getenv("SOURCES_DISABLED"), ",")
	for name, sc := range c.Sources {
		if slices.Contains(disabled, name) {
			sc.Enabled = false
		}
		prefix := "SOURCE_" + strings.ToUpper(name) + "_"
		sc.APIKey = os.Getenv(prefix + "API_KEY")
		sc.APISecret = os.Getenv(prefix + "API_SECRET")
		if v := os.Getenv(prefix + "PROXY"); v != "" {
			sc.Proxy = v
		}
		c.Sources[name] = sc
	}
}

func (c *Config) applyAuthEnv() {
	c.Auth.JWTPrivateKey = os.Getenv("AUTH_JWT_PRIVATE_KEY")
	c.Auth.CodePepper = os.Getenv("AUTH_CODE_PEPPER")
	c.Auth.GoogleClientID = os.Getenv("GOOGLE_CLIENT_ID")
	c.Auth.GoogleClientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	c.Auth.ResendAPIKey = os.Getenv("RESEND_API_KEY")
	c.Auth.DevMailDir = os.Getenv("AUTH_DEV_MAIL_DIR")
	if os.Getenv("AUTH_SECURE_COOKIES") == "" {
		c.Auth.SecureCookies = true
	}
}

func (c Config) LeagueCodes() []string {
	codes := make([]string, len(c.Leagues))
	for i, l := range c.Leagues {
		codes[i] = l.Code
	}
	return codes
}

func (c Config) LeagueSport(code string) canon.Sport {
	for _, l := range c.Leagues {
		if l.Code == code {
			return l.Sport
		}
	}
	return ""
}

func (c Config) Weight(source string) float64 {
	if w := c.Sources[source].Weight; w > 0 {
		return w
	}
	return 1
}

func (c Config) Feed(source string) string {
	if feed, ok := c.Feeds[source]; ok {
		return feed
	}
	return source
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
