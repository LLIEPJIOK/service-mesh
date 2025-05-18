package sidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/LLIEPJIOK/sidecar/internal/config"
	"github.com/LLIEPJIOK/sidecar/pkg/client"
)

type Client interface {
	Do(req *http.Request) (*http.Response, error)
}

type SideCar struct {
	cfg    *config.SideCar
	client Client
}

func New(cfg *config.Config) (*SideCar, error) {
	return &SideCar{
		cfg:    &cfg.SideCar,
		client: client.New(&cfg.Client),
	}, nil
}

func (c *SideCar) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", c.proxyHandler)
}

func (c *SideCar) proxyHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("incoming request", slog.String("url", r.URL.String()))

	if r.Host == fmt.Sprintf("%s-sidecar:%d", c.cfg.ServiceName, c.cfg.Port) {
		c.internalProxyHandler(w, r)

		return
	}

	c.externalProxyHandler(w, r)
}

func (c *SideCar) externalProxyHandler(w http.ResponseWriter, r *http.Request) {
	service, err := c.getServiceName(r.Host)
	if err != nil {
		slog.Error("failed to get service name", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)

		return
	}

	target, err := c.discover(r.Context(), service)
	if err != nil {
		slog.Error("failed to get target url", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)

		return
	}

	req, err := c.proxyRequest(r, target)
	if err != nil {
		slog.Error("failed to create proxy request", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)

		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("failed to proxy request", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)

		return
	}

	defer func() {
		if clErr := resp.Body.Close(); clErr != nil {
			slog.Error("failed to close body", slog.Any("error", clErr))
		}
	}()

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("failed to copy body", slog.Any("error", err))
	}
}

func (c *SideCar) internalProxyHandler(w http.ResponseWriter, r *http.Request) {
	req, err := c.proxyRequest(r, c.cfg.Target)
	if err != nil {
		slog.Error("failed to create proxy request", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)

		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("failed to proxy request", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)

		return
	}

	defer func() {
		if clErr := resp.Body.Close(); clErr != nil {
			slog.Error("failed to close body", slog.Any("error", clErr))
		}
	}()

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil && !errors.Is(err, io.EOF) {
		slog.Error("failed to copy body", slog.Any("error", err))
	}
}

func (c *SideCar) discover(ctx context.Context, name string) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://control-plane:8080/discover?service="+name,
		http.NoBody,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	defer func() {
		if clErr := resp.Body.Close(); clErr != nil {
			slog.Error("failed to close body", slog.Any("error", clErr))
		}
	}()

	var service struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&service); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return service.Address, nil
}

func (c *SideCar) proxyRequest(r *http.Request, target string) (*http.Request, error) {
	// URL: /api/{rest...}
	path := strings.TrimPrefix(r.URL.Path, "/api")

	var fullURL strings.Builder

	fullURL.WriteString("http://")
	fullURL.WriteString(target)
	fullURL.WriteString(path)

	if r.URL.RawQuery != "" {
		fullURL.WriteString("?")
		fullURL.WriteString(r.URL.RawQuery)
	}

	req, err := http.NewRequest(r.Method, fullURL.String(), r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: %w", err)
	}

	for k, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	ip := getClientIP(r)
	req.Header.Set("X-Forwarded-For", ip)

	return req, nil
}

func (c *SideCar) getServiceName(host string) (string, error) {
	parts := strings.Split(host, ".")
	if len(parts) != 2 {
		return "", ErrInvalidHost
	}

	return parts[0], nil
}

func getClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.Split(forwarded, ",")[0]
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return ip
}
