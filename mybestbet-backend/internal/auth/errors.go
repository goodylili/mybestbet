package auth

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrInvalidCode        = errors.New("auth: invalid code")
	ErrBusy               = errors.New("auth: busy")
	ErrUnauthorized       = errors.New("auth: unauthorized")
)

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return "auth: rate limited" }

type InputError struct {
	Field  string
	Reason string
}

func (e *InputError) Error() string { return e.Field + ": " + e.Reason }

func invalidInput(field, reason string) error { return &InputError{Field: field, Reason: reason} }
