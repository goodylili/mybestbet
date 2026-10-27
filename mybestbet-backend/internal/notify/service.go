package notify

import (
	"context"
	"html"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

const telegramOffsetKey = "telegram:updates:offset"

type Deps struct {
	Config          config.Config
	Pool            *pgxpool.Pool
	Redis           *goredis.Client
	HTTPClient      *http.Client
	TelegramBaseURL string
	WebBaseURL      string
	Consumer        string
	Log             *slog.Logger
}

type Notifier struct {
	pool         *pgxpool.Pool
	rdb          *goredis.Client
	bot          *telegramClient
	limiter      *alertLimiter
	knownLeagues []string
	leagueSports map[string]canon.Sport
	webBaseURL   string
	consumer     string
	log          *slog.Logger
	clock        func() time.Time
}

func New(deps Deps) *Notifier {
	baseURL := deps.TelegramBaseURL
	if baseURL == "" {
		baseURL = DefaultTelegramBaseURL
	}
	return &Notifier{
		pool: deps.Pool,
		rdb:  deps.Redis,
		bot:  &telegramClient{baseURL: baseURL, token: deps.Config.Telegram.Token, http: deps.HTTPClient},
		limiter: &alertLimiter{
			rdb:           deps.Redis,
			maxPerHour:    deps.Config.Telegram.MaxAlertsPerHour,
			realertGrowth: deps.Config.Telegram.RealertGrowth,
			dedupeTTL:     alertDedupeTTL,
			rateWindow:    alertRateWindow,
			clock:         time.Now,
		},
		knownLeagues: deps.Config.LeagueCodes(),
		leagueSports: sportsByLeague(deps.Config.Leagues),
		webBaseURL:   deps.WebBaseURL,
		consumer:     deps.Consumer,
		log:          deps.Log,
		clock:        time.Now,
	}
}

func sportsByLeague(leagues []config.League) map[string]canon.Sport {
	sports := make(map[string]canon.Sport, len(leagues))
	for _, l := range leagues {
		sports[l.Code] = l.Sport
	}
	return sports
}

func (n *Notifier) Run(ctx context.Context) error {
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return n.pollCommands(groupCtx) })
	group.Go(func() error { return n.consumeSignals(groupCtx) })
	return group.Wait()
}

func (n *Notifier) handleUpdate(ctx context.Context, update telegramUpdate) error {
	if update.Message == nil {
		return nil
	}
	cmd, ok := parseCommand(update.Message.Text)
	if !ok {
		return nil
	}
	chatID := update.Message.Chat.ID
	current, err := n.subscription(ctx, chatID)
	if err != nil {
		return err
	}
	next, reply, err := applyCommand(current, cmd, commandEnv{now: n.clock(), knownLeagues: n.knownLeagues})
	if err != nil {
		return n.bot.sendMessage(ctx, chatID, html.EscapeString(err.Error()))
	}
	if err := n.saveSubscription(ctx, next); err != nil {
		return err
	}
	return n.bot.sendMessage(ctx, chatID, html.EscapeString(reply))
}
