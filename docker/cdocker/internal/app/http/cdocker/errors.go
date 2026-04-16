package cdocker

import (
	"errors"
	"fmt"
)

var (
	// ErrContainerAlreadyExists is returned when a container already exists.
	ErrContainerAlreadyExists = errors.New("container already exists")

	// ErrContainerNotFound is returned when a container is not found.
	ErrContainerNotFound = errors.New("container not found")

	// ErrNetworkNotFound is returned when a network is not found.
	ErrNetworkNotFound = errors.New("network not found")

	// ErrInvalidRequest is returned when a request is invalid.
	ErrInvalidRequest = errors.New("invalid request")

	// ErrImageNotFound is returned when an image is not found.
	ErrImageNotFound = errors.New("image not found")
)

type ErrInvalidCode struct {
	code int
}

func NewErrInvalidCode(code int) error {
	return ErrInvalidCode{code: code}
}

func (e ErrInvalidCode) Error() string {
	return fmt.Sprintf("invalid status code: %d", e.code)
}
