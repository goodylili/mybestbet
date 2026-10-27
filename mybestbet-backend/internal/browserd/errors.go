package browserd

import "errors"

var (
	ErrInvalidRequest = errors.New("browserd: invalid request")
	ErrHostNotAllowed = errors.New("browserd: host not allowed")
	ErrPoolClosed     = errors.New("browserd: pool closed")
)
