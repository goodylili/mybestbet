package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	json "github.com/goccy/go-json"
)

const (
	DefaultResendBaseURL = "https://api.resend.com"
	mailSubject          = "Your mybestbet code"
	maxErrorBody         = 4 << 10
	devMailPerm          = 0o600
	devMailDirPerm       = 0o700
)

var unsafeFileChars = regexp.MustCompile(`[^a-z0-9@._+-]`)

type Mail struct {
	IdempotencyKey string
	To             string
	Purpose        string
	Code           string
	TTL            time.Duration
}

func (m Mail) text() string {
	return fmt.Sprintf("Your mybestbet code is %s. It expires in %d minutes.", m.Code, int(m.TTL.Minutes()))
}

type Mailer interface {
	Send(ctx context.Context, mail Mail) error
}

type resendMailer struct {
	client  *http.Client
	baseURL string
	apiKey  string
	from    string
}

type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

func (m resendMailer) Send(ctx context.Context, mail Mail) error {
	text := mail.text()
	body, err := json.Marshal(resendPayload{
		From:    m.from,
		To:      []string{mail.To},
		Subject: mailSubject,
		Text:    text,
		HTML:    "<p>" + html.EscapeString(text) + "</p>",
	})
	if err != nil {
		return fmt.Errorf("encode mail: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(m.baseURL, "/")+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build mail request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", mail.IdempotencyKey)
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("send mail: resend status %d", resp.StatusCode)
	}
	return nil
}

type devMailer struct {
	log *slog.Logger
	dir string
}

func (m devMailer) Send(_ context.Context, mail Mail) error {
	m.log.Info("auth code issued", "purpose", mail.Purpose)
	if m.dir == "" {
		return nil
	}
	if err := os.MkdirAll(m.dir, devMailDirPerm); err != nil {
		return fmt.Errorf("dev mail dir: %w", err)
	}
	root, err := os.OpenRoot(m.dir)
	if err != nil {
		return fmt.Errorf("dev mail dir: %w", err)
	}
	defer func() { _ = root.Close() }()
	name := unsafeFileChars.ReplaceAllString(mail.To, "_") + "." + mail.Purpose + ".txt"
	return root.WriteFile(name, []byte(mail.text()+"\n"), devMailPerm)
}

func devMailDir(configured string) (string, error) {
	if configured == "" {
		return "", nil
	}
	tmp := filepath.Clean(os.TempDir())
	dir := filepath.Clean(configured)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(tmp, dir)
	}
	rel, err := filepath.Rel(tmp, dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", errors.New("auth: AUTH_DEV_MAIL_DIR must be inside the system temp dir")
	}
	return dir, nil
}
