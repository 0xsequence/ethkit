package ethcoder

import (
	"bytes"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
)

// EIP-712 -- https://eips.ethereum.org/EIPS/eip-712

type TypedData struct {
	Types       TypedDataTypes         `json:"types"`
	PrimaryType string                 `json:"primaryType"`
	Domain      TypedDataDomain        `json:"domain"`
	Message     map[string]interface{} `json:"message"`
}

type TypedDataTypes map[string][]TypedDataArgument

func (t TypedDataTypes) EncodeType(primaryType string) (string, error) {
	args, ok := t[primaryType]
	if !ok {
		return "", fmt.Errorf("%s type is not defined", primaryType)
	}

	// Collect all dependency types (transitively).
	depsSet := make(map[string]bool)
	var collectDeps func(string)
	collectDeps = func(typeName string) {
		for _, arg := range t[typeName] {
			// Remove array suffix if any.
			baseType := arg.Type
			if idx := strings.Index(baseType, "["); idx != -1 {
				baseType = baseType[:idx]
			}
			// If it's a custom type and not already seen, add and collect its dependencies.
			if _, exists := t[baseType]; exists && !depsSet[baseType] {
				depsSet[baseType] = true
				collectDeps(baseType)
			}
		}
	}
	collectDeps(primaryType)
	delete(depsSet, primaryType)

	// Sort the dependency types alphabetically.
	var deps []string
	for dep := range depsSet {
		deps = append(deps, dep)
	}
	sort.Strings(deps)

	// Build the primary type definition.
	s := primaryType + "("
	for i, arg := range args {
		s += arg.Type + " " + arg.Name
		if i < len(args)-1 {
			s += ","
		}
	}
	s += ")"

	for _, dep := range deps {
		def, err := t.encodeTypeDefinition(dep)
		if err != nil {
			return "", err
		}
		s += def
	}

	return s, nil
}

// encodeTypeDefinition encodes the definition for a single type without processing nested dependencies.
func (t TypedDataTypes) encodeTypeDefinition(typeName string) (string, error) {
	args, ok := t[typeName]
	if !ok {
		return "", fmt.Errorf("%s type is not defined", typeName)
	}
	def := typeName + "("
	for i, arg := range args {
		def += arg.Type + " " + arg.Name
		if i < len(args)-1 {
			def += ","
		}
	}
	def += ")"
	return def, nil
}

func (t TypedDataTypes) Map() map[string]map[string]string {
	out := map[string]map[string]string{}
	for k, v := range t {
		m := make(map[string]string, len(v))
		for _, arg := range v {
			m[arg.Name] = arg.Type
		}
		out[k] = m
	}
	return out
}

func (t TypedDataTypes) TypeHash(primaryType string) ([]byte, error) {
	encodeType, err := t.EncodeType(primaryType)
	if err != nil {
		return nil, err
	}
	return Keccak256([]byte(encodeType)), nil
}

type TypedDataArgument struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type TypedDataDomain struct {
	Name              string          `json:"name,omitempty"`
	Version           string          `json:"version,omitempty"`
	ChainID           *big.Int        `json:"chainId,omitempty"`
	VerifyingContract *common.Address `json:"verifyingContract,omitempty"`
	Salt              *common.Hash    `json:"salt,omitempty"`
}

func (t TypedDataDomain) Map() map[string]interface{} {
	m := map[string]interface{}{}
	if t.Name != "" {
		m["name"] = t.Name
	}
	if t.Version != "" {
		m["version"] = t.Version
	}
	if t.ChainID != nil {
		m["chainId"] = t.ChainID
	}
	if t.VerifyingContract != nil {
		m["verifyingContract"] = *t.VerifyingContract
	}
	if t.Salt != nil {
		m["salt"] = *t.Salt
	}
	return m
}

func (t *TypedData) HashStruct(primaryType string, data map[string]interface{}) ([]byte, error) {
	typeHash, err := t.Types.TypeHash(primaryType)
	if err != nil {
		return nil, err
	}
	encodedData, err := t.encodeData(primaryType, data)
	if err != nil {
		return nil, err
	}
	return Keccak256(append(typeHash, encodedData...)), nil
}

