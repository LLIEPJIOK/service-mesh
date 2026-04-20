package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	appcertmanager "github.com/LLIEPJIOK/service-mesh/certmanager/internal/app/certmanager"
	"github.com/LLIEPJIOK/service-mesh/certmanager/internal/domain"
)

type signRequest struct {
	CSR   string `json:"csr"`
	Token string `json:"token"`
}

type signResponse struct {
	Certificate string `json:"certificate"`
	CA          string `json:"ca"`
	Identity    string `json:"identity"`
	ExpiresAt   string `json:"expiresAt"`
}

type signHandler struct {
	service         *appcertmanager.Service
	maxRequestBytes int64
	logger          *log.Logger
}

func NewHandler(
	service *appcertmanager.Service,
	maxRequestBytes int64,
	rateLimitRPS float64,
	rateLimitBurst int,
	logger *log.Logger,
) http.Handler {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	handler := &signHandler{
		service:         service,
		maxRequestBytes: maxRequestBytes,
		logger:          logger,
	}

	mux := http.NewServeMux()
	mux.Handle("/sign", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return applyRateLimit(mux, rateLimitRPS, rateLimitBurst)
}

func (h *signHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBytes)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request signRequest
	if err := decoder.Decode(&request); err != nil {
		h.logError("invalid JSON", "unknown", err)
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		h.logError("extra JSON tokens", "unknown", err)
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	request.CSR = strings.TrimSpace(request.CSR)
	request.Token = strings.TrimSpace(request.Token)
	if request.CSR == "" || request.Token == "" {
		h.logError("missing required fields", "unknown", nil)
		http.Error(w, "both csr and token are required", http.StatusBadRequest)
		return
	}

	result, err := h.service.Sign(r.Context(), []byte(request.CSR), request.Token)
	if err != nil {
		status := mapErrorToStatus(err)
		h.logError("sign failed", "unknown", err)
		http.Error(w, statusMessage(status), status)
		return
	}

	h.logIssued(result.Identity.String(), result.ExpiresAt)

	response := signResponse{
		Certificate: string(result.CertificatePEM),
		CA:          string(result.CAPEM),
		Identity:    result.Identity.String(),
		ExpiresAt:   result.ExpiresAt.UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logError("encode response", result.Identity.String(), err)
	}
}

func mapErrorToStatus(err error) int {
	if errors.Is(err, domain.ErrInvalidRequest) {
		return http.StatusBadRequest
	}

	if errors.Is(err, domain.ErrUnauthorized) {
		return http.StatusUnauthorized
	}

	if errors.Is(err, domain.ErrForbidden) {
		return http.StatusForbidden
	}

	return http.StatusInternalServerError
}

func statusMessage(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	default:
		return "internal server error"
	}
}

func (h *signHandler) logIssued(identity string, expiresAt time.Time) {
	h.logger.Printf("issued certificate identity=%s expiresAt=%s", identity, expiresAt.UTC().Format(time.RFC3339))
}

func (h *signHandler) logError(action string, identity string, err error) {
	if err == nil {
		h.logger.Printf("%s identity=%s", action, identity)
		return
	}

	h.logger.Printf("%s identity=%s error=%s", action, identity, sanitizeError(err))
}

func sanitizeError(err error) string {
	message := err.Error()
	message = strings.ReplaceAll(message, "\n", " ")
	if len(message) > 1024 {
		return fmt.Sprintf("%s...", message[:1024])
	}

	return message
}
