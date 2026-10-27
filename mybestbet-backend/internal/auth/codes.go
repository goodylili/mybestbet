package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	goredis "github.com/redis/go-redis/v9"
)

const (
	PurposeSignup     = "signup"
	PurposeReset      = "reset"
	mailAttempts      = 3
	mailRetryDelay    = time.Second
	maxVerifyFailures = 10
	verifyFailWindow  = 24 * time.Hour
	bindingTokenLen   = 32
)

type SignupInput struct {
	Email    string
	Password string
	Name     string
	Binding  string
}

type issueFunc func(ctx context.Context, codeHash []byte, expiresAt time.Time) (id string, ok bool, err error)

func (c codeRow) check(k keys, purpose, email, code string, clientHash []byte, now time.Time, maxAttempts int) error {
	codeMatches := subtle.ConstantTimeCompare(k.codeHash(purpose, email, code), c.codeHash) == 1
	clientMatches := subtle.ConstantTimeCompare(clientHash, c.clientHash) == 1
	if !now.Before(c.expiresAt) || c.attempts > maxAttempts || !codeMatches || !clientMatches {
		return ErrInvalidCode
	}
	return nil
}

func bindingHash(token string) []byte {
	if token == "" {
		return nil
	}
	return hashToken(token)
}

func (s *Service) Signup(ctx context.Context, in SignupInput, c Client) (CodeSent, error) {
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return CodeSent{}, err
	}
	if err := ValidatePassword(in.Password); err != nil {
		return CodeSent{}, err
	}
	name, err := normalizeName(in.Name)
	if err != nil {
		return CodeSent{}, err
	}
	if err := s.limitCodes(ctx, email, c); err != nil {
		return CodeSent{}, err
	}
	binding := in.Binding
	if binding == "" {
		if binding, err = randomToken(bindingTokenLen); err != nil {
			return CodeSent{}, err
		}
	}
	hash, err := HashPassword(ctx, in.Password)
	if err != nil {
		return CodeSent{}, err
	}
	sent, err := s.requestCode(ctx, email, PurposeSignup, false, func(ctx context.Context, codeHash []byte, expiresAt time.Time) (string, bool, error) {
		if exists, err := verifiedUserExists(ctx, s.pool, email); err != nil || exists {
			return "", false, err
		}
		return upsertCode(ctx, s.pool, newCodeRow{email: email, purpose: PurposeSignup, codeHash: codeHash, clientHash: bindingHash(binding), passwordHash: &hash, name: name, expiresAt: expiresAt})
	})
	sent.Binding = binding
	return sent, err
}

func (s *Service) ForgotPassword(ctx context.Context, rawEmail string, c Client) (CodeSent, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return CodeSent{}, err
	}
	if err := s.limitCodes(ctx, email, c); err != nil {
		return CodeSent{}, err
	}
	return s.requestCode(ctx, email, PurposeReset, false, s.issueReset(email))
}

func (s *Service) ResendCode(ctx context.Context, rawEmail, purpose, binding string, c Client) (CodeSent, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return CodeSent{}, err
	}
	if purpose != PurposeSignup && purpose != PurposeReset {
		return CodeSent{}, invalidInput("purpose", "must be signup or reset")
	}
	if err := s.limitCodes(ctx, email, c); err != nil {
		return CodeSent{}, err
	}
	if purpose == PurposeReset {
		return s.requestCode(ctx, email, purpose, true, s.issueReset(email))
	}
	return s.requestCode(ctx, email, purpose, true, func(ctx context.Context, codeHash []byte, expiresAt time.Time) (string, bool, error) {
		if exists, err := verifiedUserExists(ctx, s.pool, email); err != nil || exists {
			return "", false, err
		}
		return reissueCode(ctx, s.pool, email, PurposeSignup, codeHash, bindingHash(binding), expiresAt)
	})
}

func (s *Service) issueReset(email string) issueFunc {
	return func(ctx context.Context, codeHash []byte, expiresAt time.Time) (string, bool, error) {
		_, found, err := userByEmail(ctx, s.pool, email)
		if err != nil || !found {
			return "", false, err
		}
		return upsertCode(ctx, s.pool, newCodeRow{email: email, purpose: PurposeReset, codeHash: codeHash, expiresAt: expiresAt})
	}
}

func (s *Service) Verify(ctx context.Context, rawEmail, code, binding string, c Client) (Session, error) {
	email, row, err := s.attempt(ctx, rawEmail, code, PurposeSignup, bindingHash(binding), c)
	if err != nil {
		return Session{}, err
	}
	var issued issuedSession
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := consumeCode(ctx, tx, row.id); err != nil {
			return err
		}
		userID, err := upsertVerifiedUser(ctx, tx, email, row.passwordHash, row.name)
		if err != nil {
			return err
		}
		issued, err = s.newSession(ctx, tx, userID, c)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return s.complete(ctx, issued)
}

func (s *Service) ResetPassword(ctx context.Context, rawEmail, code, password string, c Client) (Session, error) {
	if err := ValidatePassword(password); err != nil {
		return Session{}, err
	}
	email, row, err := s.attempt(ctx, rawEmail, code, PurposeReset, nil, c)
	if err != nil {
		return Session{}, err
	}
	hash, err := HashPassword(ctx, password)
	if err != nil {
		return Session{}, err
	}
	var issued issuedSession
	var revoked []string
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := consumeCode(ctx, tx, row.id); err != nil {
			return err
		}
		userID, err := setPassword(ctx, tx, email, hash)
		if err != nil {
			return err
		}
		if revoked, err = revokeUserSessions(ctx, tx, userID); err != nil {
			return err
		}
		issued, err = s.newSession(ctx, tx, userID, c)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	s.markRevoked(ctx, revoked...)
	s.clearLock(ctx, email, c)
	return s.complete(ctx, issued)
}

