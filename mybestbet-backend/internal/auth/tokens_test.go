package auth

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSigner(t *testing.T, now time.Time) *signer {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 7
	s, err := newSigner(seed, 15*time.Minute, func() time.Time { return now })
	require.NoError(t, err)
	return s
}

func forge(t *testing.T, s *signer, method jwt.SigningMethod, key any, mutate func(*accessClaims, *jwt.Token)) string {
	t.Helper()
	now := s.now()
	claims := accessClaims{SessionID: uuid.NewString(), RegisteredClaims: jwt.RegisteredClaims{
		Issuer: tokenIssuer, Audience: jwt.ClaimStrings{tokenAudience}, Subject: uuid.NewString(),
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)), ID: uuid.NewString(),
	}}
	token := jwt.NewWithClaims(method, &claims)
	token.Header["kid"] = s.kid
	if mutate != nil {
		mutate(&claims, token)
	}
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func TestSignerRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s := testSigner(t, now)
	userID, sessionID := uuid.NewString(), uuid.NewString()
	token, expires, err := s.issue(userID, sessionID)
	require.NoError(t, err)
	assert.Equal(t, now.Add(15*time.Minute), expires)
	p, err := s.parse(token)
	require.NoError(t, err)
	assert.Equal(t, Principal{UserID: userID, SessionID: sessionID, ExpiresAt: expires}, p)
}

func TestSignerRejectsForgedTokens(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s := testSigner(t, now)
	_, otherKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	valid := forge(t, s, jwt.SigningMethodEdDSA, s.private, nil)
	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "garbage", token: "not.a.jwt"},
		{name: "alg none", token: forge(t, s, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, nil)},
		{name: "hs256 keyed with the public key", token: forge(t, s, jwt.SigningMethodHS256, []byte(s.public), nil)},
		{name: "other ed25519 key", token: forge(t, s, jwt.SigningMethodEdDSA, otherKey, nil)},
		{name: "wrong kid", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(_ *accessClaims, tok *jwt.Token) { tok.Header["kid"] = "other" })},
		{name: "expired beyond leeway", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) {
			c.ExpiresAt = jwt.NewNumericDate(now.Add(-tokenLeeway - time.Second))
		})},
		{name: "missing expiry", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) { c.ExpiresAt = nil })},
		{name: "not yet valid", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) {
			c.NotBefore = jwt.NewNumericDate(now.Add(time.Minute))
		})},
		{name: "missing not before", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) { c.NotBefore = nil })},
		{name: "wrong audience", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) { c.Audience = jwt.ClaimStrings{"other"} })},
		{name: "wrong issuer", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) { c.Issuer = "other" })},
		{name: "subject not a uuid", token: forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) { c.Subject = "admin" })},
		{name: "tampered payload", token: tamper(valid)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := s.parse(tt.token)
			assert.ErrorIs(t, err, ErrUnauthorized)
		})
	}
}

func TestSignerAcceptsExpiryWithinLeeway(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s := testSigner(t, now)
	token := forge(t, s, jwt.SigningMethodEdDSA, s.private, func(c *accessClaims, _ *jwt.Token) {
		c.ExpiresAt = jwt.NewNumericDate(now.Add(-tokenLeeway / 2))
	})
	_, err := s.parse(token)
	assert.NoError(t, err)
}

func TestNewSignerRejectsShortSeed(t *testing.T) {
	t.Parallel()
	_, err := newSigner([]byte("short"), time.Minute, time.Now)
	assert.Error(t, err)
}

func tamper(token string) string {
	parts := strings.Split(token, ".")
	payload := []byte(parts[1])
	payload[len(payload)/2] ^= 1
	parts[1] = string(payload)
	return strings.Join(parts, ".")
}
