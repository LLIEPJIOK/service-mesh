package proxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type Forwarder struct {
	TLSConfig   *tls.Config
	DialTimeout time.Duration
}

func NewForwarder(tlsConfig *tls.Config, dialTimeout time.Duration) *Forwarder {
	return &Forwarder{
		TLSConfig:   tlsConfig,
		DialTimeout: dialTimeout,
	}
}

func (f *Forwarder) Handle(ctx *domain.ConnContext) error {
	targetAddr := ctx.GetString(domain.MetadataTargetAddr)
	if targetAddr == "" {
		return domain.Wrap(domain.ErrorKindProxy, fmt.Errorf("missing target address"))
	}

	inMesh := ctx.GetBool(domain.MetadataInMesh)
	serverName := ctx.GetString(domain.MetadataServerName)

	slog.Debug(
		"forward routing decision",
		slog.String("target", targetAddr),
		slog.Bool("in_mesh", inMesh),
		slog.String("server_name", serverName),
	)

	var (
		targetConn net.Conn
		err        error
	)

	if inMesh {
		if f.TLSConfig == nil {
			slog.Error("forward mTLS dial skipped due to missing tls config", slog.String("target", targetAddr))
			return domain.Wrap(domain.ErrorKindTLS, fmt.Errorf("invalid tls configuration"))
		}

		targetConn, err = DialMTLS(ctx.Context, targetAddr, serverName, f.TLSConfig, f.DialTimeout)
		if err != nil {
			slog.Warn(
				"forward mTLS dial failed",
				slog.String("target", targetAddr),
				slog.String("server_name", serverName),
				slog.Any("error", err),
			)
			return err
		}

		slog.Info(
			"forward mTLS dial established",
			slog.String("target", targetAddr),
			slog.String("server_name", serverName),
		)
	} else {
		dialer := &net.Dialer{Timeout: f.DialTimeout}
		targetConn, err = dialer.DialContext(ctx.Context, "tcp", targetAddr)
		if err != nil {
			slog.Warn("forward plain dial failed", slog.String("target", targetAddr), slog.Any("error", err))
			return domain.ClassifyDialError(err)
		}

		slog.Debug("forward plain dial established", slog.String("target", targetAddr))
	}
	defer targetConn.Close()

	if err := bridgeConnections(ctx.ClientConn, targetConn); err != nil {
		return domain.Wrap(domain.ErrorKindProxy, err)
	}

	return nil
}

func bridgeConnections(clientConn net.Conn, targetConn net.Conn) error {
	errCh := make(chan error, 2)

	go func() {
		errCh <- copyStream(targetConn, clientConn)
	}()

	go func() {
		errCh <- copyStream(clientConn, targetConn)
	}()

	errFirst := <-errCh
	errSecond := <-errCh

	if !isStreamTerminationError(errFirst) {
		return errFirst
	}

	if !isStreamTerminationError(errSecond) {
		return errSecond
	}

	return nil
}

func copyStream(dst net.Conn, src net.Conn) error {
	_, err := io.Copy(dst, src)
	closeWrite(dst)
	return err
}

func closeWrite(conn net.Conn) {
	tcpConn, ok := conn.(*net.TCPConn)
	if ok {
		_ = tcpConn.CloseWrite()
	}
}

func isStreamTerminationError(err error) bool {
	if err == nil {
		return true
	}

	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "use of closed network connection") {
		return true
	}

	return false
}
