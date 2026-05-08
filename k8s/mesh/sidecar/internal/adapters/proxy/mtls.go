package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

func BuildTLSFromFiles(certFile string, keyFile string, caFile string) (*tls.Config, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("read cert file: %w", err)
	}

	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca file: %w", err)
	}

	return BuildTLSFromIssuedMaterial(certPEM, keyPEM, caPEM)
}

func BuildTLSFromIssuedMaterial(certPEM []byte, keyPEM []byte, caPEM []byte) (*tls.Config, error) {
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("append CA cert failed")
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		RootCAs:      caPool,
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}

func DialMTLS(
	ctx context.Context,
	address string,
	serverName string,
	baseConfig *tls.Config,
	dialTimeout time.Duration,
) (net.Conn, error) {
	if baseConfig == nil {
		return nil, domain.Wrap(domain.ErrorKindTLS, fmt.Errorf("missing tls config"))
	}

	if serverName == "" {
		return nil, domain.Wrap(domain.ErrorKindTLS, fmt.Errorf("missing tls server name"))
	}

	clientConfig := baseConfig.Clone()
	clientConfig.ClientAuth = tls.NoClientCert
	clientConfig.ServerName = serverName

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: dialTimeout,
		},
		Config: clientConfig,
	}

	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, domain.ClassifyDialError(err)
	}

	return connection, nil
}