func (s *Service) attempt(ctx context.Context, rawEmail, code, purpose string, clientHash []byte, c Client) (string, codeRow, error) {
	email, err := normalizeEmail(rawEmail)
	if err != nil {
		return "", codeRow{}, err
	}
	if err := s.limit(ctx, verifyPerPair, email+"|"+networkOf(c.IP)); err != nil {
		return "", codeRow{}, err
	}
	failKey := verifyFailKey(s.keys, email, purpose)
	if err := s.underBudget(ctx, failKey, maxVerifyFailures); err != nil {
		return "", codeRow{}, err
	}
	row, err := s.checkCode(ctx, email, code, purpose, clientHash)
	if errors.Is(err, ErrInvalidCode) {
		if _, countErr := s.limiter.counter.incr(ctx, failKey, verifyFailWindow); countErr != nil {
			return "", codeRow{}, countErr
		}
	}
	return email, row, err
}

func (s *Service) checkCode(ctx context.Context, email, code, purpose string, clientHash []byte) (codeRow, error) {
	if len(code) != codeDigits {
		return codeRow{}, ErrInvalidCode
	}
	row, err := attemptCode(ctx, s.pool, email, purpose)
	if err != nil {
		return codeRow{}, err
	}
	return row, row.check(s.keys, purpose, email, code, clientHash, s.now(), s.cfg.OTPMaxAttempts)
}

func verifyFailKey(k keys, email, purpose string) string {
	return "auth:vfail:" + purpose + ":" + k.subject(email)
}

func (s *Service) underBudget(ctx context.Context, key string, limit int64) error {
	count, err := s.rdb.Get(ctx, key).Int64()
	if errors.Is(err, goredis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read failure budget: %w", err)
	}
	if count < limit {
		return nil
	}
	ttl, err := s.rdb.PTTL(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("read failure budget: %w", err)
	}
	return &RateLimitError{RetryAfter: max(ttl, time.Second)}
}

func (s *Service) limitCodes(ctx context.Context, email string, c Client) error {
	if err := s.limitIP(ctx, otpPerIP, c); err != nil {
		return err
	}
	return s.limit(ctx, otpPerEmail, email)
}

func (s *Service) requestCode(ctx context.Context, email, purpose string, strict bool, issue issueFunc) (CodeSent, error) {
	remaining, err := s.acquireCooldown(ctx, email, purpose)
	if err != nil {
		return CodeSent{}, err
	}
	if remaining > 0 {
		if strict {
			return CodeSent{}, &RateLimitError{RetryAfter: remaining}
		}
		return CodeSent{ResendIn: remaining}, nil
	}
	code, err := newCode()
	if err != nil {
		s.releaseCooldown(ctx, email, purpose)
		return CodeSent{}, err
	}
	id, ok, err := issue(ctx, s.keys.codeHash(purpose, email, code), s.now().Add(s.cfg.OTPTTL))
	if err != nil {
		s.releaseCooldown(ctx, email, purpose)
		return CodeSent{}, err
	}
	if ok {
		s.deliver(ctx, Mail{IdempotencyKey: id, To: email, Purpose: purpose, Code: code, TTL: s.cfg.OTPTTL})
	}
	return CodeSent{ResendIn: s.cfg.OTPCooldown}, nil
}

func (s *Service) deliver(ctx context.Context, mail Mail) {
	ctx = context.WithoutCancel(ctx)
	s.mail.Go(func() {
		var err error
		for attempt := range mailAttempts {
			if attempt > 0 {
				time.Sleep(mailRetryDelay * time.Duration(attempt))
			}
			sendCtx, cancel := context.WithTimeout(ctx, mailTimeout)
			err = s.mailer.Send(sendCtx, mail)
			cancel()
			if err == nil {
				if err := markCodeSent(ctx, s.pool, mail.IdempotencyKey); err != nil {
					s.log.WarnContext(ctx, "mark code sent failed", "err", err)
				}
				return
			}
		}
		s.log.ErrorContext(ctx, "auth code email failed", "purpose", mail.Purpose, "err", err)
		s.releaseCooldown(ctx, mail.To, mail.Purpose)
	})
}

func cooldownKey(k keys, email, purpose string) string {
	return "auth:cooldown:" + purpose + ":" + k.subject(email)
}

func (s *Service) acquireCooldown(ctx context.Context, email, purpose string) (time.Duration, error) {
	key := cooldownKey(s.keys, email, purpose)
	err := s.rdb.SetArgs(ctx, key, "1", goredis.SetArgs{Mode: "NX", TTL: s.cfg.OTPCooldown}).Err()
	if err == nil {
		return 0, nil
	}
	if !errors.Is(err, goredis.Nil) {
		return 0, err
	}
	ttl, err := s.rdb.PTTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return max(ttl, time.Second), nil
}

func (s *Service) releaseCooldown(ctx context.Context, email, purpose string) {
	if err := s.rdb.Del(ctx, cooldownKey(s.keys, email, purpose)).Err(); err != nil {
		s.log.WarnContext(ctx, "release code cooldown failed", "err", err)
	}
}
