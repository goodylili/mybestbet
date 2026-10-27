package booking

import (
	"errors"
	"time"
)

var ErrUnavailable = errors.New("booking: unavailable")

type InputError struct {
	Field  string
	Reason string
}

func (e *InputError) Error() string { return e.Field + ": " + e.Reason }

func invalidInput(field, reason string) error { return &InputError{Field: field, Reason: reason} }

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return "booking: rate limited" }
