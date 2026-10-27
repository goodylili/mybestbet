package kalshi

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	headerAccessKey       = "KALSHI-ACCESS-KEY"
	headerAccessSignature = "KALSHI-ACCESS-SIGNATURE"
	headerAccessTimestamp = "KALSHI-ACCESS-TIMESTAMP"
)

var errUnsupportedKey = errors.New("kalshi: api secret is not an rsa or ed25519 private key in pem form")

type messageSigner func(message []byte) ([]byte, error)

func signedHandshakeHeaders(keyID, privateKeyPEM, path string, now time.Time) (http.Header, error) {
	sign, err := parseSigner(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(now.UnixMilli(), 10)
	signature, err := sign([]byte(timestamp + http.MethodGet + path))
	if err != nil {
		return nil, fmt.Errorf("sign websocket handshake: %w", err)
	}
	h := http.Header{}
	h.Set(headerAccessKey, keyID)
	h.Set(headerAccessSignature, base64.StdEncoding.EncodeToString(signature))
	h.Set(headerAccessTimestamp, timestamp)
	return h, nil
}

func parseSigner(privateKeyPEM string) (messageSigner, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errUnsupportedKey
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return signerFor(key)
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return signerFor(key)
	}
	return nil, errUnsupportedKey
}

func signerFor(key any) (messageSigner, error) {
	switch k := key.(type) {
	case *rsa.PrivateKey:
		return func(message []byte) ([]byte, error) {
			digest := sha256.Sum256(message)
			return rsa.SignPSS(rand.Reader, k, crypto.SHA256, digest[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
		}, nil
	case ed25519.PrivateKey:
		return func(message []byte) ([]byte, error) { return ed25519.Sign(k, message), nil }, nil
	}
	return nil, errUnsupportedKey
}
