package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Handler func(ctx context.Context, payload json.RawMessage) (any, error)

type ServerConfig struct {
	ReadBufferSize  int
	WriteBufferSize int
	CheckOrigin     func(r *http.Request) bool
	Logger          *slog.Logger
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(r *http.Request) bool { return true },
		Logger:          slog.Default(),
	}
}

type Server struct {
	upgrader websocket.Upgrader
	handlers map[string]Handler
	mu       sync.RWMutex
	logger   *slog.Logger
}

func NewServer(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Server{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  cfg.ReadBufferSize,
			WriteBufferSize: cfg.WriteBufferSize,
			CheckOrigin:     cfg.CheckOrigin,
		},
		handlers: make(map[string]Handler),
		logger:   cfg.Logger,
	}
}

func (s *Server) Handle(route string, handler Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[route] = handler
}

func (s *Server) getHandler(route string) (Handler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.handlers[route]
	return h, ok
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade connection", "error", err)
		return
	}
	defer conn.Close()

	s.logger.Info("client connected", "remote_addr", conn.RemoteAddr())
	defer s.logger.Info("client disconnected", "remote_addr", conn.RemoteAddr())

	s.handleConnection(r.Context(), conn)
}

func (s *Server) handleConnection(ctx context.Context, conn *websocket.Conn) {
	writeMu := &sync.Mutex{}

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

		var msg Message

		if err := json.Unmarshal(data, &msg); err != nil {
			s.logger.Error("failed to unmarshal message", "error", err)
			continue
		}

		go s.processRequest(ctx, conn, writeMu, &msg)
	}
}

func (s *Server) processRequest(
	ctx context.Context,
	conn *websocket.Conn,
	writeMu *sync.Mutex,
	msg *Message,
) {
	handler, ok := s.getHandler(msg.Route)
	if !ok {
		s.sendError(conn, writeMu, msg.ID, fmt.Errorf("%w: %s", ErrRouteNotFound, msg.Route))
		return
	}

	result, err := handler(ctx, msg.Payload)
	if err != nil {
		s.sendError(conn, writeMu, msg.ID, err)
		return
	}

	resp, err := NewResponse(msg.ID, result)
	if err != nil {
		s.sendError(conn, writeMu, msg.ID, err)
		return
	}

	s.sendMessage(conn, writeMu, resp)
}

func (s *Server) sendError(conn *websocket.Conn, mu *sync.Mutex, requestID uint64, err error) {
	resp := NewErrorResponse(requestID, err)
	s.sendMessage(conn, mu, resp)
}

func (s *Server) sendMessage(conn *websocket.Conn, mu *sync.Mutex, msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error("failed to marshal response", "error", err)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		s.logger.Error("failed to write message", "error", err)
	}
}
