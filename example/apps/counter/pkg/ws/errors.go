package ws

import "errors"

var (
	ErrRouteNotFound        = errors.New("route not found")
	ErrConnectionClosed     = errors.New("connection closed")
	ErrRequestTimeout       = errors.New("request timeout")
	ErrServerError          = errors.New("server error")
	ErrMaxReconnectAttempts = errors.New("max reconnect attempts reached")
)
