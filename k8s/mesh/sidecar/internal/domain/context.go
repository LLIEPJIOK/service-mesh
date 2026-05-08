package domain

import (
	"context"
	"net"
)

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type ConnContext struct {
	Context     context.Context
	ClientConn  net.Conn
	OriginalDst string
	Metadata    map[string]any
}

type Endpoint struct {
	IP          string
	Port        int
	ServiceName string
}

func (c *ConnContext) CloneWithContext(ctx context.Context) *ConnContext {
	clonedMetadata := make(map[string]any, len(c.Metadata))
	for key, value := range c.Metadata {
		clonedMetadata[key] = value
	}

	return &ConnContext{
		Context:     ctx,
		ClientConn:  c.ClientConn,
		OriginalDst: c.OriginalDst,
		Metadata:    clonedMetadata,
	}
}

func (c *ConnContext) Set(key string, value any) {
	if c.Metadata == nil {
		c.Metadata = make(map[string]any)
	}

	c.Metadata[key] = value
}

func (c *ConnContext) GetString(key string) string {
	if c.Metadata == nil {
		return ""
	}

	value, ok := c.Metadata[key]
	if !ok {
		return ""
	}

	stringValue, ok := value.(string)
	if !ok {
		return ""
	}

	return stringValue
}

func (c *ConnContext) GetBool(key string) bool {
	if c.Metadata == nil {
		return false
	}

	value, ok := c.Metadata[key]
	if !ok {
		return false
	}

	boolValue, ok := value.(bool)
	if !ok {
		return false
	}

	return boolValue
}
