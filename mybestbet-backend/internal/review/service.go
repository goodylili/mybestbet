package review

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"mybestbet/internal/platform/redis"
	"mybestbet/pkg/canon"
)

const (
	defaultListLimit = 100
	maxListLimit     = 500
)

type Service struct {
	pool *pgxpool.Pool
	rdb  *goredis.Client
}

func NewService(pool *pgxpool.Pool, rdb *goredis.Client) *Service {
	return &Service{pool: pool, rdb: rdb}
}

func (s *Service) Enqueue(ctx context.Context, it NewItem) error {
	return upsertPendingItem(ctx, s.pool, it)
}

func (s *Service) List(ctx context.Context, status string, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	return listItems(ctx, s.pool, status, min(limit, maxListLimit))
}

func (s *Service) Decide(ctx context.Context, d Decision) (Item, error) {
	status, err := statusForAction(d.Action)
	if err != nil {
		return Item{}, err
	}
	if err := validateDecisionIDs(d); err != nil {
		return Item{}, err
	}
	var (
		decided     Item
		aliasUpdate *canon.AliasUpdate
	)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		current, err := lockItem(ctx, tx, d.ReviewID)
		if err != nil {
			return err
		}
		if current.Status != StatusPending {
			if !isSameDecision(current, d, status) {
				return ErrAlreadyDecided
			}
			decided = current.Item
			return nil
		}
		aliasUpdate, err = applyDecision(ctx, tx, current, d)
		if err != nil {
			return err
		}
		decided, err = markDecided(ctx, tx, current.ID, status, targetOf(aliasUpdate))
		return err
	})
	if err != nil {
		return Item{}, fmt.Errorf("decide review %s: %w", d.ReviewID, err)
	}
	if aliasUpdate != nil {
		if err := s.publishAliasUpdate(ctx, *aliasUpdate); err != nil {
			return decided, err
		}
	}
	return decided, nil
}

func statusForAction(action string) (string, error) {
	switch action {
	case ActionLink:
		return StatusLinked, nil
	case ActionCreate:
		return StatusCreated, nil
	case ActionIgnore:
		return StatusIgnored, nil
	}
	return "", fmt.Errorf("%w: unknown action %q", ErrInvalidDecision, action)
}

func validateDecisionIDs(d Decision) error {
	if _, err := uuid.Parse(d.ReviewID); err != nil {
		return fmt.Errorf("%w: review id is not a uuid", ErrInvalidDecision)
	}
	if d.Action != ActionLink {
		return nil
	}
	if _, err := uuid.Parse(d.TargetID); err != nil {
		return fmt.Errorf("%w: link target is not a uuid", ErrInvalidDecision)
	}
	return nil
}

func isSameDecision(current storedItem, d Decision, status string) bool {
	return current.Status == status && (d.Action != ActionLink || current.DecidedID == d.TargetID)
}

func targetOf(u *canon.AliasUpdate) string {
	if u == nil {
		return ""
	}
	return u.TargetID
}

func (s *Service) publishAliasUpdate(ctx context.Context, u canon.AliasUpdate) error {
	body, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("encode alias update: %w", err)
	}
	if err := s.rdb.Publish(ctx, redis.ChannelAliasUpdate, body).Err(); err != nil {
		return fmt.Errorf("publish alias update: %w", err)
	}
	return nil
}
