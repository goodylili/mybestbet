package notify

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const subscriptionColumns = `"chatId", "minArb", "minEv", leagues, sports, props, "moveThreshold", "mutedUntil"`

func scanSubscription(row pgx.Row) (Subscription, error) {
	var s Subscription
	err := row.Scan(&s.ChatID, &s.MinArbPercent, &s.MinEVPercent, &s.Leagues, &s.Sports, &s.Props, &s.MovePercent, &s.MutedUntil)
	return s, err
}

func (n *Notifier) subscription(ctx context.Context, chatID int64) (Subscription, error) {
	sub, err := scanSubscription(n.pool.QueryRow(ctx, `SELECT `+subscriptionColumns+` FROM telegram_subscriptions WHERE "chatId" = $1`, chatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultSubscription(chatID), nil
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("load subscription: %w", err)
	}
	return sub, nil
}

func (n *Notifier) saveSubscription(ctx context.Context, s Subscription) error {
	_, err := n.pool.Exec(ctx, `INSERT INTO telegram_subscriptions (`+subscriptionColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT ("chatId") DO UPDATE SET "minArb" = EXCLUDED."minArb", "minEv" = EXCLUDED."minEv",
			leagues = EXCLUDED.leagues, sports = EXCLUDED.sports, props = EXCLUDED.props, "moveThreshold" = EXCLUDED."moveThreshold",
			"mutedUntil" = EXCLUDED."mutedUntil", "updatedAt" = now()`,
		s.ChatID, s.MinArbPercent, s.MinEVPercent, s.Leagues, s.Sports, s.Props, s.MovePercent, s.MutedUntil)
	if err != nil {
		return fmt.Errorf("save subscription: %w", err)
	}
	return nil
}

func (n *Notifier) subscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := n.pool.Query(ctx, `SELECT `+subscriptionColumns+` FROM telegram_subscriptions`)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	subs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Subscription, error) { return scanSubscription(row) })
	if err != nil {
		return nil, fmt.Errorf("scan subscriptions: %w", err)
	}
	return subs, nil
}

func (n *Notifier) alertContext(ctx context.Context, eventID, playerID string) (alertContext, error) {
	var c alertContext
	err := n.pool.QueryRow(ctx, `SELECT h.name, a.name, e.kickoff, coalesce((SELECT name FROM players WHERE id = nullif($2, '')::uuid), '')
		FROM events e JOIN teams h ON h.id = e."homeTeamId" JOIN teams a ON a.id = e."awayTeamId" WHERE e.id = $1`,
		eventID, playerID).Scan(&c.Home, &c.Away, &c.Kickoff, &c.PlayerName)
	if err != nil {
		return alertContext{}, fmt.Errorf("load alert context: %w", err)
	}
	return c, nil
}
