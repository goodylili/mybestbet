package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory    = 19456
	argonTime      = 2
	argonThreads   = 1
	argonSaltLen   = 16
	argonKeyLen    = 32
	maxArgonMemory = 1 << 20
	minPasswordLen = 10
	maxPasswordLen = 128
	argonWait      = 2 * time.Second
)

var errMalformedHash = errors.New("auth: malformed password hash")

//go:embed common_passwords.txt
var commonPasswordList string

var commonPasswords = sync.OnceValue(func() map[string]struct{} {
	set := map[string]struct{}{}
	for _, line := range strings.Split(commonPasswordList, "\n") {
		if word := strings.TrimSpace(line); word != "" {
			set[word] = struct{}{}
		}
	}
	return set
})

var argonSlots = newSemaphore(max(2, runtime.GOMAXPROCS(0)), argonWait)

var dummyHash = sync.OnceValue(func() string {
	hash, err := encodeHash("dummy-password-for-timing")
	if err != nil {
		panic(err)
	}
	return hash
})

func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPasswordLen || length > maxPasswordLen {
		return invalidInput("password", fmt.Sprintf("must be %d to %d characters", minPasswordLen, maxPasswordLen))
	}
	if _, common := commonPasswords()[strings.ToLower(password)]; common {
		return invalidInput("password", "is too common")
	}
	return nil
}

func HashPassword(ctx context.Context, password string) (string, error) {
	release, err := argonSlots.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	return encodeHash(password)
}

func encodeHash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(ctx context.Context, encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil || memory > maxArgonMemory || iterations == 0 || threads == 0 {
		return false, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}
	release, err := argonSlots.acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
