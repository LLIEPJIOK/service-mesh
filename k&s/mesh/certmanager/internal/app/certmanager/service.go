package certmanager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/LLIEPJIOK/service-mesh/certmanager/internal/domain"
)

type TokenValidator interface {
	ValidateToken(ctx context.Context, token string) (domain.Identity, error)
}

type CSRSigner interface {
	SignCSR(csrPEM []byte, identity domain.Identity) ([]byte, []byte, time.Time, error)
}

type SignResult struct {
	CertificatePEM []byte
	CAPEM          []byte
	Identity       domain.Identity
	ExpiresAt      time.Time
}

type Service struct {
	validator TokenValidator
	signer    CSRSigner
}

func NewService(validator TokenValidator, signer CSRSigner) *Service {
	return &Service{
		validator: validator,
		signer:    signer,
	}
}

func (s *Service) Sign(ctx context.Context, csrPEM []byte, token string) (SignResult, error) {
	identity, err := s.validator.ValidateToken(ctx, token)
	if err != nil {
		slog.Warn("token validation failed for sign request", slog.Any("error", err))
		return SignResult{}, fmt.Errorf("validate token: %w", err)
	}

	slog.Info(
		"token validation succeeded",
		slog.String("namespace", identity.Namespace),
		slog.String("service_account", identity.ServiceAccount),
	)

	certificatePEM, caPEM, expiresAt, err := s.signer.SignCSR(csrPEM, identity)
	if err != nil {
		slog.Warn(
			"csr signing failed",
			slog.String("namespace", identity.Namespace),
			slog.String("service_account", identity.ServiceAccount),
			slog.Any("error", err),
		)
		return SignResult{}, fmt.Errorf("sign csr: %w", err)
	}

	slog.Info(
		"csr signing succeeded",
		slog.String("namespace", identity.Namespace),
		slog.String("service_account", identity.ServiceAccount),
		slog.Time("expires_at", expiresAt.UTC()),
	)

	return SignResult{
		CertificatePEM: certificatePEM,
		CAPEM:          caPEM,
		Identity:       identity,
		ExpiresAt:      expiresAt,
	}, nil
}
