package plane

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LLIEPJIOK/control-plane/internal/config"
	"github.com/LLIEPJIOK/control-plane/internal/domain"
)

type Mesh struct {
	mu       *sync.Mutex
	registry map[string][]*domain.Service
	idx      map[string]int
	cfg      *config.ControlPlane
	client   *http.Client
}

func New(cfg *config.ControlPlane) (*Mesh, error) {
	return &Mesh{
		mu:       &sync.Mutex{},
		registry: make(map[string][]*domain.Service),
		idx:      make(map[string]int),
		cfg:      cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (m *Mesh) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/register", m.registerHandler)
	mux.HandleFunc("/discover", m.discoverHandler)
	mux.HandleFunc("/api/", m.proxyHandler)
}

func (m *Mesh) registerHandler(w http.ResponseWriter, r *http.Request) {
	var svc domain.Service

	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.registry[svc.Name] = append(m.registry[svc.Name], &svc)
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (m *Mesh) discoverHandler(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("service")
	if name == "" {
		http.Error(w, "missing 'service' parameter", http.StatusBadRequest)

		return
	}

	m.mu.Lock()

	services, ok := m.registry[name]
	if !ok {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)

		return
	}

	service := services[m.idx[name]]

	m.idx[name]++
	if m.idx[name] == len(services) {
		m.idx[name] = 0
	}

	m.mu.Unlock()

	raw, err := json.Marshal(service)
	if err != nil {
		slog.Error("failed to marshal service", slog.Any("error", err))
		http.Error(
			w,
			http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError,
		)

		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(raw)
}

func (m *Mesh) proxyHandler(w http.ResponseWriter, r *http.Request) {
	slog.Info("incoming request", slog.String("url", r.URL.String()))

	req, err := m.proxyRequest(r)
	if err != nil {
		slog.Error("failed to create proxy request", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusBadGateway)

		return
	}

	resp, err := m.client.Do(req)
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

func (m *Mesh) getAddress(host string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	parts := strings.Split(host, ".")
	if len(parts) != 2 {
		return "", ErrInvalidHost
	}

	name := parts[0]

	services, ok := m.registry[name]
	if !ok {
		return "", ErrNotFound
	}

	service := services[m.idx[name]]

	m.idx[name]++
	if m.idx[name] == len(services) {
		m.idx[name] = 0
	}

	fmt.Println(services, m.idx)

	return service.Address, nil
}

func (m *Mesh) proxyRequest(r *http.Request) (*http.Request, error) {
	// URL: /api/{rest...}
	url := strings.TrimPrefix(r.URL.Path, "/api")

	target, err := m.getAddress(r.Host)
	if err != nil {
		return nil, err
	}

	var fullURL strings.Builder

	fullURL.WriteString("http://")
	fullURL.WriteString(target)
	fullURL.WriteString(url)

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

	return req, nil
}
