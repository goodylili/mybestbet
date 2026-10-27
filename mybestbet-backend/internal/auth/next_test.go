package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeNext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		next     string
		expected string
	}{
		{name: "same-site path", next: "/sports/basketball?league=NBA", expected: "/sports/basketball?league=NBA"},
		{name: "root", next: "/", expected: "/"},
		{name: "empty", next: "", expected: DefaultNext},
		{name: "relative", next: "sports", expected: DefaultNext},
		{name: "protocol relative", next: "//evil.test/x", expected: DefaultNext},
		{name: "backslash host", next: "/\\evil.test", expected: DefaultNext},
		{name: "backslash later", next: "/a\\b", expected: DefaultNext},
		{name: "absolute url", next: "https://evil.test/", expected: DefaultNext},
		{name: "javascript scheme", next: "javascript:alert(1)", expected: DefaultNext},
		{name: "control character", next: "/a\r\nLocation: x", expected: DefaultNext},
		{name: "tab smuggling", next: "/\t/evil.test", expected: DefaultNext},
		{name: "too long", next: "/" + strings.Repeat("a", maxNextLen), expected: DefaultNext},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, SafeNext(tt.next))
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	email, err := normalizeEmail("  Alice@Example.COM ")
	assert.NoError(t, err)
	assert.Equal(t, "alice@example.com", email)
	for _, raw := range []string{"", "alice", "Alice <alice@example.com>", "a@b@c", strings.Repeat("a", 250) + "@b.co"} {
		_, err := normalizeEmail(raw)
		assert.Error(t, err, raw)
	}
}
