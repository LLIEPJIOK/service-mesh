package domain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

type ErrorKind string

const (
	ErrorKindDial        ErrorKind = "dial"
	ErrorKindTLS         ErrorKind = "tls"
	ErrorKindTimeout     ErrorKind = "timeout"
	ErrorKindProxy       ErrorKind = "proxy"
	ErrorKindDiscovery   ErrorKind = "discovery"
	ErrorKindBreakerOpen ErrorKind = "breaker_open"
)

type SidecarError struct {
	Kind ErrorKind
	Err  error
}

func (e *SidecarError) Error() string {
	return fmt.Sprintf("%s: %v", e.Kind, e.Err)
}

func (e *SidecarError) Unwrap() error {
	return e.Err
}

func Wrap(kind ErrorKind, err error) error {
	if err == nil {
		return nil
	}

	var sidecarErr *SidecarError
	if errors.As(err, &sidecarErr) {
		return err
	}

	return &SidecarError{
		Kind: kind,
		Err:  err,
	}
}

func IsKind(err error, kind ErrorKind) bool {
	var sidecarErr *SidecarError
	if !errors.As(err, &sidecarErr) {
		return false
	}

	return sidecarErr.Kind == kind
}

func IsEstablishError(err error) bool {
	if err == nil {
		return false
	}

	return IsKind(err, ErrorKindDial) ||
		IsKind(err, ErrorKindTLS) ||
		IsKind(err, ErrorKindTimeout) ||
		IsKind(err, ErrorKindBreakerOpen)
}

func NormalizeErrorType(err error) string {
	switch {
	case IsKind(err, ErrorKindTimeout):
		return string(ErrorKindTimeout)
	case IsKind(err, ErrorKindTLS):
		return string(ErrorKindTLS)
	case IsKind(err, ErrorKindDiscovery):
		return string(ErrorKindDiscovery)
	case IsKind(err, ErrorKindBreakerOpen):
		return string(ErrorKindBreakerOpen)
	case IsKind(err, ErrorKindDial):
		return string(ErrorKindDial)
	case IsKind(err, ErrorKindProxy):
		return string(ErrorKindProxy)
	default:
		return "unknown"
	}
}

func ClassifyDialError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return Wrap(ErrorKindTimeout, err)
	}

	var netError net.Error
	if errors.As(err, &netError) && netError.Timeout() {
		return Wrap(ErrorKindTimeout, err)
	}

	if strings.Contains(strings.ToLower(err.Error()), "tls") {
		return Wrap(ErrorKindTLS, err)
	}

	return Wrap(ErrorKindDial, err)
}
