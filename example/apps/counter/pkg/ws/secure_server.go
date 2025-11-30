package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type SecureServerConfig struct {
	ReadBufferSize  int
	WriteBufferSize int
	CheckOrigin     func(r *http.Request) bool
	Logger          *slog.Logger
	TLS             *TLSConfig
}

func DefaultSecureServerConfig() SecureServerConfig {
	return SecureServerConfig{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(r *http.Request) bool { return true },
		Logger:          slog.Default(),
	}
}

type SecureServer struct {
	upgrader  websocket.Upgrader
	handlers  map[string]Handler
	mu        sync.RWMutex
	logger    *slog.Logger
	tlsConfig *TLSConfig
}

func NewSecureServer(cfg SecureServerConfig) *SecureServer {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &SecureServer{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  cfg.ReadBufferSize,
			WriteBufferSize: cfg.WriteBufferSize,
			CheckOrigin:     cfg.CheckOrigin,
		},
		handlers:  make(map[string]Handler),
		logger:    cfg.Logger,
		tlsConfig: cfg.TLS,
	}
}

func (s *SecureServer) Handle(route string, handler Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[route] = handler
}

func (s *SecureServer) getHandler(route string) (Handler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.handlers[route]
	return h, ok
}

func (s *SecureServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade connection", "error", err)
		return
	}
	defer conn.Close()

	s.logger.Info("client connected", "remote_addr", conn.RemoteAddr())
	defer s.logger.Info("client disconnected", "remote_addr", conn.RemoteAddr())

	secureConn, err := s.performHandshake(conn)
	if err != nil {
		s.logger.Error("handshake failed", "error", err, "remote_addr", conn.RemoteAddr())
		return
	}

	s.logger.Info("handshake completed", "remote_addr", conn.RemoteAddr())

	s.handleConnection(r.Context(), conn, secureConn)
}

func (s *SecureServer) performHandshake(conn *websocket.Conn) (*SecureConn, error) {
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read handshake init: %w", err)
	}

	var initMsg HandshakeMessage
	if err := json.Unmarshal(data, &initMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal handshake init: %w", err)
	}

	if s.tlsConfig == nil {
		return nil, errors.New("TLS config is required for secure connection")
	}

	secureConn, err := NewSecureConn(s.tlsConfig, false)
	if err != nil {
		return nil, err
	}

	responseMsg, err := secureConn.ProcessHandshakeInit(&initMsg)
	if err != nil {
		return nil, err
	}

	responseData, err := json.Marshal(responseMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal handshake response: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
		return nil, fmt.Errorf("failed to send handshake response: %w", err)
	}

	return secureConn, nil
}

type secureConnContext struct {
	secureConn *SecureConn
	writeMu    *sync.Mutex
}

func (s *SecureServer) handleConnection(
	ctx context.Context,
	conn *websocket.Conn,
	secureConn *SecureConn,
) {
	sctx := &secureConnContext{
		secureConn: secureConn,
		writeMu:    &sync.Mutex{},
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(
				err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
			) {
				s.logger.Error("read error", "error", err)
			}
			return
		}

		var encMsg EncryptedMessage
		if err := json.Unmarshal(data, &encMsg); err != nil {
			s.logger.Error("failed to unmarshal encrypted message", "error", err)
			continue
		}

		msg, err := secureConn.DecryptMessage(&encMsg)
		if err != nil {
			s.logger.Error("failed to decrypt message", "error", err)
			continue
		}

		go s.processRequest(ctx, conn, sctx, msg)
	}
}

func (s *SecureServer) processRequest(
	ctx context.Context,
	conn *websocket.Conn,
	sctx *secureConnContext,
	msg *Message,
) {
	handler, ok := s.getHandler(msg.Route)
	if !ok {
		s.sendError(conn, sctx, msg.ID, fmt.Errorf("%w: %s", ErrRouteNotFound, msg.Route))
		return
	}

	result, err := handler(ctx, msg.Payload)
	if err != nil {
		s.sendError(conn, sctx, msg.ID, err)
		return
	}

	resp, err := NewResponse(msg.ID, result)
	if err != nil {
		s.sendError(conn, sctx, msg.ID, err)
		return
	}

	s.sendMessage(conn, sctx, resp)
}

func (s *SecureServer) sendError(
	conn *websocket.Conn,
	sctx *secureConnContext,
	requestID uint64,
	err error,
) {
	resp := NewErrorResponse(requestID, err)
	s.sendMessage(conn, sctx, resp)
}

func (s *SecureServer) sendMessage(conn *websocket.Conn, sctx *secureConnContext, msg *Message) {
	encMsg, err := sctx.secureConn.EncryptMessage(msg)
	if err != nil {
		s.logger.Error("failed to encrypt response", "error", err)
		return
	}

	data, err := json.Marshal(encMsg)
	if err != nil {
		s.logger.Error("failed to marshal encrypted response", "error", err)
		return
	}

	sctx.writeMu.Lock()
	defer sctx.writeMu.Unlock()

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		s.logger.Error("failed to write message", "error", err)
	}
}
