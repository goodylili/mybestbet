package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/auth"
)

const (
	googleClientID = "client-123.apps.googleusercontent.com"
	googleKeyID    = "test-key"
)

type googleIdentity struct {
	subject       string
	email         string
	emailVerified bool
}

type googleMock struct {
	*httptest.Server
	key        *rsa.PrivateKey
	mu         sync.Mutex
	identity   googleIdentity
	challenges map[string]string
	nonces     map[string]string
}

func newGoogleMock(t *testing.T) *googleMock {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	m := &googleMock{key: key, challenges: map[string]string{}, nonces: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": googleKeyID,
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		m.mu.Lock()
		challenge, nonce := m.challenges[r.Form.Get("code")], m.nonces[r.Form.Get("code")]
		identity := m.identity
		m.mu.Unlock()
		verifier := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if challenge == "" || base64.RawURLEncoding.EncodeToString(verifier[:]) != challenge || r.Form.Get("client_secret") != "google-secret" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		now := time.Now()
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": m.URL, "aud": googleClientID, "sub": identity.subject, "email": identity.email, "email_verified": identity.emailVerified,
			"nonce": nonce, "name": "Grace Hopper", "picture": "https://lh3.googleusercontent.com/a/photo", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		})
		token.Header["kid"] = googleKeyID
		signed, err := token.SignedString(key)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": signed})
	})
	m.Server = httptest.NewServer(mux)
	t.Cleanup(m.Close)
	return m
}

func (m *googleMock) endpoints() auth.GoogleEndpoints {
	return auth.GoogleEndpoints{Issuer: m.URL, AuthURL: m.URL + "/authorize", TokenURL: m.URL + "/token", JWKSURL: m.URL + "/jwks"}
}

func (m *googleMock) approve(t *testing.T, location string, identity googleIdentity) (code, state string) {
	t.Helper()
	redirect, err := url.Parse(location)
	require.NoError(t, err)
	query := redirect.Query()
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.Equal(t, googleClientID, query.Get("client_id"))
	assert.Contains(t, query.Get("scope"), "openid")
	require.NotEmpty(t, query.Get("nonce"))
	code = uuid.NewString()
	m.mu.Lock()
	m.identity = identity
	m.challenges[code] = query.Get("code_challenge")
	m.nonces[code] = query.Get("nonce")
	m.mu.Unlock()
	return code, query.Get("state")
}

func googleSignIn(t *testing.T, h *authHarness, google *googleMock, b *browser, next string, identity googleIdentity) (*http.Response, string) {
	t.Helper()
	resp, _ := b.do(http.MethodGet, "/v1/auth/google/start?next="+url.QueryEscape(next), "")
	require.Equal(t, http.StatusFound, resp.StatusCode)
	location := resp.Header.Get("Location")
	require.Contains(t, location, google.URL+"/authorize")
	code, state := google.approve(t, location, identity)
	callback := "/v1/auth/google/callback?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state)
	resp, _ = b.do(http.MethodGet, callback, "")
	require.Equal(t, http.StatusFound, resp.StatusCode)
	return resp, callback
}

