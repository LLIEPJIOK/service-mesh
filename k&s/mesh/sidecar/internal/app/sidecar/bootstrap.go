package sidecar

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/LLIEPJIOK/sidecar/internal/adapters/certmanager"
	"github.com/LLIEPJIOK/sidecar/internal/adapters/proxy"
	"github.com/LLIEPJIOK/sidecar/internal/config"
)

func bootstrapTLSConfig(ctx context.Context, cfg config.Config) (*tls.Config, error) {
	if !cfg.BootstrapCertificates {
		return proxy.BuildTLSFromFiles(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	}

	tokenRaw, err := os.ReadFile(cfg.ServiceAccountTokenPath)
	if err != nil {
		return nil, fmt.Errorf("read service account token: %w", err)
	}

	token := strings.TrimSpace(string(tokenRaw))
	if token == "" {
		return nil, fmt.Errorf("service account token is empty")
	}

	keyPEM, csrPEM, err := generateCSR(cfg)
	if err != nil {
		return nil, fmt.Errorf("generate csr: %w", err)
	}

	client := certmanager.NewClient(cfg.CertManagerSignURL, nil)
	leafCertPEM, caPEM, err := client.Sign(ctx, csrPEM, token)
	if err != nil {
		return nil, fmt.Errorf("request certificate from cert-manager: %w", err)
	}

	tlsConfig, err := proxy.BuildTLSFromIssuedMaterial(leafCertPEM, keyPEM, caPEM)
	if err != nil {
		return nil, fmt.Errorf("build tls config from issued certificate: %w", err)
	}

	return tlsConfig, nil
}

func generateCSR(cfg config.Config) ([]byte, []byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate rsa private key: %w", err)
	}

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: fmt.Sprintf("%s.%s", cfg.ServiceAccount, cfg.Namespace),
		},
		DNSNames: []string{
			fmt.Sprintf("%s.%s.pod.cluster.local", cfg.PodName, cfg.Namespace),
			fmt.Sprintf("%s.%s", cfg.ServiceAccount, cfg.Namespace),
		},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate request: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return keyPEM, csrPEM, nil
}
