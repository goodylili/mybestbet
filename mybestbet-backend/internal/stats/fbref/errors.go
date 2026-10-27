package fbref

import "errors"

var (
	ErrTableNotFound = errors.New("fbref: stats table not found")
	ErrUnknownLeague = errors.New("fbref: no competition for league code")
)
