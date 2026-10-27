package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword(t.Context(), "correct horse battery")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$"))
	other, err := HashPassword(t.Context(), "correct horse battery")
	require.NoError(t, err)
	assert.NotEqual(t, hash, other)

	ok, err := VerifyPassword(t.Context(), hash, "correct horse battery")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = VerifyPassword(t.Context(), hash, "wrong horse battery")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	t.Parallel()
	for _, hash := range []string{"", "plain", "$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA", "$argon2id$v=19$m=99999999,t=2,p=1$c2FsdA$aGFzaA", "$argon2id$v=19$m=19456,t=2,p=1$!!$aGFzaA"} {
		_, err := VerifyPassword(t.Context(), hash, "whatever")
		assert.ErrorIs(t, err, errMalformedHash, hash)
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "long enough", password: "tangerine-sky", valid: true},
		{name: "too short", password: "short1234", valid: false},
		{name: "too long", password: strings.Repeat("a", 129), valid: false},
		{name: "max length", password: strings.Repeat("ab", 64), valid: true},
		{name: "multibyte counted as characters", password: strings.Repeat("é", 10), valid: true},
		{name: "common", password: "Password123", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePassword(tt.password)
			if tt.valid {
				assert.NoError(t, err)
				return
			}
			var input *InputError
			assert.ErrorAs(t, err, &input)
		})
	}
}

func TestSemaphoreReportsBusy(t *testing.T) {
	t.Parallel()
	sem := newSemaphore(1, 20*time.Millisecond)
	release, err := sem.acquire(t.Context())
	require.NoError(t, err)
	_, err = sem.acquire(t.Context())
	assert.ErrorIs(t, err, ErrBusy)
	release()
	again, err := sem.acquire(t.Context())
	require.NoError(t, err)
	again()
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	hold, err := sem.acquire(t.Context())
	require.NoError(t, err)
	defer hold()
	_, err = sem.acquire(cancelled)
	assert.ErrorIs(t, err, context.Canceled)
}
