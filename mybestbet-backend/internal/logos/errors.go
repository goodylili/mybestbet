package logos

import "errors"

var (
	ErrNotFound       = errors.New("logos: not found")
	ErrUnsupportedURL = errors.New("logos: only absolute http(s) urls are fetched")
	ErrBlockedAddress = errors.New("logos: refusing non-public address")
	ErrTooLarge       = errors.New("logos: image exceeds size limit")
	ErrNotImage       = errors.New("logos: body is not an accepted image type")
	ErrUpstreamStatus = errors.New("logos: upstream returned non-200 status")
)
