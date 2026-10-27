package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/platform/config"
	"mybestbet/internal/platform/postgres"
	"mybestbet/internal/platform/redis"
)

const integrationPassword = "tangerine-sky-42"

type captureMailer struct {
	mu    sync.Mutex
	mails []Mail
}

func (m *captureMailer) Send(_ context.Context, mail Mail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mails = append(m.mails, mail)
	return nil
}

func (m *captureMailer) last(t *testing.T, s *Service, email string) Mail {
	t.Helper()
	s.Wait()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.mails) - 1; i >= 0; i-- {
		if m.mails[i].To == email {
			return m.mails[i]
		}
	}
	require.FailNow(t, "no mail", email)
	return Mail{}
}

func openService(t *testing.T) (*Service, *pgxpool.Pool, *captureMailer) {
	t.Helper()
	dbURL, redisURL := os.Getenv("TEST_DATABASE_URL"), os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL and TEST_REDIS_URL are required")
	}
	pool, err := postgres.Open(t.Context(), dbURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	rdb, err := redis.Open(t.Context(), redisURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rdb.Close() })
	mailer := &captureMailer{}
	cfg := config.Config{Env: "test", Auth: config.Auth{
		JWTPrivateKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		CodePepper:    "integration-pepper-integration-pepper",
	}}
	s, err := New(Deps{Config: cfg, Pool: pool, Redis: rdb, Mailer: mailer})
	require.NoError(t, err)
	t.Cleanup(s.Wait)
	return s, pool, mailer
}

func uniqueEmail(t *testing.T, pool *pgxpool.Pool) string {
	email := "svc-" + uuid.NewString()[:12] + "@web.test"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE email = $1`, email)
		_, _ = pool.Exec(context.Background(), `DELETE FROM email_codes WHERE email = $1`, email)
	})
	return email
}

func freshClient() Client {
	return Client{IP: fmt.Sprintf("10.%d.%d.%d", rand.IntN(256), rand.IntN(256), rand.IntN(254)+1), UserAgent: "test"}
}

func (s *Service) endCooldown(t *testing.T, email, purpose string) {
	t.Helper()
	require.NoError(t, s.rdb.Del(context.Background(), cooldownKey(s.keys, email, purpose)).Err())
}

func TestRefreshRotationReuseAndGrace(t *testing.T) {
	s, pool, _ := openService(t)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	hash, err := HashPassword(ctx, integrationPassword)
	require.NoError(t, err)
	userID, err := upsertVerifiedUser(ctx, pool, email, &hash, nil)
	require.NoError(t, err)
	issued, err := s.newSession(ctx, pool, userID, freshClient())
	require.NoError(t, err)
	first, err := s.complete(ctx, issued)
	require.NoError(t, err)

	second, err := s.Refresh(ctx, first.RefreshToken, freshClient())
	require.NoError(t, err)
	assert.NotEqual(t, first.RefreshToken, second.RefreshToken)
	assert.Equal(t, first.RefreshExpiresAt.Unix(), second.RefreshExpiresAt.Unix(), "the family keeps its absolute expiry")

	concurrent, err := s.Refresh(ctx, first.RefreshToken, freshClient())
	require.NoError(t, err)
	assert.Equal(t, second.RefreshToken, concurrent.RefreshToken, "grace window returns the same successor")

	third, err := s.Refresh(ctx, second.RefreshToken, freshClient())
	require.NoError(t, err)
	chained, err := s.Refresh(ctx, first.RefreshToken, freshClient())
	require.NoError(t, err)
	assert.Equal(t, third.RefreshToken, chained.RefreshToken, "grace follows a chain of fast rotations")

	_, err = pool.Exec(ctx, `UPDATE sessions SET "lastUsedAt" = now() - interval '1 minute' WHERE "familyId" = $1`, issued.familyID)
	require.NoError(t, err)
	_, err = s.Refresh(ctx, first.RefreshToken, freshClient())
	require.ErrorIs(t, err, ErrUnauthorized, "reuse after the grace window")
	_, err = s.Refresh(ctx, third.RefreshToken, freshClient())
	assert.ErrorIs(t, err, ErrUnauthorized, "reuse revokes the whole family")
	_, err = s.Authenticate(ctx, third.AccessToken)
	assert.ErrorIs(t, err, ErrUnauthorized, "access tokens of a revoked family stop working")

	var live int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE "familyId" = $1 AND "revokedAt" IS NULL`, issued.familyID).Scan(&live))
	assert.Zero(t, live)
}

