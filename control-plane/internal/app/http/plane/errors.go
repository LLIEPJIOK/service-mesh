package plane

import "errors"

var (
	ErrNotFound    = errors.New("no service with this name")
	ErrInvalidHost = errors.New("invalid host")
)
