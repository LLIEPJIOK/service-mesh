package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/LLIEPJIOK/service-mesh/certmanager/internal/domain"
)

func TestSignerRejectsInvalidCSR(t *testing.T) {
	t.Parallel()

	signer := mustNewTestSigner(t)
	_, _, _, err := signer.SignCSR([]byte("not-a-csr"), domain.Identity{Namespace: "default", ServiceAccount: "reviews"})
	if err == nil {
		t.Fatalf("expected error for invalid CSR")
	}

	if !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("expected invalid request error, got %v", err)
	}
}

func TestSignerIssuesCertificate(t *testing.T) {
	t.Parallel()

	identity := domain.Identity{Namespace: "default", ServiceAccount: "reviews"}
	signer := mustNewTestSigner(t)
	_, csrPEM := mustNewCSR(t)

	leafPEM, caPEM, expiresAt, err := signer.SignCSR(csrPEM, identity)
	if err != nil {
		t.Fatalf("sign csr: %v", err)
	}

	leaf := mustParseCertificate(t, leafPEM)
	if leaf.Subject.CommonName != identity.String() {
		t.Fatalf("unexpected CN: got %q want %q", leaf.Subject.CommonName, identity.String())
	}

	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != identity.DNSName() {
		t.Fatalf("unexpected DNS SANs: %#v", leaf.DNSNames)
	}

	if !leaf.NotAfter.Equal(expiresAt.Truncate(time.Second)) {
		t.Fatalf("expiresAt mismatch: cert=%s response=%s", leaf.NotAfter, expiresAt)
	}

	if len(caPEM) == 0 {
		t.Fatalf("CA PEM must not be empty")
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatalf("append CA PEM to cert pool")
	}

	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		t.Fatalf("verify leaf with CA: %v", err)
	}
}

func mustNewTestSigner(t *testing.T) *Signer {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(2 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	return &Signer{
		caCert:  caCert,
		caKey:   caKey,
		caPEM:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		leafTTL: time.Hour,
	}
}

func mustNewCSR(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: "ignored-in-certmanager"},
		DNSNames: []string{"ignored.example"},
	}, key)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	return key, csrPEM
}

func mustParseCertificate(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatalf("decode cert PEM failed")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	return cert
}
