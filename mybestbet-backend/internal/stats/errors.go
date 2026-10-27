package stats

import "errors"

var (
	ErrUnsupportedCode = errors.New("stats: market code has no player stat")
	ErrInvalidID       = errors.New("stats: invalid id")
)
