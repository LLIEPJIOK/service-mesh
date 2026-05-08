package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/LLIEPJIOK/service-mesh/certmanager/internal/domain"
)

type Signer struct {
	caCert  *x509.Certificate
	caKey   crypto.PrivateKey
	caPEM   []byte
	leafTTL time.Duration
}

func NewSignerFromFiles(caCertFile string, caKeyFile string, leafTTL time.Duration) (*Signer, error) {
	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("read CA cert file: %w", err)
	}

	caKeyPEM, err := os.ReadFile(caKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read CA key file: %w", err)
	}

	caCert, err := parseCertificate(caCertPEM)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	caKey, err := parsePrivateKey(caKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}

	if leafTTL <= 0 {
		return nil, fmt.Errorf("leaf TTL must be positive")
	}

	return &Signer{
		caCert:  caCert,
		caKey:   caKey,
		caPEM:   caCertPEM,
		leafTTL: leafTTL,
	}, nil
}

func (s *Signer) SignCSR(csrPEM []byte, identity domain.Identity) ([]byte, []byte, time.Time, error) {
	csr, err := parseCSR(csrPEM)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("parse csr: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("%w: csr signature check failed: %v", domain.ErrInvalidRequest, err)
	}

	notBefore := time.Now().UTC()
	notAfter := notBefore.Add(s.leafTTL)
	if notAfter.After(s.caCert.NotAfter) {
		notAfter = s.caCert.NotAfter
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("generate serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: identity.String(),
		},
		DNSNames:              []string{identity.DNSName()},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derCert, err := x509.CreateCertificate(rand.Reader, template, s.caCert, csr.PublicKey, s.caKey)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("create certificate: %w", err)
	}

	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derCert})
	return leafPEM, s.caPEM, notAfter, nil
}

func parseCSR(csrPEM []byte) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, fmt.Errorf("%w: decode PEM block", domain.ErrInvalidRequest)
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse certificate request: %v", domain.ErrInvalidRequest, err)
	}

	return csr, nil
}

func parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	return cert, nil
}

func parsePrivateKey(keyPEM []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("failed to decode private key PEM")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	return nil, errors.New("unsupported private key format")
}
