package server

import "errors"

var (
	ErrNotFound = errors.New("server: not found")
	ErrConflict = errors.New("server: conflict")
)

type invalidInputError struct {
	field  string
	reason string
}

func (e invalidInputError) Error() string { return e.field + ": " + e.reason }

func invalidInput(field, reason string) error { return invalidInputError{field: field, reason: reason} }
