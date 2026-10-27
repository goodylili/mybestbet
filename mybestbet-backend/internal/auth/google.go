package auth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"mybestbet/internal/platform/config"
)

const (
	DefaultNext   = "/sports/football"
	maxNextLen    = 2048
	OAuthStateTTL = 10 * time.Minute
)

var errGoogle = errors.New("auth: google sign-in failed")

type GoogleEndpoints struct {
	Issuer   string
	AuthURL  string
	TokenURL string
	JWKSURL  string
}

var defaultGoogleEndpoints = GoogleEndpoints{ //nolint:gosec
	Issuer:   "https://accounts.google.com",
	AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
	JWKSURL:  "https://www.googleapis.com/oauth2/v3/certs",
}

type googleClient struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	http     *http.Client
}

type oauthState struct {
	Verifier string `json:"verifier"`
	Nonce    string `json:"nonce"`
	Next     string `json:"next"`
}

type googleClaims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func newGoogleClient(cfg config.Auth, endpoints GoogleEndpoints, client *http.Client) *googleClient {
	if cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		return nil
	}
	if endpoints == (GoogleEndpoints{}) {
		endpoints = defaultGoogleEndpoints
	}
	keySet := oidc.NewRemoteKeySet(oidc.ClientContext(context.Background(), client), endpoints.JWKSURL)
	return &googleClient{
		oauth: oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			Endpoint:     oauth2.Endpoint{AuthURL: endpoints.AuthURL, TokenURL: endpoints.TokenURL, AuthStyle: oauth2.AuthStyleInParams},
			RedirectURL:  cfg.GoogleRedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		verifier: oidc.NewVerifier(endpoints.Issuer, keySet, &oidc.Config{ClientID: cfg.GoogleClientID, SupportedSigningAlgs: []string{oidc.RS256}}),
		http:     client,
	}
}

func SafeNext(next string) string {
	invalid := next == "" || len(next) > maxNextLen || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") ||
		strings.ContainsAny(next, "\\") || strings.ContainsFunc(next, isControl)
	if invalid {
		return DefaultNext
	}
	parsed, err := url.Parse(next)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil {
		return DefaultNext
	}
	return next
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }

func oauthKey(state string) string { return "auth:oauth:" + state }

type GoogleRedirect struct {
	URL          string
	StateBinding string
}

func StateBinding(state string) string {
	return base64.RawURLEncoding.EncodeToString(hashToken(state))
}

func (s *Service) GoogleStart(ctx context.Context, next string, c Client) (GoogleRedirect, error) {
	if err := s.limitIP(ctx, googlePerIP, c); err != nil {
		return GoogleRedirect{}, err
	}
	if s.google == nil {
		return GoogleRedirect{}, errGoogle
	}
	state, err := randomToken(stateTokenLen)
	if err != nil {
		return GoogleRedirect{}, err
	}
	nonce, err := randomToken(stateTokenLen)
	if err != nil {
		return GoogleRedirect{}, err
	}
	stored := oauthState{Verifier: oauth2.GenerateVerifier(), Nonce: nonce, Next: SafeNext(next)}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return GoogleRedirect{}, fmt.Errorf("encode oauth state: %w", err)
	}
	if err := s.rdb.Set(ctx, oauthKey(state), encoded, OAuthStateTTL).Err(); err != nil {
		return GoogleRedirect{}, fmt.Errorf("store oauth state: %w", err)
	}
	return GoogleRedirect{
		URL:          s.google.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(stored.Verifier), oidc.Nonce(nonce)),
		StateBinding: StateBinding(state),
	}, nil
}

func (s *Service) GoogleCallback(ctx context.Context, code, state, binding string, c Client) (Session, string, error) {
	if s.google == nil || code == "" || state == "" {
		return Session{}, "", errGoogle
	}
	if subtle.ConstantTimeCompare([]byte(binding), []byte(StateBinding(state))) != 1 {
		return Session{}, "", fmt.Errorf("%w: state not bound to this browser", errGoogle)
	}
	raw, err := s.rdb.GetDel(ctx, oauthKey(state)).Bytes()
	if err != nil {
		return Session{}, "", fmt.Errorf("%w: unknown state", errGoogle)
	}
	var stored oauthState
	if err := json.Unmarshal(raw, &stored); err != nil {
		return Session{}, "", fmt.Errorf("%w: decode state", errGoogle)
	}
	profile, err := s.google.profile(ctx, code, stored)
	if err != nil {
		return Session{}, "", err
	}
	var issued issuedSession
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		userID, err := linkGoogleUser(ctx, tx, profile)
		if err != nil {
			return err
		}
		issued, err = s.newSession(ctx, tx, userID, c)
		return err
	})
	if err != nil {
		return Session{}, "", err
	}
	session, err := s.complete(ctx, issued)
	return session, stored.Next, err
}

func (g *googleClient) profile(ctx context.Context, code string, stored oauthState) (googleProfile, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, g.http)
	token, err := g.oauth.Exchange(ctx, code, oauth2.VerifierOption(stored.Verifier))
	if err != nil {
		return googleProfile{}, fmt.Errorf("%w: exchange: %w", errGoogle, err)
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		return googleProfile{}, fmt.Errorf("%w: missing id token", errGoogle)
	}
	idToken, err := g.verifier.Verify(ctx, rawID)
	if err != nil {
		return googleProfile{}, fmt.Errorf("%w: verify id token: %w", errGoogle, err)
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(stored.Nonce)) != 1 {
		return googleProfile{}, fmt.Errorf("%w: nonce mismatch", errGoogle)
	}
	var claims googleClaims
	if err := idToken.Claims(&claims); err != nil {
		return googleProfile{}, fmt.Errorf("%w: claims: %w", errGoogle, err)
	}
	if !claims.EmailVerified || idToken.Subject == "" {
		return googleProfile{}, fmt.Errorf("%w: email not verified", errGoogle)
	}
	email, err := normalizeEmail(claims.Email)
	if err != nil {
		return googleProfile{}, fmt.Errorf("%w: email: %w", errGoogle, err)
	}
	name, _ := normalizeName(claims.Name)
	return googleProfile{subject: idToken.Subject, email: email, name: name, avatarURL: httpsURL(claims.Picture)}, nil
}

func httpsURL(raw string) *string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(raw) > maxNextLen {
		return nil
	}
	return &raw
}
