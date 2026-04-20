package sidecar

import (
	"math"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/adapters/metrics"
	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type retryMiddleware struct {
	attempts     int
	backoffType  string
	baseInterval time.Duration
	recorder     *metrics.Recorder
}

func newRetryMiddleware(attempts int, backoffType string, baseInterval time.Duration, recorder *metrics.Recorder) *retryMiddleware {
	return &retryMiddleware{
		attempts:     attempts,
		backoffType:  backoffType,
		baseInterval: baseInterval,
		recorder:     recorder,
	}
}

func (m *retryMiddleware) Handle(ctx *domain.ConnContext, next domain.NextFunc) error {
	if m.attempts <= 1 {
		return next(ctx)
	}

	var lastErr error
	for attempt := 1; attempt <= m.attempts; attempt++ {
		err := next(ctx)
		if err == nil {
			return nil
		}

		lastErr = err
		if !domain.IsEstablishError(err) {
			return err
		}

		if attempt == m.attempts {
			return err
		}

		service := ctx.GetString(domain.MetadataService)
		m.recorder.IncRetry(service)

		waitFor := m.backoffDuration(attempt)
		select {
		case <-ctx.Context.Done():
			return domain.Wrap(domain.ErrorKindTimeout, ctx.Context.Err())
		case <-time.After(waitFor):
		}
	}

	return lastErr
}

func (m *retryMiddleware) backoffDuration(retryNumber int) time.Duration {
	if retryNumber < 1 {
		retryNumber = 1
	}

	if m.backoffType != "exponential" {
		return time.Duration(retryNumber) * m.baseInterval
	}

	factor := math.Pow(2, float64(retryNumber-1))
	return time.Duration(factor) * m.baseInterval
}
