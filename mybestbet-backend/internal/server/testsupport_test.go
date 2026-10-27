package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	json "github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"mybestbet/internal/auth"
	"mybestbet/internal/platform/config"
)

const (
	testWebOrigin = "http://web.test"
	testPassword  = "tangerine-sky-42"
)

var mailedCode = regexp.MustCompile(`\b[0-9]{6}\b`)

type sentMail struct {
	idempotencyKey string
	authorization  string
	to             []string
	text           string
}

type resendMock struct {
	*httptest.Server
	mu    sync.Mutex
	mails []sentMail
}

func newResendMock(t *testing.T) *resendMock {
	t.Helper()
	m := &resendMock{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			To   []string `json:"to"`
			Text string   `json:"text"`
		}
		if r.Method != http.MethodPost || r.URL.Path != "/emails" || json.NewDecoder(r.Body).Decode(&payload) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.mails = append(m.mails, sentMail{idempotencyKey: r.Header.Get("Idempotency-Key"), authorization: r.Header.Get("Authorization"), to: payload.To, text: payload.Text})
		m.mu.Unlock()
		_, _ = io.WriteString(w, `{"id":"`+uuid.NewString()+`"}`)
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *resendMock) sentTo(email string) []sentMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sent []sentMail
	for _, mail := range m.mails {
		if slices.Contains(mail.to, email) {
			sent = append(sent, mail)
		}
	}
	return sent
}

func (m *resendMock) lastCode(t *testing.T, svc *auth.Service, email string) string {
	t.Helper()
	svc.Wait()
	sent := m.sentTo(email)
	require.NotEmpty(t, sent, "no mail sent to %s", email)
	code := mailedCode.FindString(sent[len(sent)-1].text)
	require.NotEmpty(t, code)
	return code
}

func testAuthConfig() config.Auth {
	return config.Auth{
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     720 * time.Hour,
		OTPTTL:         10 * time.Minute,
		OTPCooldown:    time.Minute,
		OTPMaxAttempts: 5,
		TrustProxy:     true,
		WebOrigin:      testWebOrigin,
		AllowedOrigins: []string{testWebOrigin},
		EmailFrom:      "mybestbet <codes@web.test>",
		JWTPrivateKey:  base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		CodePepper:     "pepper-pepper-pepper-pepper-pepper",
		ResendAPIKey:   "re_test",
	}
}

func newTestAuth(t *testing.T, cfg config.Config, pool *pgxpool.Pool, rdb *goredis.Client, resendURL string, google auth.GoogleEndpoints) *auth.Service {
	t.Helper()
	svc, err := auth.New(auth.Deps{Config: cfg, Pool: pool, Redis: rdb, ResendBaseURL: resendURL, Google: google})
	require.NoError(t, err)
	t.Cleanup(svc.Wait)
	return svc
}

func testEmail() string { return "user-" + uuid.NewString()[:12] + "@web.test" }

func testIP() string {
	return fmt.Sprintf("10.%d.%d.%d", rand.IntN(256), rand.IntN(256), rand.IntN(254)+1)
}

func cleanupEmail(t *testing.T, pool *pgxpool.Pool, email string) {
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
		_, _ = pool.Exec(ctx, `DELETE FROM email_codes WHERE email = $1`, email)
	})
}

func createUser(t *testing.T, pool *pgxpool.Pool, email string) {
	t.Helper()
	hash, err := auth.HashPassword(context.Background(), testPassword)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `INSERT INTO users (email, "passwordHash", "emailVerifiedAt") VALUES ($1, $2, now())`, email, hash)
	require.NoError(t, err)
	cleanupEmail(t, pool, email)
}

func userToken(t *testing.T, s *Server, pool *pgxpool.Pool) string {
	t.Helper()
	email := testEmail()
	createUser(t, pool, email)
	session, err := s.auth.Login(context.Background(), email, testPassword, auth.Client{IP: testIP()})
	require.NoError(t, err)
	return session.AccessToken
}

func authorize(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
}

type browser struct {
	t       *testing.T
	baseURL string
	ip      string
	cookies map[string]*http.Cookie
}

func newBrowser(t *testing.T, baseURL string) *browser {
	return &browser{t: t, baseURL: baseURL, ip: testIP(), cookies: map[string]*http.Cookie{}}
}

func (b *browser) do(method, path, body string, headers ...string) (*http.Response, map[string]any) {
	b.t.Helper()
	req, err := http.NewRequest(method, b.baseURL+path, strings.NewReader(body))
	require.NoError(b.t, err)
	req.Header.Set("Origin", testWebOrigin)
	req.Header.Set("X-Forwarded-For", b.ip)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	for _, cookie := range b.cookies {
		req.AddCookie(cookie)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	require.NoError(b.t, err)
	defer resp.Body.Close()
	for _, cookie := range resp.Cookies() {
		if cookie.MaxAge < 0 {
			delete(b.cookies, cookie.Name)
			continue
		}
		b.cookies[cookie.Name] = cookie
	}
	raw, err := io.ReadAll(resp.Body)
	require.NoError(b.t, err)
	var decoded map[string]any
	if len(raw) > 0 && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		require.NoError(b.t, json.Unmarshal(raw, &decoded))
	}
	return resp, decoded
}
