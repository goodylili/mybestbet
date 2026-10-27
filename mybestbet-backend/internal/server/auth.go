package server

import (
	"errors"
	"io"
	"math"
	"net/http"
	"time"

	json "github.com/goccy/go-json"

	"mybestbet/internal/auth"
)

const (
	maxAuthBodyBytes     = 4 << 10
	maxSettingsBodyBytes = auth.MaxSettingsBytes + 1<<10
	codeSentStatus       = "code_sent"
	googleErrorPath      = "/login?error=google"
)

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Purpose  string `json:"purpose"`
	All      bool   `json:"all"`
}

type codeSentResponse struct {
	Status   string `json:"status"`
	ResendIn int    `json:"resendIn"`
}

type userResponse struct {
	User auth.User `json:"user"`
}

type settingsBody struct {
	Settings json.RawMessage `json:"settings"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, limit int64, into any) error {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(into)
	if err != nil && !errors.Is(err, io.EOF) {
		return invalidInput("body", "must be a json object")
	}
	return nil
}

func (s *Server) authBody(w http.ResponseWriter, r *http.Request) (credentialsRequest, bool) {
	w.Header().Set("Cache-Control", "no-store")
	var body credentialsRequest
	if s.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "authentication is not configured")
		return body, false
	}
	if err := decodeBody(w, r, maxAuthBodyBytes, &body); err != nil {
		s.writeFailure(w, r, err)
		return body, false
	}
	return body, true
}

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	if body, ok := s.authBody(w, r); ok {
		input := auth.SignupInput{Email: body.Email, Password: body.Password, Name: body.Name, Binding: s.cookieValue(r, signupCookieName)}
		sent, err := s.auth.Signup(r.Context(), input, s.client(r))
		if err == nil {
			http.SetCookie(w, s.cookie(signupCookieName, sent.Binding, s.auth.Config().OTPTTL, http.SameSiteStrictMode))
		}
		s.respondCodeSent(w, r, sent, err)
	}
}

func (s *Server) resendCode(w http.ResponseWriter, r *http.Request) {
	if body, ok := s.authBody(w, r); ok {
		sent, err := s.auth.ResendCode(r.Context(), body.Email, body.Purpose, s.cookieValue(r, signupCookieName), s.client(r))
		s.respondCodeSent(w, r, sent, err)
	}
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if body, ok := s.authBody(w, r); ok {
		sent, err := s.auth.ForgotPassword(r.Context(), body.Email, s.client(r))
		s.respondCodeSent(w, r, sent, err)
	}
}

func (s *Server) verifyCode(w http.ResponseWriter, r *http.Request) {
	if body, ok := s.authBody(w, r); ok {
		session, err := s.auth.Verify(r.Context(), body.Email, body.Code, s.cookieValue(r, signupCookieName), s.client(r))
		if err == nil {
			http.SetCookie(w, s.cookie(signupCookieName, "", -1, http.SameSiteStrictMode))
		}
		s.respondSession(w, r, session, err)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if body, ok := s.authBody(w, r); ok {
		session, err := s.auth.Login(r.Context(), body.Email, body.Password, s.client(r))
		s.respondSession(w, r, session, err)
	}
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	if body, ok := s.authBody(w, r); ok {
		session, err := s.auth.ResetPassword(r.Context(), body.Email, body.Code, body.Password, s.client(r))
		s.respondSession(w, r, session, err)
	}
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authBody(w, r); !ok {
		return
	}
	session, err := s.auth.Refresh(r.Context(), s.cookieValue(r, refreshCookieName), s.client(r))
	if errors.Is(err, auth.ErrUnauthorized) {
		s.clearSessionCookies(w)
	}
	s.respondSession(w, r, session, err)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	body, ok := s.authBody(w, r)
	if !ok {
		return
	}
	s.clearSessionCookies(w)
	if err := s.auth.Logout(r.Context(), s.cookieValue(r, refreshCookieName), s.accessToken(r), body.All); err != nil {
		s.writeFailure(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		http.Error(w, "authentication is not configured", http.StatusServiceUnavailable)
		return
	}
	redirect, err := s.auth.GoogleStart(r.Context(), r.URL.Query().Get("next"), s.client(r))
	var limited *auth.RateLimitError
	switch {
	case errors.As(err, &limited):
		s.writeFailure(w, r, err)
		return
	case err != nil:
		s.log.WarnContext(r.Context(), "google start failed", "err", err)
		redirect.URL = s.auth.Config().WebOrigin + googleErrorPath
	default:
		http.SetCookie(w, s.cookie(oauthCookieName, redirect.StateBinding, auth.OAuthStateTTL, http.SameSiteLaxMode))
	}
	http.Redirect(w, r, redirect.URL, http.StatusFound) //nolint:gosec
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.auth == nil {
		http.Error(w, "authentication is not configured", http.StatusServiceUnavailable)
		return
	}
	query := r.URL.Query()
	binding := s.cookieValue(r, oauthCookieName)
	http.SetCookie(w, s.cookie(oauthCookieName, "", -1, http.SameSiteLaxMode))
	session, next, err := s.auth.GoogleCallback(r.Context(), query.Get("code"), query.Get("state"), binding, s.client(r))
	if err != nil {
		s.log.WarnContext(r.Context(), "google callback failed", "err", err)
		http.Redirect(w, r, s.auth.Config().WebOrigin+googleErrorPath, http.StatusFound)
		return
	}
	s.setSessionCookies(w, session)
	http.Redirect(w, r, s.auth.Config().WebOrigin+next, http.StatusFound) //nolint:gosec
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	user, err := s.auth.Me(r.Context(), principal.UserID)
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, userResponse{User: user})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	settings, err := s.userSettings(r.Context(), principal.UserID)
	s.respondSettings(w, r, settings, err)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFrom(r.Context())
	var body settingsBody
	if err := decodeBody(w, r, maxSettingsBodyBytes, &body); err != nil {
		s.writeFailure(w, r, err)
		return
	}
	settings, err := s.auth.PutSettings(r.Context(), principal.UserID, body.Settings)
	if err == nil {
		s.settings.put(principal.UserID, settings, time.Now())
	}
	s.respondSettings(w, r, settings, err)
}

func (s *Server) respondSettings(w http.ResponseWriter, r *http.Request, settings json.RawMessage, err error) {
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeCachedJSON(w, r, settingsBody{Settings: settings}, 0)
}

func (s *Server) respondCodeSent(w http.ResponseWriter, r *http.Request, sent auth.CodeSent, err error) {
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, codeSentResponse{Status: codeSentStatus, ResendIn: seconds(sent.ResendIn)})
}

func (s *Server) respondSession(w http.ResponseWriter, r *http.Request, session auth.Session, err error) {
	if err != nil {
		s.writeFailure(w, r, err)
		return
	}
	s.setSessionCookies(w, session)
	writeJSON(w, http.StatusOK, userResponse{User: session.User})
}

func (s *Server) setSessionCookies(w http.ResponseWriter, session auth.Session) {
	http.SetCookie(w, s.cookie(accessCookieName, session.AccessToken, time.Until(session.AccessExpiresAt), http.SameSiteLaxMode))
	http.SetCookie(w, s.cookie(refreshCookieName, session.RefreshToken, time.Until(session.RefreshExpiresAt), http.SameSiteStrictMode))
}

func (s *Server) clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, s.cookie(accessCookieName, "", -1, http.SameSiteLaxMode))
	http.SetCookie(w, s.cookie(refreshCookieName, "", -1, http.SameSiteStrictMode))
}

func (s *Server) cookie(base, value string, ttl time.Duration, sameSite http.SameSite) *http.Cookie {
	maxAge := -1
	if ttl > 0 {
		maxAge = seconds(ttl)
	}
	return &http.Cookie{ //nolint:gosec
		Name:     s.cookieName(base),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: sameSite,
	}
}

func seconds(d time.Duration) int { return int(math.Ceil(d.Seconds())) }
