package certmanager

import (
	"context"
	"fmt"
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
		return SignResult{}, fmt.Errorf("validate token: %w", err)
	}

	certificatePEM, caPEM, expiresAt, err := s.signer.SignCSR(csrPEM, identity)
	if err != nil {
		return SignResult{}, fmt.Errorf("sign csr: %w", err)
	}

	return SignResult{
		CertificatePEM: certificatePEM,
		CAPEM:          caPEM,
		Identity:       identity,
		ExpiresAt:      expiresAt,
	}, nil
}
