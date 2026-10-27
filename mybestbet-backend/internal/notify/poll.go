package notify

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const pollRetryDelay = 3 * time.Second

func (n *Notifier) pollCommands(ctx context.Context) error {
	offset, err := n.storedOffset(ctx)
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		updates, err := n.bot.updates(ctx, offset)
		if err != nil {
			n.log.WarnContext(ctx, "telegram poll failed", "err", err)
			sleepContext(ctx, pollRetryDelay)
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			if err := n.rdb.Set(ctx, telegramOffsetKey, offset, 0).Err(); err != nil {
				return fmt.Errorf("persist telegram offset: %w", err)
			}
			if err := n.handleUpdate(ctx, update); err != nil {
				n.log.WarnContext(ctx, "telegram command failed", "update_id", update.UpdateID, "err", err)
			}
		}
	}
	return nil
}

func (n *Notifier) storedOffset(ctx context.Context) (int64, error) {
	offset, err := n.rdb.Get(ctx, telegramOffsetKey).Int64()
	if errors.Is(err, goredis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load telegram offset: %w", err)
	}
	return offset, nil
}

func sleepContext(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
