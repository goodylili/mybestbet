package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCodeCheck(t *testing.T) {
	t.Parallel()
	k := deriveKeys([]byte("0123456789abcdef0123456789abcdef"))
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	browser := bindingHash("browser")
	live := codeRow{codeHash: k.codeHash(PurposeSignup, "a@b.test", "123456"), clientHash: browser, attempts: 1, expiresAt: now.Add(time.Minute)}
	reset := codeRow{codeHash: k.codeHash(PurposeReset, "a@b.test", "123456"), attempts: 1, expiresAt: now.Add(time.Minute)}
	tests := []struct {
		name     string
		row      codeRow
		purpose  string
		email    string
		code     string
		client   []byte
		expected error
	}{
		{name: "matching code", row: live, purpose: PurposeSignup, email: "a@b.test", code: "123456", client: browser},
		{name: "another browser", row: live, purpose: PurposeSignup, email: "a@b.test", code: "123456", client: bindingHash("attacker"), expected: ErrInvalidCode},
		{name: "no browser binding", row: live, purpose: PurposeSignup, email: "a@b.test", code: "123456", expected: ErrInvalidCode},
		{name: "reset codes are unbound", row: reset, purpose: PurposeReset, email: "a@b.test", code: "123456"},
		{name: "wrong code", row: live, purpose: PurposeSignup, email: "a@b.test", code: "123457", client: browser, expected: ErrInvalidCode},
		{name: "bound to the email", row: live, purpose: PurposeSignup, email: "c@b.test", code: "123456", client: browser, expected: ErrInvalidCode},
		{name: "bound to the purpose", row: live, purpose: PurposeReset, email: "a@b.test", code: "123456", client: browser, expected: ErrInvalidCode},
		{name: "fifth attempt still counts", row: with(live, func(c *codeRow) { c.attempts = 5 }), purpose: PurposeSignup, email: "a@b.test", code: "123456", client: browser},
		{name: "sixth attempt is dead", row: with(live, func(c *codeRow) { c.attempts = 6 }), purpose: PurposeSignup, email: "a@b.test", code: "123456", expected: ErrInvalidCode},
		{name: "expired", row: with(live, func(c *codeRow) { c.expiresAt = now }), purpose: PurposeSignup, email: "a@b.test", code: "123456", client: browser, expected: ErrInvalidCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.row.check(k, tt.purpose, tt.email, tt.code, tt.client, now, 5)
			if tt.expected == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.expected)
		})
	}
}

func with[T any](value T, mutate func(*T)) T {
	mutate(&value)
	return value
}

func TestNewCodeIsSixDigits(t *testing.T) {
	t.Parallel()
	for range 50 {
		code, err := newCode()
		assert.NoError(t, err)
		assert.Regexp(t, `^[0-9]{6}$`, code)
	}
}

func TestSuccessorIsDeterministicAndKeyed(t *testing.T) {
	t.Parallel()
	a := deriveKeys([]byte("0123456789abcdef0123456789abcdef"))
	b := deriveKeys([]byte("fedcba9876543210fedcba9876543210"))
	assert.Equal(t, a.successor("token"), a.successor("token"))
	assert.NotEqual(t, a.successor("token"), a.successor("other"))
	assert.NotEqual(t, a.successor("token"), b.successor("token"))
	assert.NotEqual(t, "token", a.successor("token"))
}
