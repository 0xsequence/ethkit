package sonic

import "github.com/bytedance/sonic"

var Config = sonic.Config{
	// We can set this to true after updating `MarshalText()` methods to return quoted strings.
	NoQuoteTextMarshaler:    false,
	NoValidateJSONMarshaler: true,
	NoValidateJSONSkip:      true,
}.Froze()
