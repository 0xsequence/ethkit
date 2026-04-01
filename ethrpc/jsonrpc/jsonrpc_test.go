package jsonrpc

import (
	"encoding/json"
	"testing"
)

func TestErrorString(t *testing.T) {
	t.Run("without data", func(t *testing.T) {
		err := Error{
			Code:    -32000,
			Message: "header not found",
		}

		if got, want := err.Error(), "jsonrpc error -32000: header not found"; got != want {
			t.Fatalf("unexpected error string: got %q, want %q", got, want)
		}
	})

	t.Run("with json data", func(t *testing.T) {
		err := Error{
			Code:    3,
			Message: "execution reverted",
			Data:    json.RawMessage("{\n\t\"reason\": \"bad\",\n\t\"code\": 123\n}"),
		}

		if got, want := err.Error(), `jsonrpc error 3: execution reverted, data: {"reason":"bad","code":123}`; got != want {
			t.Fatalf("unexpected error string: got %q, want %q", got, want)
		}
	})
}
