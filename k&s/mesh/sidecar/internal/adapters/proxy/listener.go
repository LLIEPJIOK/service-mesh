package proxy

import (
	"context"
	"fmt"
	"net"

	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type ListenerProfile string

const (
	ProfileInboundPlain ListenerProfile = "inbound_plain"
	ProfileOutbound     ListenerProfile = "outbound"
	ProfileInboundMTLS  ListenerProfile = "inbound_mtls"
)

type TransparentListener struct {
	profile  ListenerProfile
	listener net.Listener
}

func NewTCPListener(addr string, profile ListenerProfile) (*TransparentListener, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}

	return NewFromListener(profile, listener), nil
}

func NewFromListener(profile ListenerProfile, listener net.Listener) *TransparentListener {
	return &TransparentListener{
		profile:  profile,
		listener: listener,
	}
}

func (l *TransparentListener) Profile() ListenerProfile {
	return l.profile
}

func (l *TransparentListener) Addr() net.Addr {
	return l.listener.Addr()
}

func (l *TransparentListener) Close() error {
	return l.listener.Close()
}

func (l *TransparentListener) Accept() (*domain.ConnContext, error) {
	conn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}

	originalDst := conn.LocalAddr().String()
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		resolvedDst, dstErr := GetOriginalDst(tcpConn)
		if dstErr == nil && resolvedDst != "" {
			originalDst = resolvedDst
		}
	}

	metadata := map[string]any{
		domain.MetadataListener:  string(l.profile),
		domain.MetadataDirection: string(directionForProfile(l.profile)),
	}

	return &domain.ConnContext{
		Context:     context.Background(),
		ClientConn:  conn,
		OriginalDst: originalDst,
		Metadata:    metadata,
	}, nil
}

func directionForProfile(profile ListenerProfile) domain.Direction {
	switch profile {
	case ProfileOutbound:
		return domain.DirectionOutbound
	default:
		return domain.DirectionInbound
	}
}
