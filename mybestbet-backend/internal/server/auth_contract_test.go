package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/auth"
)

func TestAuthEndpointsMatchOpenAPIContract(t *testing.T) {
	h := newAuthHarness(t, auth.GoogleEndpoints{}, "")
	email, fresh := testEmail(), testEmail()
	createUser(t, h.pool, email)
	cleanupEmail(t, h.pool, fresh)
	session, err := h.auth.Login(context.Background(), email, testPassword, auth.Client{IP: testIP()})
	require.NoError(t, err)
	errorCode := func(code string) func(t *testing.T, body map[string]any) {
		return func(t *testing.T, body map[string]any) { assert.Equal(t, code, asMap(body["error"])["code"]) }
	}
	post := func(name, path, body string, status int, check func(t *testing.T, body map[string]any)) contractCase {
		return contractCase{name: name, method: "post", template: path, path: path, body: body, anonymous: true, status: status, check: check}
	}
	cases := []contractCase{
		post("signup", "/v1/auth/signup", jsonBody(t, map[string]string{"email": fresh, "password": testPassword}), http.StatusAccepted, func(t *testing.T, body map[string]any) {
			assert.Equal(t, "code_sent", body["status"])
			assert.InDelta(t, 60, body["resendIn"], 0)
		}),
		post("signup weak password", "/v1/auth/signup", jsonBody(t, map[string]string{"email": fresh, "password": "short"}), http.StatusBadRequest, errorCode("bad_request")),
		post("signup malformed email", "/v1/auth/signup", jsonBody(t, map[string]string{"email": "nope", "password": testPassword}), http.StatusBadRequest, errorCode("bad_request")),
		post("signup malformed body", "/v1/auth/signup", "{", http.StatusBadRequest, errorCode("bad_request")),
		post("verify unknown code", "/v1/auth/verify", jsonBody(t, map[string]string{"email": testEmail(), "code": "123456"}), http.StatusBadRequest, errorCode("invalid_code")),
		post("resend without pending signup", "/v1/auth/otp/resend", jsonBody(t, map[string]string{"email": testEmail(), "purpose": "signup"}), http.StatusAccepted, nil),
		post("resend bad purpose", "/v1/auth/otp/resend", jsonBody(t, map[string]string{"email": testEmail(), "purpose": "other"}), http.StatusBadRequest, errorCode("bad_request")),
		post("forgot unknown email", "/v1/auth/password/forgot", jsonBody(t, map[string]string{"email": testEmail()}), http.StatusAccepted, nil),
		post("reset unknown code", "/v1/auth/password/reset", jsonBody(t, map[string]string{"email": email, "code": "123456", "password": "a-brand-new-secret"}), http.StatusBadRequest, errorCode("invalid_code")),
		post("login wrong password", "/v1/auth/login", jsonBody(t, map[string]string{"email": email, "password": "wrong-password-1"}), http.StatusUnauthorized, errorCode("invalid_credentials")),
		post("login unknown email", "/v1/auth/login", jsonBody(t, map[string]string{"email": testEmail(), "password": testPassword}), http.StatusUnauthorized, errorCode("invalid_credentials")),
		post("refresh without cookie", "/v1/auth/refresh", "", http.StatusUnauthorized, errorCode("unauthorized")),
		{name: "me anonymous", method: "get", template: "/v1/me", path: "/v1/me", anonymous: true, status: http.StatusUnauthorized},
		{name: "me", method: "get", template: "/v1/me", path: "/v1/me", status: http.StatusOK, check: func(t *testing.T, body map[string]any) {
			assert.Equal(t, email, asMap(body["user"])["email"])
		}},
		{name: "settings default", method: "get", template: "/v1/me/settings", path: "/v1/me/settings", status: http.StatusOK, check: func(t *testing.T, body map[string]any) {
			assert.Equal(t, map[string]any{}, body["settings"])
		}},
		{name: "settings replace", method: "put", template: "/v1/me/settings", path: "/v1/me/settings", body: `{"settings":{"theme":"dark"}}`, status: http.StatusOK},
		{name: "settings not an object", method: "put", template: "/v1/me/settings", path: "/v1/me/settings", body: `{"settings":"dark"}`, status: http.StatusBadRequest},
		{name: "settings anonymous", method: "get", template: "/v1/me/settings", path: "/v1/me/settings", anonymous: true, status: http.StatusUnauthorized},
	}
	runContractCases(t, h.server.URL, h.spec, session.AccessToken, cases)
}
