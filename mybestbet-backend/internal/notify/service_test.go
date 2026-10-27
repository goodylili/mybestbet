package notify

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/pkg/canon"
)

func newTestNotifier(t *testing.T, pool *pgxpool.Pool, telegramURL string) *Notifier {
	t.Helper()
	cfg := config.Config{
		Leagues:  []config.League{{Code: "EPL"}, {Code: "LALIGA"}},
		Telegram: config.Telegram{Token: testToken, MaxAlertsPerHour: 30, RealertGrowth: 0.5},
	}
	return New(Deps{Config: cfg, Pool: pool, Redis: testRedis(t), HTTPClient: &http.Client{Timeout: 5 * time.Second}, TelegramBaseURL: telegramURL, WebBaseURL: "https://odds.example", Consumer: "test", Log: slog.Default()})
}

func insertTestEvent(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO leagues (code, name, country) VALUES ('EPL', 'Premier League', 'England') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	suffix := uuid.NewString()[:8]
	var home, away, event string
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, "Home "+suffix).Scan(&home))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO teams ("leagueCode", name, "normalizedName") VALUES ('EPL', $1, $1) RETURNING id::text`, "Away "+suffix).Scan(&away))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO events ("leagueCode", "homeTeamId", "awayTeamId", kickoff) VALUES ('EPL', $1, $2, $3) RETURNING id::text`,
		home, away, time.Now().Add(24*time.Hour)).Scan(&event))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM events WHERE id = $1`, event)
		_, _ = pool.Exec(context.Background(), `DELETE FROM teams WHERE id IN ($1, $2)`, home, away)
	})
	return event
}

func TestHandleUpdatePersistsSubscriptionIdempotently(t *testing.T) {
	pool := testPool(t)
	fake := &fakeTelegram{}
	n := newTestNotifier(t, pool, fake.serve(t).URL)
	ctx := context.Background()
	chatID := -time.Now().UnixNano()
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM telegram_subscriptions WHERE "chatId" = $1`, chatID) })

	for _, text := range []string{"/start", "/arbs 1.5", "/league epl", "/league epl", "/props on", "/sport basketball tennis", "/sport basketball tennis"} {
		require.NoError(t, n.handleUpdate(ctx, telegramUpdate{Message: &telegramMessage{Chat: telegramChat{ID: chatID}, Text: text}}))
	}
	sub, err := n.subscription(ctx, chatID)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, sub.MinArbPercent, 1e-9)
	assert.InDelta(t, 3.0, sub.MinEVPercent, 1e-9)
	assert.Equal(t, []string{"EPL"}, sub.Leagues)
	assert.True(t, sub.Props)
	assert.Equal(t, []string{"basketball", "tennis"}, sub.Sports)
	assert.Len(t, fake.messages(), 7)

	require.NoError(t, n.handleUpdate(ctx, telegramUpdate{Message: &telegramMessage{Chat: telegramChat{ID: chatID}, Text: "/league <b>"}}))
	last := fake.messages()[7]
	assert.Contains(t, last.Text, "&lt;b&gt;")
}

func TestAlertSubscribersDeliversOncePerChat(t *testing.T) {
	pool := testPool(t)
	fake := &fakeTelegram{}
	n := newTestNotifier(t, pool, fake.serve(t).URL)
	ctx := context.Background()
	eventID := insertTestEvent(t, pool)
	chatID := -time.Now().UnixNano()
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM telegram_subscriptions WHERE "chatId" = $1`, chatID) })
	require.NoError(t, n.saveSubscription(ctx, defaultSubscription(chatID)))

	signal := canon.Signal{
		ID: uuid.NewString(), DedupeKey: "arb:" + uuid.NewString(), Type: canon.SignalArb, Status: canon.SignalOpen,
		EventID: eventID, LeagueCode: "EPL", Margin: 0.02, Market: canon.MarketKey{EventID: eventID, Code: canon.Code1X2},
		Legs: []canon.Leg{{Source: "bet9ja", Side: canon.SideHome, Price: canon.NewPrice(2.2, canon.FormDecimal, time.Now()), Stake: 47}},
	}
	require.NoError(t, n.alertSubscribers(ctx, signal))
	require.NoError(t, n.alertSubscribers(ctx, signal))
	signal.Margin = 0.031
	require.NoError(t, n.alertSubscribers(ctx, signal))

	var mine []sentMessage
	for _, m := range fake.messages() {
		if m.ChatID == chatID {
			mine = append(mine, m)
		}
	}
	require.Len(t, mine, 2)
	assert.True(t, strings.HasPrefix(mine[0].Text, "<b>ARB 2.00%</b>"))
	assert.Contains(t, mine[1].Text, "ARB 3.10%")
	assert.Contains(t, mine[0].Text, "https://odds.example/event/"+eventID)
}

func TestPollCommandsPersistsOffset(t *testing.T) {
	pool := testPool(t)
	fake := &fakeTelegram{}
	n := newTestNotifier(t, pool, fake.serve(t).URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chatID := -time.Now().UnixNano()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM telegram_subscriptions WHERE "chatId" = $1`, chatID)
	})
	require.NoError(t, n.rdb.Set(ctx, telegramOffsetKey, 41, 0).Err())
	fake.mu.Lock()
	fake.updates = []telegramUpdate{{UpdateID: 41, Message: &telegramMessage{Chat: telegramChat{ID: chatID}, Text: "/ev 4"}}}
	fake.mu.Unlock()

	done := make(chan error, 1)
	go func() { done <- n.pollCommands(ctx) }()
	require.Eventually(t, func() bool {
		sub, err := n.subscription(ctx, chatID)
		return err == nil && sub.MinEVPercent == 4
	}, 5*time.Second, 20*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	offset, err := n.storedOffset(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(42), offset)
}
