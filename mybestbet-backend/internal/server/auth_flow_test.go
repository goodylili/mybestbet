package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	json "github.com/goccy/go-json"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/auth"
	"mybestbet/internal/platform/config"
)

type authHarness struct {
	server *httptest.Server
	auth   *auth.Service
	resend *resendMock
	pool   *pgxpool.Pool
	spec   *openAPISpec
}

func newAuthHarness(t *testing.T, google auth.GoogleEndpoints, googleClient string) *authHarness {
	t.Helper()
	pool, rdb := integrationDeps(t)
	cfg := config.Config{Env: "test", Auth: testAuthConfig()}
	if googleClient != "" {
		cfg.Auth.GoogleClientID, cfg.Auth.GoogleClientSecret = googleClient, "google-secret"
		cfg.Auth.GoogleRedirectURL = testWebOrigin + "/api/v1/auth/google/callback"
	}
	resend := newResendMock(t)
	svc := newTestAuth(t, cfg, pool, rdb, resend.URL, google)
	server := httptest.NewServer(New(Deps{Config: cfg, Pool: pool, Redis: rdb, Auth: svc, Log: slog.Default()}).Handler())
	t.Cleanup(server.Close)
	return &authHarness{server: server, auth: svc, resend: resend, pool: pool, spec: loadSpec(t)}
}

func (h *authHarness) conforms(t *testing.T, template, method string, resp *http.Response, body map[string]any) {
	t.Helper()
	schema, err := h.spec.responseSchema(template, method, resp.StatusCode)
	require.NoError(t, err)
	assert.Empty(t, h.spec.validate(body, schema, "response"))
}

