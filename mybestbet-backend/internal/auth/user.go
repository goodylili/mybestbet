package auth

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ProviderPassword = "password"
	ProviderGoogle   = "google"
	maxEmailLen      = 254
	maxNameLen       = 100
)

type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name,omitempty"`
	AvatarURL     string    `json:"avatarUrl,omitempty"`
	EmailVerified bool      `json:"emailVerified"`
	HasPassword   bool      `json:"hasPassword"`
	Providers     []string  `json:"providers"`
	CreatedAt     time.Time `json:"createdAt"`
}

type userRecord struct {
	id           string
	email        string
	verifiedAt   *time.Time
	passwordHash *string
	name         *string
	avatarURL    *string
	createdAt    time.Time
	identities   []string
}

func (u userRecord) view() User {
	providers := make([]string, 0, len(u.identities)+1)
	if u.passwordHash != nil {
		providers = append(providers, ProviderPassword)
	}
	providers = append(providers, u.identities...)
	return User{
		ID:            u.id,
		Email:         u.email,
		Name:          deref(u.name),
		AvatarURL:     deref(u.avatarURL),
		EmailVerified: u.verifiedAt != nil,
		HasPassword:   u.passwordHash != nil,
		Providers:     providers,
		CreatedAt:     u.createdAt,
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > maxEmailLen {
		return "", invalidInput("email", "must be a valid email address")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || parsed.Name != "" {
		return "", invalidInput("email", "must be a valid email address")
	}
	return email, nil
}

func normalizeName(raw string) (*string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(name) > maxNameLen || !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return nil, invalidInput("name", "must be at most 100 characters")
	}
	return &name, nil
}