func TestRefreshRejectsUnknownAndExpiredTokens(t *testing.T) {
	s, pool, _ := openService(t)
	ctx := context.Background()
	_, err := s.Refresh(ctx, "unknown", freshClient())
	assert.ErrorIs(t, err, ErrUnauthorized)
	email := uniqueEmail(t, pool)
	userID, err := upsertVerifiedUser(ctx, pool, email, nil, nil)
	require.NoError(t, err)
	issued, err := s.newSession(ctx, pool, userID, freshClient())
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE sessions SET "familyExpiresAt" = now() - interval '1 second' WHERE "familyId" = $1`, issued.familyID)
	require.NoError(t, err)
	_, err = s.Refresh(ctx, issued.refreshToken, freshClient())
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func signupCode(t *testing.T, s *Service, mailer *captureMailer, email, password, binding string) (CodeSent, Mail) {
	t.Helper()
	sent, err := s.Signup(context.Background(), SignupInput{Email: email, Password: password, Name: password, Binding: binding}, freshClient())
	require.NoError(t, err)
	return sent, mailer.last(t, s, email)
}

func TestSignupCannotSwapAPendingPassword(t *testing.T) {
	s, pool, mailer := openService(t)
	ctx := context.Background()

	t.Run("attacker after victim", func(t *testing.T) {
		email := uniqueEmail(t, pool)
		victim, mail := signupCode(t, s, mailer, email, "victim-password-1", "")
		require.NotEmpty(t, victim.Binding)
		s.endCooldown(t, email, PurposeSignup)
		_, err := s.Signup(ctx, SignupInput{Email: email, Password: "attacker-password-1"}, freshClient())
		require.NoError(t, err)
		assert.Equal(t, mail.IdempotencyKey, mailer.last(t, s, email).IdempotencyKey, "a foreign signup sends nothing")
		session, err := s.Verify(ctx, email, mail.Code, victim.Binding, freshClient())
		require.NoError(t, err)
		assert.Equal(t, "victim-password-1", session.User.Name)
		_, err = s.Login(ctx, email, "attacker-password-1", freshClient())
		assert.ErrorIs(t, err, ErrInvalidCredentials)
		_, err = s.Login(ctx, email, "victim-password-1", freshClient())
		assert.NoError(t, err)
	})

	t.Run("attacker before victim", func(t *testing.T) {
		email := uniqueEmail(t, pool)
		_, attackerMail := signupCode(t, s, mailer, email, "attacker-password-1", "")
		s.endCooldown(t, email, PurposeSignup)
		victim, err := s.Signup(ctx, SignupInput{Email: email, Password: "victim-password-1"}, freshClient())
		require.NoError(t, err)
		s.endCooldown(t, email, PurposeSignup)
		_, err = s.ResendCode(ctx, email, PurposeSignup, victim.Binding, freshClient())
		require.NoError(t, err)
		assert.Equal(t, attackerMail.IdempotencyKey, mailer.last(t, s, email).IdempotencyKey, "the victim cannot reissue a foreign code")
		_, err = s.Verify(ctx, email, attackerMail.Code, victim.Binding, freshClient())
		assert.ErrorIs(t, err, ErrInvalidCode, "the mailed code only works in the browser that chose the password")

		_, err = pool.Exec(ctx, `UPDATE email_codes SET "expiresAt" = now() - interval '1 second' WHERE email = $1`, email)
		require.NoError(t, err)
		s.endCooldown(t, email, PurposeSignup)
		_, err = s.Signup(ctx, SignupInput{Email: email, Password: "victim-password-1", Binding: victim.Binding}, freshClient())
		require.NoError(t, err)
		_, err = s.Verify(ctx, email, mailer.last(t, s, email).Code, victim.Binding, freshClient())
		require.NoError(t, err, "an expired foreign code is replaced")
		_, err = s.Login(ctx, email, "victim-password-1", freshClient())
		assert.NoError(t, err)
	})

	t.Run("same browser keeps the first password", func(t *testing.T) {
		email := uniqueEmail(t, pool)
		first, _ := signupCode(t, s, mailer, email, "first-password-1", "")
		s.endCooldown(t, email, PurposeSignup)
		_, second := signupCode(t, s, mailer, email, "second-password-2", first.Binding)
		s.endCooldown(t, email, PurposeSignup)
		sent, err := s.ResendCode(ctx, email, PurposeSignup, first.Binding, freshClient())
		require.NoError(t, err)
		assert.Equal(t, time.Minute, sent.ResendIn)
		resent := mailer.last(t, s, email)
		assert.NotEqual(t, second.IdempotencyKey, resent.IdempotencyKey, "each code has its own idempotency key")
		_, err = s.Verify(ctx, email, second.Code, first.Binding, freshClient())
		if second.Code != resent.Code {
			assert.ErrorIs(t, err, ErrInvalidCode, "the replaced code is dead")
		}
		session, err := s.Verify(ctx, email, resent.Code, first.Binding, freshClient())
		require.NoError(t, err)
		assert.Equal(t, "first-password-1", session.User.Name)
		_, err = s.Login(ctx, email, "first-password-1", freshClient())
		assert.NoError(t, err)
	})
}

func TestVerifyAttemptLimitAndBudget(t *testing.T) {
	s, pool, mailer := openService(t)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	sent, mail := signupCode(t, s, mailer, email, integrationPassword, "")
	wrong := func(code string) string {
		if code == "000000" {
			return "111111"
		}
		return "000000"
	}
	for range 5 {
		_, err := s.Verify(ctx, email, wrong(mail.Code), sent.Binding, freshClient())
		require.ErrorIs(t, err, ErrInvalidCode)
	}
	_, err := s.Verify(ctx, email, mail.Code, sent.Binding, freshClient())
	assert.ErrorIs(t, err, ErrInvalidCode, "five wrong attempts kill the code")

	_, err = pool.Exec(ctx, `UPDATE email_codes SET attempts = 0, "expiresAt" = now() - interval '1 second' WHERE email = $1`, email)
	require.NoError(t, err)
	_, err = s.Verify(ctx, email, mail.Code, sent.Binding, freshClient())
	assert.ErrorIs(t, err, ErrInvalidCode, "expired codes look like any other failure")

	s.endCooldown(t, email, PurposeSignup)
	_, err = s.Signup(ctx, SignupInput{Email: email, Password: integrationPassword, Binding: sent.Binding}, freshClient())
	require.NoError(t, err)
	fresh := mailer.last(t, s, email)
	for range 3 {
		_, err := s.Verify(ctx, email, wrong(fresh.Code), sent.Binding, freshClient())
		require.ErrorIs(t, err, ErrInvalidCode)
	}
	_, err = s.Verify(ctx, email, fresh.Code, sent.Binding, freshClient())
	var limited *RateLimitError
	require.ErrorAs(t, err, &limited, "the failure budget survives a reissued code")
	assert.Greater(t, limited.RetryAfter, 23*time.Hour)
	_, err = s.ResetPassword(ctx, email, fresh.Code, "a-brand-new-secret", freshClient())
	assert.ErrorIs(t, err, ErrInvalidCode, "the budget is per purpose")
}

func TestLoginLockIsPerEmailAndIP(t *testing.T) {
	s, pool, _ := openService(t)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	hash, err := HashPassword(ctx, integrationPassword)
	require.NoError(t, err)
	_, err = upsertVerifiedUser(ctx, pool, email, &hash, nil)
	require.NoError(t, err)
	attacker := freshClient()
	for range 5 {
		_, err := s.Login(ctx, email, "wrong-password-1", attacker)
		require.ErrorIs(t, err, ErrInvalidCredentials)
	}
	var limited *RateLimitError
	_, err = s.Login(ctx, email, integrationPassword, attacker)
	require.ErrorAs(t, err, &limited)
	_, err = s.Login(ctx, email, integrationPassword, freshClient())
	require.NoError(t, err, "the victim can still sign in from elsewhere")

	for range maxEmailLoginFailures - 5 {
		_, err := s.Login(ctx, email, "wrong-password-1", freshClient())
		require.ErrorIs(t, err, ErrInvalidCredentials)
	}
	_, err = s.Login(ctx, email, integrationPassword, freshClient())
	assert.ErrorAs(t, err, &limited, "a global per-email cap stops distributed guessing")
}

func TestLogoutAllRevokesEveryFamily(t *testing.T) {
	s, pool, _ := openService(t)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	hash, err := HashPassword(ctx, integrationPassword)
	require.NoError(t, err)
	_, err = upsertVerifiedUser(ctx, pool, email, &hash, nil)
	require.NoError(t, err)
	a, err := s.Login(ctx, email, integrationPassword, freshClient())
	require.NoError(t, err)
	b, err := s.Login(ctx, email, integrationPassword, freshClient())
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, b.AccessToken)
	require.NoError(t, err)
	require.NoError(t, s.Logout(ctx, a.RefreshToken, "", true))
	require.NoError(t, s.Logout(ctx, a.RefreshToken, "", true), "logout is idempotent")
	_, err = s.Authenticate(ctx, b.AccessToken)
	assert.ErrorIs(t, err, ErrUnauthorized)
	_, err = s.Refresh(ctx, b.RefreshToken, freshClient())
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestStaleTokenCannotLogoutEverySession(t *testing.T) {
	s, pool, _ := openService(t)
	ctx := context.Background()
	email := uniqueEmail(t, pool)
	hash, err := HashPassword(ctx, integrationPassword)
	require.NoError(t, err)
	_, err = upsertVerifiedUser(ctx, pool, email, &hash, nil)
	require.NoError(t, err)
	a, err := s.Login(ctx, email, integrationPassword, freshClient())
	require.NoError(t, err)
	b, err := s.Login(ctx, email, integrationPassword, freshClient())
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, a.AccessToken)
	require.NoError(t, err, "a second login keeps the first session alive")
	_, err = s.Refresh(ctx, a.RefreshToken, freshClient())
	require.NoError(t, err)

	require.NoError(t, s.Logout(ctx, a.RefreshToken, "", true))
	_, err = s.Authenticate(ctx, b.AccessToken)
	require.NoError(t, err, "a rotated refresh token cannot sign out other sessions")
	_, err = s.Authenticate(ctx, a.AccessToken)
	assert.ErrorIs(t, err, ErrUnauthorized, "it still ends its own family")

	require.NoError(t, s.Logout(ctx, "", a.AccessToken, true))
	_, err = s.Authenticate(ctx, b.AccessToken)
	require.NoError(t, err, "an access token of a revoked family cannot sign out other sessions")
}
