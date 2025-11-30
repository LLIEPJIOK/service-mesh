package client

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/LLIEPJIOK/service-mesh/ws/pkg/ws"
)

// TLSConfigFromEnv загружает TLS конфигурацию из переменных окружения
// TLS_CERT - сертификат в base64
// TLS_KEY - приватный ключ в base64
// TLS_CA - CA сертификат в base64
func TLSConfigFromEnv() (*ws.TLSConfig, error) {
	certB64 := os.Getenv("TLS_CERT")
	keyB64 := os.Getenv("TLS_KEY")
	caB64 := os.Getenv("TLS_CA")

	if certB64 == "" || keyB64 == "" {
		return nil, fmt.Errorf("TLS_CERT and TLS_KEY environment variables are required")
	}

	certPEM, err := base64.StdEncoding.DecodeString(certB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode TLS_CERT: %w", err)
	}

	keyPEM, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode TLS_KEY: %w", err)
	}

	cfg := &ws.TLSConfig{
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
	}

	// CA необязателен, но рекомендуется
	if caB64 != "" {
		caPEM, err := base64.StdEncoding.DecodeString(caB64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode TLS_CA: %w", err)
		}

		rootCAs := x509.NewCertPool()
		if !rootCAs.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		cfg.RootCAs = rootCAs
	}

	return cfg, nil
}
