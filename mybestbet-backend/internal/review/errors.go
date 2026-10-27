package review

import "errors"

var (
	ErrNotFound        = errors.New("review: item not found")
	ErrAlreadyDecided  = errors.New("review: item already decided differently")
	ErrInvalidDecision = errors.New("review: invalid decision")
	errUnknownKind     = errors.New("review: unknown item kind")
)
