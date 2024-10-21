package ethcoder

// Deprecated: use ABIPackArguments instead
func AbiCoder(argTypes []string, argValues []interface{}) ([]byte, error) {
	return ABIPackArguments(argTypes, argValues)
}

// Deprecated: use ABIPackArgumentsHex instead
func AbiCoderHex(argTypes []string, argValues []interface{}) (string, error) {
	return ABIPackArgumentsHex(argTypes, argValues)
}

// Deprecated: use ABIUnpackArgumentsByRef instead
func AbiDecoder(argTypes []string, input []byte, outArgValues []interface{}) error {
	return ABIUnpackArgumentsByRef(argTypes, input, outArgValues)
}

// Deprecated: use ABIUnpackArguments instead
func AbiDecoderWithReturnedValues(argTypes []string, input []byte) ([]interface{}, error) {
	return ABIUnpackArguments(argTypes, input)
}

// Deprecated: use ABIUnpack instead
func AbiDecodeExpr(expr string, input []byte, argValues []interface{}) error {
	return ABIUnpack(expr, input, argValues)
}

// Deprecated: use ABIUnpackAndStringify instead
func AbiDecodeExprAndStringify(expr string, input []byte) ([]string, error) {
	return ABIUnpackAndStringify(expr, input)
}

// Deprecated: use ABIMarshalStringValues instead
func AbiMarshalStringValues(argTypes []string, input []byte) ([]string, error) {
	return ABIMarshalStringValues(argTypes, input)
}

// AbiUnmarshalStringValues will take an array of ethereum types as string values, and decode
// the string values to runtime objects. This allows simple string value input from an app
// or user, and converts them to the appropriate runtime objects.
//
// The common use for this method is to pass a JSON object of string values for an abi method
// and have it properly encode to the native abi types.
//
// Deprecated: use ABIUnmarshalStringValues instead
func AbiUnmarshalStringValues(argTypes []string, stringValues []string) ([]any, error) {
	return ABIUnmarshalStringValues(argTypes, stringValues)
}

// Deprecated: use ABIEncodeMethodCalldata instead
func AbiEncodeMethodCalldata(methodExpr string, argValues []interface{}) ([]byte, error) {
	return ABIEncodeMethodCalldata(methodExpr, argValues)
}

// Deprecated: use ABIEncodeMethodCalldataFromStringValuesAny instead
func AbiEncodeMethodCalldataFromStringValues(methodExpr string, argStringValues []string) ([]byte, error) {
	return ABIEncodeMethodCalldataFromStringValues(methodExpr, argStringValues)
}

// Deprecated: use ABIEncodeMethodCalldataFromStringValuesAny instead
func AbiEncodeMethodCalldataFromStringValuesAny(methodSig string, argStringValues []any) ([]byte, error) {
	return ABIEncodeMethodCalldataFromStringValuesAny(methodSig, argStringValues)
}