func (t *TypedData) encodeData(primaryType string, data map[string]interface{}) ([]byte, error) {
	args, ok := t.Types[primaryType]
	if !ok {
		return nil, fmt.Errorf("%s type is unknown", primaryType)
	}
	if len(args) != len(data) {
		return nil, fmt.Errorf("encoding failed for type %s, expecting %d arguments but received %d data values", primaryType, len(args), len(data))
	}

	var encoded []byte

	// Encode each field
	for _, arg := range args {
		dataValue, ok := data[arg.Name]
		if !ok {
			return nil, fmt.Errorf("data value missing for type %s with argument name %s", primaryType, arg.Name)
		}

		encValue, err := t.encodeValue(arg.Type, dataValue)
		if err != nil {
			return nil, fmt.Errorf("failed to encode %s: %w", arg.Name, err)
		}
		// Ensure each encoded value is exactly 32 bytes
		if len(encValue) != 32 {
			return nil, fmt.Errorf("encoded value for %s is %d bytes, expected 32", arg.Name, len(encValue))
		}
		encoded = append(encoded, encValue...)
	}

	return encoded, nil
}

// encodeValue handles the recursive encoding of values according to their types
func (t *TypedData) encodeValue(typ string, value interface{}) ([]byte, error) {
	// Handle arrays
	if strings.Index(typ, "[") > 0 {
		baseType := typ[:strings.Index(typ, "[")]
		var values []interface{}
		v := reflect.ValueOf(value)

		switch v.Kind() {
		case reflect.Array, reflect.Slice:
			values = make([]interface{}, v.Len())
			for i := 0; i < v.Len(); i++ {
				values[i] = v.Index(i).Interface()
			}
		default:
			return nil, fmt.Errorf("expected array or slice for type %s, got %T", typ, value)
		}

		// Encode each element and ensure it's 32 bytes
		encodedValues := make([][]byte, len(values))
		for i, val := range values {
			encoded, err := t.encodeValue(baseType, val)
			if err != nil {
				return nil, fmt.Errorf("failed to encode array element %d: %w", i, err)
			}

			if baseType == "string" || baseType == "bytes" {
				if len(encoded) != 32 {
					encoded = Keccak256(encoded) // explicitly hash if not already 32 bytes
				}
			}

			encodedValues[i] = encoded
		}

		// Concatenate hashes and then hash the result
		concat := bytes.Join(encodedValues, nil)
		return Keccak256(concat), nil
	}

	// Handle custom struct types
	if _, isCustomType := t.Types[typ]; isCustomType {
		mapVal, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid value for custom type %s", typ)
		}
		return t.HashStruct(typ, mapVal)
	}

	// Handle primitive types
	switch typ {
	case "string":
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid string value")
		}
		return Keccak256([]byte(str)), nil
	case "bytes", "bytes32":
		var b []byte
		switch v := value.(type) {
		case []byte:
			b = v
		case string:
			if strings.HasPrefix(v, "0x") {
				b = common.FromHex(v)
			} else {
				b = []byte(v)
			}
		case common.Hash:
			b = v.Bytes()
		default:
			return nil, fmt.Errorf("invalid bytes value type: %T", value)
		}
		if typ == "bytes32" {
			if len(b) != 32 {
				return nil, fmt.Errorf("invalid bytes32 length: got %d, want 32", len(b))
			}
			return b, nil
		}
		return Keccak256(b), nil
	case "address":
		switch v := value.(type) {
		case common.Address:
			return common.LeftPadBytes(v.Bytes(), 32), nil
		case string:
			if !common.IsHexAddress(v) {
				return nil, fmt.Errorf("invalid address format: %s", v)
			}
			addr := common.HexToAddress(v)
			return common.LeftPadBytes(addr.Bytes(), 32), nil
		default:
			return nil, fmt.Errorf("invalid address value")
		}
	case "bool":
		if b, ok := value.(bool); ok {
			var val []byte
			if b {
				val = []byte{1}
			} else {
				val = []byte{0}
			}
			return common.LeftPadBytes(val, 32), nil
		}
		return nil, fmt.Errorf("invalid bool value")
	}

	// Handle uint/int types
	if strings.HasPrefix(typ, "uint") || strings.HasPrefix(typ, "int") {
		var n *big.Int
		switch v := value.(type) {
		case *big.Int:
			n = v
		case string:
			if strings.HasPrefix(v, "0x") {
				n = new(big.Int).SetBytes(common.FromHex(v))
			} else {
				var ok bool
				n, ok = new(big.Int).SetString(v, 10)
				if !ok {
					return nil, fmt.Errorf("invalid number string: %s", v)
				}
			}
		case int64:
			n = big.NewInt(v)
		case uint64:
			n = new(big.Int).SetUint64(v)
		case int:
			n = big.NewInt(int64(v))
		case uint:
			n = new(big.Int).SetUint64(uint64(v))
		case uint8:
			n = big.NewInt(int64(v))
		case int8:
			n = big.NewInt(int64(v))
		case uint16:
			n = big.NewInt(int64(v))
		case int16:
			n = big.NewInt(int64(v))
		case uint32:
			n = big.NewInt(int64(v))
		case int32:
			n = big.NewInt(int64(v))
		default:
			return nil, fmt.Errorf("invalid number value type: %T", value)
		}

		// Check if it's a negative number for int types
		if strings.HasPrefix(typ, "int") && n.Sign() < 0 {
			return PadZerosSigned(n.Bytes(), 32), nil
		}
		return common.LeftPadBytes(n.Bytes(), 32), nil
	}

	// Handle fixed-size bytes
	if strings.HasPrefix(typ, "bytes") {
		var b []byte
		switch v := value.(type) {
		case []byte:
			b = v
		case string:
			if strings.HasPrefix(v, "0x") {
				b = common.FromHex(v)
			} else {
				b = []byte(v)
			}
		case common.Hash:
			b = v.Bytes()
		default:
			return nil, fmt.Errorf("invalid bytes value type: %T", value)
		}

		size := 0
		if len(typ) > 5 {
			var err error
			size, err = strconv.Atoi(typ[5:])
			if err != nil {
				return nil, fmt.Errorf("invalid bytes size: %w", err)
			}
		}
		if size > 0 && len(b) != size {
			return nil, fmt.Errorf("invalid bytes length for %s: got %d, want %d", typ, len(b), size)
		}
		return common.RightPadBytes(b, 32), nil
	}

	return nil, fmt.Errorf("unsupported type: %s", typ)
}

