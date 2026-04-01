package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Message is either a JSONRPC request or response.
type Message struct {
	Version string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  []any           `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// NewRequest returns a new JSONRPC request Message.
func NewRequest(id uint64, method string, params []any) Message {
	return Message{
		Version: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

// Error is a JSONRPC error returned from the node.
type Error struct {
	Code    int             `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e Error) Error() string {
	message := fmt.Sprintf("jsonrpc error %v: %v", e.Code, e.Message)
	if len(e.Data) == 0 {
		return message
	}

	var data bytes.Buffer
	if err := json.Compact(&data, e.Data); err != nil {
		return fmt.Sprintf("%v, data: %q", message, string(e.Data))
	}

	return fmt.Sprintf("%v, data: %v", message, data.String())
}
