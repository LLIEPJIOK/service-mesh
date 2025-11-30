package ws

import (
	"encoding/json"
	"sync/atomic"
)

var requestIDCounter atomic.Uint64

type Message struct {
	ID      uint64          `json:"id"`
	Route   string          `json:"route"`
	Payload json.RawMessage `json:"payload"`
	Error   string          `json:"error,omitempty"`
}

func NewRequest(route string, payload any) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:      requestIDCounter.Add(1),
		Route:   route,
		Payload: data,
	}, nil
}

func NewResponse(requestID uint64, payload any) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:      requestID,
		Payload: data,
	}, nil
}

func NewErrorResponse(requestID uint64, err error) *Message {
	return &Message{
		ID:    requestID,
		Error: err.Error(),
	}
}

func (m *Message) UnmarshalPayload(v any) error {
	return json.Unmarshal(m.Payload, v)
}
