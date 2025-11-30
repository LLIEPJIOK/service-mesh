package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type ClientConfig struct {
	URL                  string
	RequestTimeout       time.Duration
	ReconnectInterval    time.Duration
	MaxReconnectAttempts int
	Logger               *slog.Logger
}

func DefaultClientConfig(wsURL string) ClientConfig {
	return ClientConfig{
		URL:                  wsURL,
		RequestTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0,
		Logger:               slog.Default(),
	}
}

type pendingRequest struct {
	responseCh chan *Message
	errCh      chan error
}

type Client struct {
	cfg      ClientConfig
	conn     *websocket.Conn
	connMu   sync.RWMutex
	writeMu  sync.Mutex
	pending  map[uint64]*pendingRequest
	pendMu   sync.RWMutex
	done     chan struct{}
	closed   bool
	closedMu sync.RWMutex
	logger   *slog.Logger
}

func NewClient(cfg ClientConfig) *Client {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Client{
		cfg:     cfg,
		pending: make(map[uint64]*pendingRequest),
		done:    make(chan struct{}),
		logger:  cfg.Logger,
	}
}

func (c *Client) Connect(ctx context.Context) error {
	u, err := url.Parse(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	c.logger.Info("connecting to server", slog.String("url", u.String()))

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()

	c.logger.Info("connected to server", "url", u.String())

	go c.readLoop()

	return nil
}

func (c *Client) readLoop() {
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

		var msg Message

		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Error("failed to unmarshal response", "error", err)
			continue
		}

		c.pendMu.Lock()

		pr, ok := c.pending[msg.ID]
		if ok {
			delete(c.pending, msg.ID)
		}

		c.pendMu.Unlock()

		if ok {
			pr.responseCh <- &msg
		} else {
			c.logger.Warn("received response for unknown request", "id", msg.ID)
		}
	}
}

func (c *Client) Request(ctx context.Context, route string, payload any) (*Message, error) {
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

	data, err := json.Marshal(msg)
	if err != nil {
		c.pendMu.Lock()
		delete(c.pending, msg.ID)
		c.pendMu.Unlock()

		return nil, fmt.Errorf("failed to marshal request: %w", err)
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

func (c *Client) RequestTyped(ctx context.Context, route string, payload any, response any) error {
	msg, err := c.Request(ctx, route, payload)
	if err != nil {
		return err
	}

	return msg.UnmarshalPayload(response)
}

func (c *Client) Close() error {
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

func (c *Client) Done() <-chan struct{} {
	return c.done
}

func (c *Client) IsClosed() bool {
	c.closedMu.RLock()
	defer c.closedMu.RUnlock()
	return c.closed
}

func (c *Client) Reconnect(ctx context.Context) error {
	c.closedMu.Lock()
	c.closed = false
	c.done = make(chan struct{})
	c.closedMu.Unlock()

	attempts := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.Connect(ctx)
		if err == nil {
			return nil
		}

		attempts++
		if c.cfg.MaxReconnectAttempts > 0 && attempts >= c.cfg.MaxReconnectAttempts {
			return ErrMaxReconnectAttempts
		}

		c.logger.Warn("reconnect failed, retrying", "attempt", attempts, "error", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.cfg.ReconnectInterval):
		}
	}
}
