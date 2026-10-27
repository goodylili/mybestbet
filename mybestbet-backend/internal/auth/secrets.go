package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
)

const (
	minPepperLen     = 32
	refreshTokenLen  = 32
	stateTokenLen    = 32
	codeDigits       = 6
	codeLabel        = "email-code"
	rotationLabel    = "refresh-rotation"
	identifierLabel  = "identifier"
	jwtKeyEnvName    = "AUTH_JWT_PRIVATE_KEY"
	pepperEnvName    = "AUTH_CODE_PEPPER"
	ephemeralWarning = "auth secret missing, using an ephemeral value; sessions and codes will not survive a restart"
)

var codeSpace = big.NewInt(1_000_000)

type keys struct {
	code       []byte
	rotation   []byte
	identifier []byte
}

func deriveKeys(pepper []byte) keys {
	return keys{code: mac(pepper, codeLabel), rotation: mac(pepper, rotationLabel), identifier: mac(pepper, identifierLabel)}
}

func loadJWTSeed(encoded, env string, log *slog.Logger) ([]byte, error) {
	if encoded == "" {
		return ephemeral(env, jwtKeyEnvName, 32, log)
	}
	seed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("auth: %s is not base64", jwtKeyEnvName)
	}
	return seed, nil
}

func loadPepper(raw, env string, log *slog.Logger) ([]byte, error) {
	if raw == "" {
		return ephemeral(env, pepperEnvName, minPepperLen, log)
	}
	if len(raw) < minPepperLen {
		return nil, fmt.Errorf("auth: %s must be at least %d bytes", pepperEnvName, minPepperLen)
	}
	return []byte(raw), nil
}

func isLocalEnv(env string) bool { return env == "dev" || env == "test" }

func ephemeral(env, name string, size int, log *slog.Logger) ([]byte, error) {
	if !isLocalEnv(env) {
		return nil, fmt.Errorf("auth: %s is required outside APP_ENV=dev or test", name)
	}
	log.Warn(ephemeralWarning, "secret", name)
	return randomBytes(size)
}

func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("random bytes: %w", err)
	}
	return buf, nil
}

func randomToken(n int) (string, error) {
	buf, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, codeSpace)
	if err != nil {
		return "", fmt.Errorf("random code: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, n.Int64()), nil
}

func mac(key []byte, parts ...string) []byte {
	h := hmac.New(sha256.New, key)
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return h.Sum(nil)
}

func hashToken(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

func (k keys) codeHash(purpose, email, code string) []byte { return mac(k.code, purpose, email, code) }

func (k keys) successor(token string) string {
	return base64.RawURLEncoding.EncodeToString(mac(k.rotation, token))
}

func (k keys) subject(value string) string { return hex.EncodeToString(mac(k.identifier, value)[:16]) }
