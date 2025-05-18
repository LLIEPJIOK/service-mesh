package client

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/LLIEPJIOK/sidecar/pkg/client/config"
	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

type Backoff = func(mn, mx time.Duration, attemptNum int, resp *http.Response) time.Duration

type Client struct {
	retry *retryablehttp.Client
}

func New(cfg *config.Config) *Client {
	retry := retryablehttp.NewClient()
	retry.RetryMax = cfg.Retry.RetryMax
	retry.RetryWaitMin = cfg.Retry.RetryWaitMin
	retry.RetryWaitMax = cfg.Retry.RetryWaitMax
	//nolint:bodyclose // nothing to close
	retry.Backoff = customBackoff(cfg.Retry.BackoffType)
	retry.HTTPClient = configureHTTPClient(cfg)
	retry.ErrorHandler = retryablehttp.PassthroughErrorHandler

	return &Client{
		retry: retry,
	}
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	retryReq, err := retryablehttp.FromRequest(req)
	if err != nil {
		return nil, err
	}

	return c.retry.Do(retryReq)
}

func customBackoff(backoffType string) Backoff {
	var backoff Backoff

	switch backoffType {
	case "exponential":
		backoff = retryablehttp.DefaultBackoff

	case "linear":
		backoff = retryablehttp.LinearJitterBackoff

	default:
		backoff = retryablehttp.DefaultBackoff
	}

	return func(mn, mx time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp == nil {
			return retryablehttp.DefaultBackoff(mn, mx, attemptNum, resp)
		}

		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if sec, err := strconv.Atoi(ra); err == nil {
				return time.Duration(sec) * time.Second
			}
		}

		return backoff(mn, mx, attemptNum, resp)
	}
}

func configureHTTPClient(cfg *config.Config) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.HTTPClient.DialTimeout,
			KeepAlive: cfg.HTTPClient.DialKeepAlive,
		}).DialContext,
		MaxIdleConns:          cfg.HTTPClient.MaxIdleConns,
		IdleConnTimeout:       cfg.HTTPClient.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.HTTPClient.TLSHandshakeTimeout,
		ExpectContinueTimeout: cfg.HTTPClient.ExpectContinueTimeout,
	}

	return &http.Client{
		Transport: NewTransport(&cfg.CircuitBreaker, transport),
		Timeout:   cfg.HTTPClient.Timeout,
	}
}
