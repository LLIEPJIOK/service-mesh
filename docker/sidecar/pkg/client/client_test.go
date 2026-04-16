package client_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LLIEPJIOK/sidecar/pkg/client"
	"github.com/LLIEPJIOK/sidecar/pkg/client/config"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestConfig() *config.Config {
	return &config.Config{
		Retry: config.Retry{
			RetryMax:     3,
			RetryWaitMin: 10 * time.Millisecond,
			RetryWaitMax: 50 * time.Millisecond,
		},
		HTTPClient: config.HTTPClient{
			Timeout:               1 * time.Second,
			DialTimeout:           100 * time.Millisecond,
			DialKeepAlive:         100 * time.Millisecond,
			MaxIdleConns:          10,
			IdleConnTimeout:       100 * time.Millisecond,
			TLSHandshakeTimeout:   100 * time.Millisecond,
			ExpectContinueTimeout: 100 * time.Millisecond,
		},
		CircuitBreaker: config.CircuitBreaker{
			MaxHalfOpenRequests: 1,
			Interval:            50 * time.Millisecond,
			Timeout:             100 * time.Millisecond,
			MinRequests:         4,
			ConsecutiveFailures: 4,
			FailureRate:         0.6,
		},
	}
}

func TestClient_Do_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	}))
	defer server.Close()

	c := client.New(newTestConfig())

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL,
		http.NoBody,
	)
	require.NoError(t, err, "failed to create request")

	resp, err := c.Do(req)
	require.NoError(t, err, "failed to execute request")

	defer func() {
		err := resp.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected status code 200")
}

func TestClient_Do_RetryOn503(t *testing.T) {
	t.Parallel()

	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		if requestCount <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := client.New(newTestConfig())

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL,
		http.NoBody,
	)
	require.NoError(t, err, "failed to create request")

	resp, err := c.Do(req)
	require.NoError(t, err, "failed to execute request")

	defer func() {
		err := resp.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected status code 200")
	assert.Equal(
		t,
		3,
		requestCount,
		"expected 3 requests (1 initial + 2 retries)",
	)
}

func TestClient_Do_RetryOn429WithHeader(t *testing.T) {
	t.Parallel()

	requestCount := 0
	retryAfterSeconds := "1"

	var (
		firstRequestTime  time.Time
		secondRequestTime time.Time
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		now := time.Now()

		if requestCount == 1 {
			firstRequestTime = now

			w.Header().Set("Retry-After", retryAfterSeconds)
			w.WriteHeader(http.StatusTooManyRequests)

			return
		}

		secondRequestTime = now

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := newTestConfig()
	cfg.Retry.RetryMax = 1
	cfg.Retry.RetryWaitMin = 5 * time.Second
	cfg.Retry.RetryWaitMax = 10 * time.Second
	c := client.New(cfg)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL,
		http.NoBody,
	)
	require.NoError(t, err, "failed to create request")

	resp, err := c.Do(req)
	require.NoError(t, err, "failed to execute request")

	defer func() {
		err := resp.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected status code 200")
	assert.Equal(
		t,
		2,
		requestCount,
		"expected 2 requests (1 initial + 1 retry)",
	)

	expectedDelay, err := time.ParseDuration(retryAfterSeconds + "s")
	require.NoError(t, err, "failed to parse Retry-After duration")

	actualDelay := secondRequestTime.Sub(firstRequestTime)
	assert.GreaterOrEqual(t, actualDelay, expectedDelay, "actual delay should be >= Retry-After")
	assert.Less(
		t,
		actualDelay,
		expectedDelay+500*time.Millisecond,
		"actual delay should be close to Retry-After",
	)
}

func TestClient_Do_NoRetryOn400(t *testing.T) {
	t.Parallel()

	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++

		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := client.New(newTestConfig())

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL,
		http.NoBody,
	)
	require.NoError(t, err)

	resp, err := c.Do(req)
	require.NoError(t, err, "failed to execute request")

	defer func() {
		err := resp.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "expected status code 400")
	assert.Equal(t, 1, requestCount, "expected only 1 request")
}

func TestClient_Do_CircuitBreakerOpens(t *testing.T) {
	t.Parallel()

	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := newTestConfig()
	cfg.CircuitBreaker.MinRequests = 2
	cfg.CircuitBreaker.ConsecutiveFailures = 2
	cfg.Retry.RetryMax = 0

	c := client.New(cfg)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL,
		http.NoBody,
	)
	require.NoError(t, err, "failed to create request")

	resp1, err1 := c.Do(req.Clone(context.Background()))
	if resp1 != nil {
		err := resp1.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}

	require.NoError(t, err1, "shouldn't return retry error")

	resp2, err2 := c.Do(req.Clone(context.Background()))
	if resp2 != nil {
		err := resp2.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}

	require.NoError(t, err2, "shouldn't return retry error")

	resp3, err3 := c.Do(req.Clone(context.Background()))
	if resp3 != nil {
		err := resp3.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}

	require.Error(t, err3, "expected error on third request")
	assert.ErrorIs(t, err3, gobreaker.ErrOpenState, "expected ErrOpenState")

	assert.Equal(
		t,
		2,
		requestCount,
		"expected only 2 requests to reach the server",
	)
}

func TestClient_Do_CircuitBreakerRecovers(t *testing.T) {
	t.Parallel()

	requestCount := 0
	serverShouldFail := true

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++

		if serverShouldFail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := newTestConfig()
	cfg.CircuitBreaker.MinRequests = 2
	cfg.CircuitBreaker.ConsecutiveFailures = 2
	cfg.CircuitBreaker.Interval = 0
	cfg.CircuitBreaker.Timeout = 100 * time.Millisecond
	cfg.Retry.RetryMax = 0
	c := client.New(cfg)

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		server.URL,
		http.NoBody,
	)
	require.NoError(t, err, "failed to create request")

	resp1, err1 := c.Do(req.Clone(context.Background()))
	if resp1 != nil {
		err := resp1.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}

	require.NoError(t, err1, "shouldn't return retry error")

	resp2, err2 := c.Do(req.Clone(context.Background()))
	if resp2 != nil {
		err := resp2.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}

	require.NoError(t, err2, "shouldn't return retry error")

	resp3, err3 := c.Do(req.Clone(context.Background()))
	if resp3 != nil {
		err := resp3.Body.Close()
		assert.NoError(t, err, "failed to close response body")
	}

	require.Error(t, err3, "expected error on third request")
	assert.ErrorIs(t, err3, gobreaker.ErrOpenState, "expected ErrOpenState")

	assert.Equal(t, 2, requestCount, "expected 2 requests before opening")

	time.Sleep(cfg.CircuitBreaker.Timeout + 50*time.Millisecond)

	serverShouldFail = false // Simulate server recovery

	resp4, err4 := c.Do(req.Clone(context.Background()))
	require.NoError(t, err4, "expected success on fourth request")
	require.NotNil(t, resp4, "expected non-nil response")
	assert.Equal(t, http.StatusOK, resp4.StatusCode, "expected status code 200")

	err = resp4.Body.Close()
	require.NoError(t, err, "failed to close response body")

	assert.Equal(t, 3, requestCount, "expected 3rd request in half-open")

	resp5, err5 := c.Do(req.Clone(context.Background()))
	require.NoError(t, err5, "expected success on fifth request")
	require.NotNil(t, resp5, "expected non-nil response")
	assert.Equal(t, http.StatusOK, resp5.StatusCode, "expected status code 200")

	err = resp5.Body.Close()
	require.NoError(t, err, "failed to close response body")

	assert.Equal(
		t,
		4,
		requestCount,
		"expected 4th request in closed state",
	)
}