// Encode returns the digest of the typed data and the fully encoded EIP712 typed data message.
//
// NOTE:
// * the digest is the hash of the fully encoded EIP712 message
// * the encoded message is the fully encoded EIP712 message (0x1901 + domain + hashStruct(message))
func (t *TypedData) Encode() ([]byte, []byte, error) {
	EIP191_HEADER := "0x1901" // EIP191 for typed data
	eip191Header, err := HexDecode(EIP191_HEADER)
	if err != nil {
		return nil, nil, err
	}

	// Prepare hash struct for the domain
	domainHash, err := t.HashStruct("EIP712Domain", t.Domain.Map())
	if err != nil {
		return nil, nil, err
	}

	// Prepare hash struct for the message object
	var messageHash []byte
	if t.PrimaryType == "EIP712Domain" {
		messageHash = domainHash
	} else {
		messageHash, err = t.HashStruct(t.PrimaryType, t.Message)
		if err != nil {
			return nil, nil, err
		}
	}

	encodedMessage := append(eip191Header, append(domainHash, messageHash...)...)
	digest := crypto.Keccak256(encodedMessage)

	return digest, encodedMessage, nil
}

// EncodeDigest returns the digest of the typed data message.
func (t *TypedData) EncodeDigest() ([]byte, error) {
	digest, _, err := t.Encode()
	if err != nil {
		return nil, err
	}
	return digest, nil
}

// PadZerosSigned pads a byte slice with 1s (for negative numbers) or 0s (for positive numbers) to the specified length
func PadZerosSigned(b []byte, length int) []byte {
	if len(b) >= length {
		return b[:length]
	}
	padded := make([]byte, length)
	if len(b) > 0 && (b[0]&0x80) != 0 { // Check if the number is negative
		for i := 0; i < length-len(b); i++ {
			padded[i] = 0xff // Pad with 1s for negative numbers
		}
	}
	copy(padded[length-len(b):], b)
	return padded
}
