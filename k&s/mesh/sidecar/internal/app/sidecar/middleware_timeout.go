package sidecar

import (
	"context"
	"fmt"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type timeoutMiddleware struct {
	timeout time.Duration
}

func newTimeoutMiddleware(timeout time.Duration) *timeoutMiddleware {
	return &timeoutMiddleware{timeout: timeout}
}

func (m *timeoutMiddleware) Handle(ctx *domain.ConnContext, next domain.NextFunc) error {
	ctxWithTimeout, cancel := context.WithTimeout(ctx.Context, m.timeout)
	defer cancel()

	nextCtx := ctx.CloneWithContext(ctxWithTimeout)
	err := next(nextCtx)
	if err == nil {
		return nil
	}

	if ctxWithTimeout.Err() != nil {
		return domain.Wrap(domain.ErrorKindTimeout, fmt.Errorf("connection establish timed out after %s", m.timeout))
	}

	return err
}
