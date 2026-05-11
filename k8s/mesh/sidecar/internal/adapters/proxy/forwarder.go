package proxy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type Forwarder struct {
	TLSConfig      *tls.Config
	DialTimeout    time.Duration
	CopyMode       CopyMode
	transportMu    sync.Mutex
	httpTransports map[string]*http.Transport
}

type CopyMode string

const (
	CopyModeBuffered CopyMode = "buffered"
	CopyModeZeroCopy CopyMode = "zero-copy"
)

func NewForwarder(tlsConfig *tls.Config, dialTimeout time.Duration, copyMode CopyMode) *Forwarder {
	if copyMode == "" {
		copyMode = CopyModeBuffered
	}

	return &Forwarder{
		TLSConfig:      tlsConfig,
		DialTimeout:    dialTimeout,
		CopyMode:       copyMode,
		httpTransports: make(map[string]*http.Transport),
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
		clientReader := bufio.NewReader(ctx.ClientConn)
		if f.TLSConfig == nil {
			slog.Error("forward mTLS dial skipped due to missing tls config", slog.String("target", targetAddr))
			return domain.Wrap(domain.ErrorKindTLS, fmt.Errorf("invalid tls configuration"))
		}

		if handled, err := f.handleHTTP(ctx, targetAddr, serverName, clientReader); handled {
			return err
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

		slog.Debug(
			"forward mTLS dial established",
			slog.String("target", targetAddr),
			slog.String("server_name", serverName),
		)

		defer targetConn.Close()
		if err := bridgeConnectionsWithReader(ctx.ClientConn, clientReader, targetConn, f.CopyMode); err != nil {
			return domain.Wrap(domain.ErrorKindProxy, err)
		}

		return nil
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

	if err := bridgeConnections(ctx.ClientConn, targetConn, f.CopyMode); err != nil {
		return domain.Wrap(domain.ErrorKindProxy, err)
	}

	return nil
}

func (f *Forwarder) handleHTTP(ctx *domain.ConnContext, targetAddr string, serverName string, reader *bufio.Reader) (bool, error) {
	if !looksLikeHTTPRequest(ctx.ClientConn, reader) {
		return false, nil
	}

	transport := f.httpTransport(serverName)
	for {
		request, err := http.ReadRequest(reader)
		if err != nil {
			if isStreamTerminationError(err) {
				return true, nil
			}
			return true, domain.Wrap(domain.ErrorKindProxy, err)
		}

		request.RequestURI = ""
		request.URL.Scheme = "https"
		request.URL.Host = targetAddr
		if request.URL.Path == "" {
			request.URL.Path = "/"
		}

		response, err := roundTripHTTP(ctx, transport, request)
		if err != nil {
			return true, domain.Wrap(domain.ErrorKindProxy, err)
		}

		writeErr := response.Write(ctx.ClientConn)
		closeErr := response.Body.Close()
		if writeErr != nil {
			return true, domain.Wrap(domain.ErrorKindProxy, writeErr)
		}
		if closeErr != nil {
			return true, domain.Wrap(domain.ErrorKindProxy, closeErr)
		}

		if request.Close || response.Close {
			return true, nil
		}
	}
}

func roundTripHTTP(ctx *domain.ConnContext, transport *http.Transport, request *http.Request) (*http.Response, error) {
	response, err := transport.RoundTrip(request.WithContext(ctx.Context))
	if err == nil {
		return response, nil
	}

	transport.CloseIdleConnections()
	if !canReplayHTTPRequest(request) {
		return nil, err
	}

	retryRequest := request.Clone(ctx.Context)
	if request.GetBody != nil {
		body, bodyErr := request.GetBody()
		if bodyErr != nil {
			return nil, err
		}
		retryRequest.Body = body
	}

	return transport.RoundTrip(retryRequest)
}

func canReplayHTTPRequest(request *http.Request) bool {
	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
	default:
		return false
	}

	return request.Body == nil || request.Body == http.NoBody || request.GetBody != nil
}

func (f *Forwarder) httpTransport(serverName string) *http.Transport {
	f.transportMu.Lock()
	defer f.transportMu.Unlock()

	if transport, ok := f.httpTransports[serverName]; ok {
		return transport
	}

	tlsConfig := f.TLSConfig.Clone()
	tlsConfig.ClientAuth = tls.NoClientCert
	tlsConfig.ServerName = serverName

	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: f.DialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     tlsConfig,
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        1024,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: f.DialTimeout,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	f.httpTransports[serverName] = transport
	return transport
}

func looksLikeHTTPRequest(conn net.Conn, reader *bufio.Reader) bool {
	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		return false
	}
	prefix, err := reader.Peek(4)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return false
	}

	switch string(prefix) {
	case "GET ", "POST", "PUT ", "HEAD", "DELE", "PATC", "OPTI":
		return true
	default:
		return false
	}
}

func bridgeConnections(clientConn net.Conn, targetConn net.Conn, copyMode CopyMode) error {
	errCh := make(chan error, 2)

	go func() {
		errCh <- copyStream(targetConn, clientConn, copyMode)
	}()

	go func() {
		errCh <- copyStream(clientConn, targetConn, copyMode)
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

func bridgeConnectionsWithReader(clientConn net.Conn, clientReader *bufio.Reader, targetConn net.Conn, copyMode CopyMode) error {
	errCh := make(chan error, 2)

	go func() {
		errCh <- copyStreamFromReader(targetConn, clientReader, copyMode)
	}()

	go func() {
		errCh <- copyStream(clientConn, targetConn, copyMode)
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

func copyStream(dst net.Conn, src net.Conn, copyMode CopyMode) error {
	if copyMode == CopyModeZeroCopy {
		if copied, err := copyStreamZeroCopy(dst, src); copied {
			closeWrite(dst)
			return err
		}
	}

	_, err := copyStreamBuffered(dst, src)
	closeWrite(dst)
	return err
}

func copyStreamFromReader(dst net.Conn, src *bufio.Reader, copyMode CopyMode) error {
	if src.Buffered() > 0 {
		if _, err := copyStreamBuffered(dst, io.LimitReader(src, int64(src.Buffered()))); err != nil {
			closeWrite(dst)
			return err
		}
	}

	_, err := copyStreamBuffered(dst, src)
	closeWrite(dst)
	return err
}

type plainReader struct {
	io.Reader
}

type plainWriter struct {
	io.Writer
}

func copyStreamBuffered(dst io.Writer, src io.Reader) (int64, error) {
	return io.CopyBuffer(plainWriter{Writer: dst}, plainReader{Reader: src}, make([]byte, 32*1024))
}

func copyStreamZeroCopy(dst net.Conn, src net.Conn) (bool, error) {
	tcpDst, ok := dst.(*net.TCPConn)
	if !ok {
		return false, nil
	}

	tcpSrc, ok := src.(*net.TCPConn)
	if !ok {
		return false, nil
	}

	_, err := tcpDst.ReadFrom(tcpSrc)
	return true, err
}

func closeWrite(conn net.Conn) {
	type closeWriter interface {
		CloseWrite() error
	}

	if writer, ok := conn.(closeWriter); ok {
		_ = writer.CloseWrite()
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
