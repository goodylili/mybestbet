package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	tokenIssuer   = "mybestbet"
	tokenAudience = "mybestbet"
	tokenLeeway   = 30 * time.Second
)

type Principal struct {
	UserID    string
	SessionID string
	ExpiresAt time.Time
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

type signer struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	kid     string
	ttl     time.Duration
	now     func() time.Time
}

func newSigner(seed []byte, ttl time.Duration, now func() time.Time) (*signer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("auth: jwt key must be a %d byte ed25519 seed", ed25519.SeedSize)
	}
	private := ed25519.NewKeyFromSeed(seed)
	public := private.Public().(ed25519.PublicKey)
	digest := sha256.Sum256(public)
	return &signer{private: private, public: public, kid: base64.RawURLEncoding.EncodeToString(digest[:8]), ttl: ttl, now: now}, nil
}

func (s *signer) issue(userID, sessionID string) (string, time.Time, error) {
	now := s.now()
	expires := now.Add(s.ttl)
	claims := accessClaims{
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.kid
	signed, err := token.SignedString(s.private)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expires, nil
}

func (s *signer) parse(raw string) (Principal, error) {
	if raw == "" {
		return Principal{}, ErrUnauthorized
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithAudience(tokenAudience),
		jwt.WithLeeway(tokenLeeway),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(s.now),
	)
	var claims accessClaims
	_, err := parser.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if kid, _ := token.Header["kid"].(string); kid != s.kid {
			return nil, ErrUnauthorized
		}
		return s.public, nil
	})
	if err != nil || claims.NotBefore == nil || uuid.Validate(claims.Subject) != nil || uuid.Validate(claims.SessionID) != nil {
		return Principal{}, ErrUnauthorized
	}
	return Principal{UserID: claims.Subject, SessionID: claims.SessionID, ExpiresAt: claims.ExpiresAt.UTC()}, nil
}
