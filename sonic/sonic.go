package sonic

import (
	"encoding"

	"github.com/bytedance/sonic"
)

var Config = sonic.Config{
	// We can set this to true after updating `MarshalText()` methods to return quoted strings.
	NoQuoteTextMarshaler:    true,
	NoValidateJSONMarshaler: true,
	NoValidateJSONSkip:      true,
}.Froze()

// QuoteString adds quotes to the byte slice returned by the TextMarshaler.
func QuoteString(b encoding.TextMarshaler) ([]byte, error) {
	raw, err := b.MarshalText()
	if err != nil {
		return nil, err
	}
	raw = append([]byte{'"'}, raw...)
	raw = append(raw, '"')
	return raw, nil
}
