package client

import (
	"errors"
	"net/http"

	"github.com/LLIEPJIOK/sidecar/pkg/client/config"
	"github.com/sony/gobreaker/v2"
)

type CircuitBreakerTransport struct {
	base http.RoundTripper
	cb   *gobreaker.CircuitBreaker[*http.Response]
}

func NewTransport(cfg *config.CircuitBreaker, base http.RoundTripper) *CircuitBreakerTransport {
	cbSettings := gobreaker.Settings{
		MaxRequests: cfg.MaxHalfOpenRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < cfg.MinRequests {
				return false
			}

			return counts.ConsecutiveFailures >= cfg.ConsecutiveFailures ||
				float64(counts.TotalFailures)/float64(counts.Requests) > cfg.FailureRate
		},
	}
	//nolint:bodyclose // nothing to close
	cb := gobreaker.NewCircuitBreaker[*http.Response](cbSettings)

	return &CircuitBreakerTransport{
		base: base,
		cb:   cb,
	}
}

func (t *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.cb.Execute(func() (*http.Response, error) {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode >= 500 {
			return resp, NewErrUnexpectedStatusCode(resp.StatusCode)
		}

		return resp, nil
	})
	if err != nil && !errors.As(err, &ErrUnexpectedStatusCode{}) {
		return nil, err
	}

	return res, nil
}
