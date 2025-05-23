package util

import "encoding"

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