func TestGoogleSignIn(t *testing.T) {
	google := newGoogleMock(t)
	h := newAuthHarness(t, google.endpoints(), googleClientID)
	errorTarget := testWebOrigin + "/login?error=google"

	t.Run("meta advertises google", func(t *testing.T) {
		_, body := newBrowser(t, h.server.URL).do(http.MethodGet, "/v1/meta", "")
		assert.Equal(t, map[string]any{"google": true}, body["auth"])
	})

	t.Run("new user lands on next and state is single use", func(t *testing.T) {
		email := testEmail()
		cleanupEmail(t, h.pool, email)
		b := newBrowser(t, h.server.URL)
		resp, callback := googleSignIn(t, h, google, b, "/sports/basketball?league=NBA", googleIdentity{subject: uuid.NewString(), email: email, emailVerified: true})
		assert.Equal(t, testWebOrigin+"/sports/basketball?league=NBA", resp.Header.Get("Location"))
		resp, body := b.do(http.MethodGet, "/v1/me", "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		user := asMap(body["user"])
		assert.Equal(t, []any{"google"}, user["providers"])
		assert.Equal(t, true, user["emailVerified"])
		assert.Equal(t, false, user["hasPassword"])
		assert.Equal(t, "Grace Hopper", user["name"])

		resp, _ = newBrowser(t, h.server.URL).do(http.MethodGet, callback, "")
		assert.Equal(t, errorTarget, resp.Header.Get("Location"))
	})

	t.Run("links an existing password account by verified email", func(t *testing.T) {
		email := testEmail()
		createUser(t, h.pool, email)
		subject := uuid.NewString()
		b := newBrowser(t, h.server.URL)
		resp, _ := googleSignIn(t, h, google, b, "//evil.test/x", googleIdentity{subject: subject, email: email, emailVerified: true})
		assert.Equal(t, testWebOrigin+auth.DefaultNext, resp.Header.Get("Location"))
		_, body := b.do(http.MethodGet, "/v1/me", "")
		assert.Equal(t, []any{"password", "google"}, asMap(body["user"])["providers"])

		again := newBrowser(t, h.server.URL)
		googleSignIn(t, h, google, again, "/", googleIdentity{subject: subject, email: email, emailVerified: true})
		_, second := again.do(http.MethodGet, "/v1/me", "")
		assert.Equal(t, asMap(body["user"])["id"], asMap(second["user"])["id"])
	})

	t.Run("unverified google email is refused", func(t *testing.T) {
		b := newBrowser(t, h.server.URL)
		resp, _ := googleSignIn(t, h, google, b, "/", googleIdentity{subject: uuid.NewString(), email: testEmail(), emailVerified: false})
		assert.Equal(t, errorTarget, resp.Header.Get("Location"))
		assert.Empty(t, b.cookies)
	})

	t.Run("unknown state is refused", func(t *testing.T) {
		resp, _ := newBrowser(t, h.server.URL).do(http.MethodGet, "/v1/auth/google/callback?code=x&state=forged", "")
		assert.Equal(t, errorTarget, resp.Header.Get("Location"))
	})
}

func TestGoogleStartWithoutGoogleConfigured(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	resp, _ := newBrowser(t, h.server.URL).do(http.MethodGet, "/v1/auth/google/start?next=/x", "")
	require.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, testWebOrigin+"/login?error=google", resp.Header.Get("Location"))
}

func TestGoogleStateIsBoundToTheBrowser(t *testing.T) {
	google := newGoogleMock(t)
	h := newAuthHarness(t, google.endpoints(), googleClientID)
	errorTarget := testWebOrigin + "/login?error=google"
	identity := googleIdentity{subject: uuid.NewString(), email: testEmail(), emailVerified: true}
	cleanupEmail(t, h.pool, identity.email)

	attacker := newBrowser(t, h.server.URL)
	resp, _ := attacker.do(http.MethodGet, "/v1/auth/google/start?next=/", "")
	require.Equal(t, http.StatusFound, resp.StatusCode)
	oauthCookie := attacker.cookies[oauthCookieName]
	require.NotNil(t, oauthCookie)
	assert.True(t, oauthCookie.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, oauthCookie.SameSite)
	assert.Equal(t, int(auth.OAuthStateTTL.Seconds()), oauthCookie.MaxAge)
	code, state := google.approve(t, resp.Header.Get("Location"), identity)
	callback := "/v1/auth/google/callback?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state)

	victim := newBrowser(t, h.server.URL)
	resp, _ = victim.do(http.MethodGet, "/v1/auth/google/start?next=/", "")
	require.Equal(t, http.StatusFound, resp.StatusCode)
	resp, _ = victim.do(http.MethodGet, callback, "")
	assert.Equal(t, errorTarget, resp.Header.Get("Location"), "another browser's state is refused")
	assert.NotContains(t, victim.cookies, accessCookieName)
	assert.NotContains(t, victim.cookies, oauthCookieName, "the callback clears the oauth cookie")

	resp, _ = newBrowser(t, h.server.URL).do(http.MethodGet, callback, "")
	assert.Equal(t, errorTarget, resp.Header.Get("Location"), "a missing cookie is refused")
}

func TestGoogleStartIsRateLimitedPerIP(t *testing.T) {
	google := newGoogleMock(t)
	h := newAuthHarness(t, google.endpoints(), googleClientID)
	b := newBrowser(t, h.server.URL)
	for range 20 {
		resp, _ := b.do(http.MethodGet, "/v1/auth/google/start", "")
		require.Equal(t, http.StatusFound, resp.StatusCode)
	}
	resp, body := b.do(http.MethodGet, "/v1/auth/google/start", "")
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	h.conforms(t, "/v1/auth/google/start", "get", resp, body)
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))
}