func jsonBody(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func TestSignupVerifyMeRefreshLogout(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	b := newBrowser(t, h.server.URL)
	email := testEmail()
	cleanupEmail(t, h.pool, email)

	resp, _ := b.do(http.MethodGet, "/v1/me", "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Equal(t, "Bearer", resp.Header.Get("WWW-Authenticate"))

	signup := jsonBody(t, map[string]string{"email": strings.ToUpper(email), "password": testPassword, "name": "Ada"})
	resp, body := b.do(http.MethodPost, "/v1/auth/signup", signup)
	require.Equal(t, http.StatusAccepted, resp.StatusCode, body)
	h.conforms(t, "/v1/auth/signup", "post", resp, body)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	resp, _ = b.do(http.MethodPost, "/v1/auth/signup", signup)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	code := h.resend.lastCode(t, h.auth, email)
	sent := h.resend.sentTo(email)
	require.Len(t, sent, 1, "a replayed signup inside the cooldown sends nothing")
	assert.Equal(t, "Bearer re_test", sent[0].authorization)
	assert.NotEmpty(t, sent[0].idempotencyKey)

	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	resp, body = b.do(http.MethodPost, "/v1/auth/verify", jsonBody(t, map[string]string{"email": email, "code": wrong}))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, "invalid_code", asMap(body["error"])["code"])

	resp, body = b.do(http.MethodPost, "/v1/auth/verify", jsonBody(t, map[string]string{"email": email, "code": code}))
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	h.conforms(t, "/v1/auth/verify", "post", resp, body)
	user := asMap(body["user"])
	assert.Equal(t, email, user["email"])
	assert.Equal(t, "Ada", user["name"])
	assert.Equal(t, []any{"password"}, user["providers"])
	require.Contains(t, b.cookies, accessCookieName)
	require.Contains(t, b.cookies, refreshCookieName)
	for _, cookie := range resp.Cookies() {
		assert.True(t, cookie.HttpOnly, cookie.Name)
		assert.Equal(t, "/", cookie.Path)
	}
	assert.NotContains(t, jsonBody(t, body), b.cookies[accessCookieName].Value, "tokens never appear in bodies")
	assert.NotContains(t, jsonBody(t, body), b.cookies[refreshCookieName].Value)

	resp, _ = b.do(http.MethodPost, "/v1/auth/verify", jsonBody(t, map[string]string{"email": email, "code": code}))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "a code is single use")

	resp, body = b.do(http.MethodGet, "/v1/me", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	h.conforms(t, "/v1/me", "get", resp, body)
	assert.Equal(t, true, asMap(body["user"])["hasPassword"])

	settings := `{"settings":{"fmt":"decimal","starred":["EPL"]}}`
	for range 2 {
		resp, body = b.do(http.MethodPut, "/v1/me/settings", settings)
		require.Equal(t, http.StatusOK, resp.StatusCode, body)
	}
	h.conforms(t, "/v1/me/settings", "put", resp, body)
	resp, body = b.do(http.MethodGet, "/v1/me/settings", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, map[string]any{"fmt": "decimal", "starred": []any{"EPL"}}, body["settings"])
	resp, _ = b.do(http.MethodPut, "/v1/me/settings", `{"settings":[1]}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp, _ = b.do(http.MethodPut, "/v1/me/settings", `{"settings":{"x":"`+strings.Repeat("a", auth.MaxSettingsBytes)+`"}}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	oldAccess, oldRefresh := b.cookies[accessCookieName].Value, b.cookies[refreshCookieName].Value
	resp, body = b.do(http.MethodPost, "/v1/auth/refresh", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	h.conforms(t, "/v1/auth/refresh", "post", resp, body)
	successor := b.cookies[refreshCookieName].Value
	assert.NotEqual(t, oldRefresh, successor)

	tab := newBrowser(t, h.server.URL)
	tab.ip = b.ip
	tab.cookies[refreshCookieName] = &http.Cookie{Name: refreshCookieName, Value: oldRefresh}
	resp, _ = tab.do(http.MethodPost, "/v1/auth/refresh", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "a concurrent tab inside the grace window gets the same successor")
	assert.Equal(t, successor, tab.cookies[refreshCookieName].Value)

	resp, _ = b.do(http.MethodPost, "/v1/auth/logout", "")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, b.cookies)
	resp, _ = b.do(http.MethodPost, "/v1/auth/logout", "")
	assert.Equal(t, http.StatusNoContent, resp.StatusCode, "logout is idempotent")

	resp, _ = b.do(http.MethodGet, "/v1/me", "", "Authorization", "Bearer "+oldAccess)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a revoked session's access token stops working")
	b.cookies[refreshCookieName] = &http.Cookie{Name: refreshCookieName, Value: successor}
	resp, _ = b.do(http.MethodPost, "/v1/auth/refresh", "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Empty(t, b.cookies, "a failed refresh clears the cookies")
}

func TestSignupForExistingAccountSendsNothing(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	email := testEmail()
	createUser(t, h.pool, email)
	resp, body := newBrowser(t, h.server.URL).do(http.MethodPost, "/v1/auth/signup", jsonBody(t, map[string]string{"email": email, "password": "another-password-1"}))
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Equal(t, map[string]any{"status": "code_sent", "resendIn": 60.0}, body)
	h.auth.Wait()
	assert.Empty(t, h.resend.sentTo(email))
}

func TestLoginLockout(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	for _, known := range []bool{true, false} {
		email := testEmail()
		if known {
			createUser(t, h.pool, email)
		}
		b := newBrowser(t, h.server.URL)
		for range 5 {
			resp, body := b.do(http.MethodPost, "/v1/auth/login", jsonBody(t, map[string]string{"email": email, "password": "wrong-password-1"}))
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			assert.Equal(t, "invalid_credentials", asMap(body["error"])["code"])
		}
		resp, body := b.do(http.MethodPost, "/v1/auth/login", jsonBody(t, map[string]string{"email": email, "password": testPassword}))
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "known=%v", known)
		h.conforms(t, "/v1/auth/login", "post", resp, body)
		assert.NotEmpty(t, resp.Header.Get("Retry-After"))
		assert.Empty(t, b.cookies)
	}
}

func TestPasswordResetRevokesSessions(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	email := testEmail()
	createUser(t, h.pool, email)
	old := newBrowser(t, h.server.URL)
	resp, _ := old.do(http.MethodPost, "/v1/auth/login", jsonBody(t, map[string]string{"email": email, "password": testPassword}))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	b := newBrowser(t, h.server.URL)
	resp, body := b.do(http.MethodPost, "/v1/auth/password/forgot", jsonBody(t, map[string]string{"email": email}))
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	h.conforms(t, "/v1/auth/password/forgot", "post", resp, body)
	resp, body = b.do(http.MethodPost, "/v1/auth/otp/resend", jsonBody(t, map[string]string{"email": email, "purpose": "reset"}))
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "resend inside the cooldown")
	h.conforms(t, "/v1/auth/otp/resend", "post", resp, body)
	assert.Positive(t, body["resendIn"])

	code := h.resend.lastCode(t, h.auth, email)
	newPassword := "a-brand-new-secret"
	resp, body = b.do(http.MethodPost, "/v1/auth/password/reset", jsonBody(t, map[string]string{"email": email, "code": code, "password": newPassword}))
	require.Equal(t, http.StatusOK, resp.StatusCode, body)
	h.conforms(t, "/v1/auth/password/reset", "post", resp, body)

	resp, _ = old.do(http.MethodGet, "/v1/me", "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "reset revokes other sessions")
	resp, _ = old.do(http.MethodPost, "/v1/auth/refresh", "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp, _ = b.do(http.MethodGet, "/v1/me", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	fresh := newBrowser(t, h.server.URL)
	resp, _ = fresh.do(http.MethodPost, "/v1/auth/login", jsonBody(t, map[string]string{"email": email, "password": testPassword}))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp, _ = fresh.do(http.MethodPost, "/v1/auth/login", jsonBody(t, map[string]string{"email": email, "password": newPassword}))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestProtectedRouteRejectsMissingOrBadTokens(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	b := newBrowser(t, h.server.URL)
	for _, header := range [][]string{nil, {"Authorization", "Bearer not-a-jwt"}, {"Authorization", "Basic abc"}} {
		resp, body := b.do(http.MethodGet, "/v1/sports", "", header...)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		h.conforms(t, "/v1/sports", "get", resp, body)
	}
	resp, _ := b.do(http.MethodGet, "/v1/meta", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode, "meta stays public")
	resp, _ = b.do(http.MethodGet, "/healthz", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMetaCountryFromTrustedProxyHeaders(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	b := newBrowser(t, h.server.URL)
	tests := []struct {
		name    string
		headers []string
		want    string
	}{
		{name: "no header", want: ""},
		{name: "cloudflare", headers: []string{"CF-IPCountry", "id"}, want: "ID"},
		{name: "vercel", headers: []string{"X-Vercel-IP-Country", "NG"}, want: "NG"},
		{name: "cloudfront", headers: []string{"CloudFront-Viewer-Country", "US"}, want: "US"},
		{name: "cloudflare unknown falls through", headers: []string{"CF-IPCountry", "XX", "X-Vercel-IP-Country", "GH"}, want: "GH"},
		{name: "tor", headers: []string{"CF-IPCountry", "T1"}, want: ""},
		{name: "garbage", headers: []string{"CF-IPCountry", "N1"}, want: ""},
	}
	for _, tt := range tests {
		resp, body := b.do(http.MethodGet, "/v1/meta", "", tt.headers...)
		require.Equal(t, http.StatusOK, resp.StatusCode, tt.name)
		h.conforms(t, "/v1/meta", "get", resp, body)
		assert.Equal(t, tt.want, body["country"], tt.name)
	}
}

func TestAuthPostsRequireSameOrigin(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	b := newBrowser(t, h.server.URL)
	login := jsonBody(t, map[string]string{"email": testEmail(), "password": testPassword})
	resp, body := b.do(http.MethodPost, "/v1/auth/login", login, "Origin", "https://evil.test")
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Equal(t, "forbidden", asMap(body["error"])["code"])
	resp, _ = b.do(http.MethodPost, "/v1/auth/login", login, "Origin", "", "Sec-Fetch-Site", "same-origin")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestBearerWinsOverCookie(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	email := testEmail()
	createUser(t, h.pool, email)
	b := newBrowser(t, h.server.URL)
	resp, _ := b.do(http.MethodPost, "/v1/auth/login", jsonBody(t, map[string]string{"email": email, "password": testPassword}))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp, _ = b.do(http.MethodGet, "/v1/me", "", "Authorization", "Bearer forged")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	session, err := h.auth.Login(context.Background(), email, testPassword, auth.Client{IP: testIP()})
	require.NoError(t, err)
	resp, body := newBrowser(t, h.server.URL).do(http.MethodGet, "/v1/me", "", "Authorization", "Bearer "+session.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, email, asMap(body["user"])["email"])
}
