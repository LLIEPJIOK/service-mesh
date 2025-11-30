package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type SecureClientConfig struct {
	URL                  string
	RequestTimeout       time.Duration
	ReconnectInterval    time.Duration
	MaxReconnectAttempts int
	Logger               *slog.Logger
	TLS                  *TLSConfig
}

func DefaultSecureClientConfig(wsURL string) SecureClientConfig {
	return SecureClientConfig{
		URL:                  wsURL,
		RequestTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0,
		Logger:               slog.Default(),
	}
}

type SecureClient struct {
	cfg        SecureClientConfig
	conn       *websocket.Conn
	connMu     sync.RWMutex
	writeMu    sync.Mutex
	pending    map[uint64]*pendingRequest
	pendMu     sync.RWMutex
	done       chan struct{}
	closed     bool
	closedMu   sync.RWMutex
	logger     *slog.Logger
	secureConn *SecureConn
}

func NewSecureClient(cfg SecureClientConfig) *SecureClient {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &SecureClient{
		cfg:     cfg,
		pending: make(map[uint64]*pendingRequest),
		done:    make(chan struct{}),
		logger:  cfg.Logger,
	}
}

// noProxyDialer - WebSocket диалер без использования HTTP_PROXY
var noProxyDialer = websocket.Dialer{
	Proxy:            nil, // Игнорируем прокси
	HandshakeTimeout: 45 * time.Second,
}

func (c *SecureClient) Connect(ctx context.Context) error {
	u, err := url.Parse(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	c.logger.Info("connecting to server", slog.String("url", u.String()))

	conn, _, err := noProxyDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	c.logger.Info("connected to server, starting handshake", "url", u.String())

	if err := c.performHandshake(ctx); err != nil {
		conn.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	c.logger.Info("handshake completed", "url", u.String())

	go c.readLoop()

	return nil
}

func (c *SecureClient) performHandshake(ctx context.Context) error {
	if c.cfg.TLS == nil {
		return errors.New("TLS config is required for secure connection")
	}

	secureConn, err := NewSecureConn(c.cfg.TLS, true)
	if err != nil {
		return err
	}

	initMsg, err := secureConn.CreateHandshakeInit()
	if err != nil {
		return err
	}
	data, err := json.Marshal(initMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake init: %w", err)
	}

	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("failed to send handshake init: %w", err)
	}

	_, responseData, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read handshake response: %w", err)
	}

	var responseMsg HandshakeMessage
	if err := json.Unmarshal(responseData, &responseMsg); err != nil {
		return fmt.Errorf("failed to unmarshal handshake response: %w", err)
	}

	if err := secureConn.ProcessHandshakeResponse(&responseMsg); err != nil {
		return err
	}

	c.secureConn = secureConn
	return nil
}

func (c *SecureClient) readLoop() {
	defer func() {
		c.closedMu.Lock()
		c.closed = true
		c.closedMu.Unlock()
		close(c.done)

		c.pendMu.Lock()
		for _, pr := range c.pending {
			pr.errCh <- ErrConnectionClosed
		}
		c.pending = make(map[uint64]*pendingRequest)
		c.pendMu.Unlock()
	}()

	for {
		c.connMu.RLock()
		conn := c.conn
		c.connMu.RUnlock()

		if conn == nil {
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(
				err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
			) {
				c.logger.Error("read error", "error", err)
			}
			return
		}

		var encMsg EncryptedMessage
		if err := json.Unmarshal(data, &encMsg); err != nil {
			c.logger.Error("failed to unmarshal encrypted message", "error", err)
			continue
		}

		msg, err := c.secureConn.DecryptMessage(&encMsg)
		if err != nil {
			c.logger.Error("failed to decrypt message", "error", err)
			continue
		}

		c.pendMu.Lock()
		pr, ok := c.pending[msg.ID]
		if ok {
			delete(c.pending, msg.ID)
		}
		c.pendMu.Unlock()

		if ok {
			pr.responseCh <- msg
		} else {
			c.logger.Warn("received response for unknown request", "id", msg.ID)
		}
	}
}

func (c *SecureClient) Request(ctx context.Context, route string, payload any) (*Message, error) {
	c.closedMu.RLock()
	if c.closed {
		c.closedMu.RUnlock()
		return nil, ErrConnectionClosed
	}
	c.closedMu.RUnlock()

	msg, err := NewRequest(route, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	pr := &pendingRequest{
		responseCh: make(chan *Message, 1),
		errCh:      make(chan error, 1),
	}

	c.pendMu.Lock()
	c.pending[msg.ID] = pr
	c.pendMu.Unlock()

	encMsg, err := c.secureConn.EncryptMessage(msg)
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, msg.ID)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("failed to encrypt request: %w", err)
	}

	data, err := json.Marshal(encMsg)
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, msg.ID)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("failed to marshal encrypted request: %w", err)
	}

	c.writeMu.Lock()
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		c.writeMu.Unlock()
		c.pendMu.Lock()
		delete(c.pending, msg.ID)
		c.pendMu.Unlock()
		return nil, ErrConnectionClosed
	}

	err = conn.WriteMessage(websocket.TextMessage, data)
	c.writeMu.Unlock()

	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, msg.ID)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	timeout := c.cfg.RequestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d < timeout {
			timeout = d
		}
	}

	select {
	case resp := <-pr.responseCh:
		if resp.Error != "" {
			return nil, fmt.Errorf("%w: %s", ErrServerError, resp.Error)
		}
		return resp, nil

	case err := <-pr.errCh:
		return nil, err

	case <-time.After(timeout):
		c.pendMu.Lock()
		delete(c.pending, msg.ID)
		c.pendMu.Unlock()
		return nil, ErrRequestTimeout

	case <-ctx.Done():
		c.pendMu.Lock()
		delete(c.pending, msg.ID)
		c.pendMu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *SecureClient) RequestTyped(
	ctx context.Context,
	route string,
	payload any,
	response any,
) error {
	msg, err := c.Request(ctx, route, payload)
	if err != nil {
		return err
	}
	return msg.UnmarshalPayload(response)
}

func (c *SecureClient) Close() error {
	c.closedMu.Lock()
	if c.closed {
		c.closedMu.Unlock()
		return nil
	}
	c.closed = true
	c.closedMu.Unlock()

	c.connMu.Lock()
	conn := c.conn
	c.conn = nil
	c.connMu.Unlock()

	if conn != nil {
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client closing")
		_ = conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))
		return conn.Close()
	}

	return nil
}

func (c *SecureClient) Done() <-chan struct{} {
	return c.done
}

func (c *SecureClient) IsClosed() bool {
	c.closedMu.RLock()
	defer c.closedMu.RUnlock()
	return c.closed
}

func (c *SecureClient) PeerCertificate() interface{} {
	if c.secureConn == nil {
		return nil
	}
	return c.secureConn.PeerCertificate()
}
