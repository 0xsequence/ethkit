package ethrpc

import "encoding/json"

var (
	jsonMarshaler   func(v any) ([]byte, error)
	jsonUnmarshaler func(data []byte, v any) error
)

func SetJSONMarshaler(marshaler func(v any) ([]byte, error)) {
	jsonMarshaler = marshaler
}

func SetJSONUnmarshaler(unmarshaler func(data []byte, v any) error) {
	jsonUnmarshaler = unmarshaler
}

func init() {
	jsonMarshaler = json.Marshal
	jsonUnmarshaler = json.Unmarshal
}
