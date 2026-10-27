package auth

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	maxLoginFailures      = 5
	lockDuration          = 15 * time.Minute
	maxEmailLoginFailures = 50
	emailFailureWindow    = time.Hour
)

type lockKeys struct {
	fail       string
	lock       string
	emailFails string
}

func (s *Service) lockKeys(email string, c Client) lockKeys {
	pair := s.keys.subject(email + "|" + networkOf(c.IP))
	return lockKeys{fail: "auth:fail:" + pair, lock: "auth:lock:" + pair, emailFails: "auth:efail:" + s.keys.subject(email)}
}

func (s *Service) Login(ctx context.Context, rawEmail, password string, c Client) (Session, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return Session{}, ErrInvalidCredentials
	}
	if err := s.limitIP(ctx, loginPerIP, c); err != nil {
		return Session{}, err
	}
	locks := s.lockKeys(email, c)
	if err := s.underBudget(ctx, locks.emailFails, maxEmailLoginFailures); err != nil {
		return Session{}, err
	}
	if remaining, err := s.ttl(ctx, locks.lock); err != nil || remaining > 0 {
		return Session{}, lockedError(remaining, err)
	}
	user, found, err := userByEmail(ctx, s.pool, email)
	if err != nil {
		return Session{}, err
	}
	hasPassword := found && user.passwordHash != nil
	stored := dummyHash()
	if hasPassword {
		stored = *user.passwordHash
	}
	matches, err := VerifyPassword(ctx, stored, password)
	if err != nil {
		return Session{}, err
	}
	if !matches || !hasPassword {
		return Session{}, s.loginFailed(ctx, locks)
	}
	s.clearLock(ctx, email, c)
	issued, err := s.newSession(ctx, s.pool, user.id, c)
	if err != nil {
		return Session{}, err
	}
	return s.complete(ctx, issued)
}

func lockedError(remaining time.Duration, err error) error {
	if err != nil {
		return err
	}
	return &RateLimitError{RetryAfter: remaining}
}

func (s *Service) ttl(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := s.rdb.PTTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return max(ttl, 0), nil
}

func (s *Service) loginFailed(ctx context.Context, locks lockKeys) error {
	if _, err := s.limiter.counter.incr(ctx, locks.emailFails, emailFailureWindow); err != nil {
		return err
	}
	failures, err := s.limiter.counter.incr(ctx, locks.fail, lockDuration)
	if err != nil {
		return err
	}
	if failures >= maxLoginFailures {
		pipe := s.rdb.TxPipeline()
		pipe.Set(ctx, locks.lock, "1", lockDuration)
		pipe.Del(ctx, locks.fail)
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	return ErrInvalidCredentials
}

func (s *Service) clearLock(ctx context.Context, email string, c Client) {
	locks := s.lockKeys(email, c)
	err := s.rdb.Del(ctx, locks.fail, locks.lock).Err()
	if err != nil && !errors.Is(err, goredis.Nil) {
		s.log.WarnContext(ctx, "clear login lock failed", "err", err)
	}
}
