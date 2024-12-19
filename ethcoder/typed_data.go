package ethcoder

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"
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

	subTypes := []string{}
	s := primaryType + "("

	for i, arg := range args {
		baseType := arg.Type
		if strings.Index(baseType, "[") > 0 {
			baseType = baseType[:strings.Index(baseType, "[")]
		}

		if _, ok := t[baseType]; ok {
			set := false
			for _, v := range subTypes {
				if v == baseType {
					set = true
					break
				}
			}
			if !set {
				subTypes = append(subTypes, baseType)
			}
		}

		s += arg.Type + " " + arg.Name
		if i < len(args)-1 {
			s += ","
		}
	}
	s += ")"

	sort.Strings(subTypes)
	for _, subType := range subTypes {
		subEncodeType, err := t.EncodeType(subType)
		if err != nil {
			return "", err
		}
		s += subEncodeType
	}

	return s, nil
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
	if t.VerifyingContract != nil && t.VerifyingContract.String() != "0x0000000000000000000000000000000000000000" {
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
	v, err := SolidityPack([]string{"bytes32", "bytes"}, []interface{}{BytesToBytes32(typeHash), encodedData})
	if err != nil {
		return nil, err
	}
	return Keccak256(v), nil
}

func (t *TypedData) encodeData(primaryType string, data map[string]interface{}) ([]byte, error) {
	args, ok := t.Types[primaryType]
	if !ok {
		return nil, fmt.Errorf("%s type is unknown", primaryType)
	}
	if len(args) != len(data) {
		return nil, fmt.Errorf("encoding failed for type %s, expecting %d arguments but received %d data values", primaryType, len(args), len(data))
	}

	encodedTypes := make([]string, len(args))
	encodedValues := make([]interface{}, len(args))

	for i, arg := range args {
		dataValue, ok := data[arg.Name]
		if !ok {
			return nil, fmt.Errorf("data value missing for type %s with argument name %s", primaryType, arg.Name)
		}

		encValue, err := t.encodeValue(arg.Type, dataValue)
		if err != nil {
			return nil, fmt.Errorf("failed to encode %s: %w", arg.Name, err)
		}
		encodedTypes[i] = "bytes"
		encodedValues[i] = encValue
	}

	return SolidityPack(encodedTypes, encodedValues)
}

// encodeValue handles the recursive encoding of values according to their types
func (t *TypedData) encodeValue(typ string, value interface{}) ([]byte, error) {
	// Handle arrays
	if strings.Index(typ, "[") > 0 {
		baseType := typ[:strings.Index(typ, "[")]
		values, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected array for type %s", typ)
		}

		encodedValues := make([][]byte, len(values))
		for i, val := range values {
			encoded, err := t.encodeValue(baseType, val)
			if err != nil {
				return nil, fmt.Errorf("failed to encode array element %d: %w", i, err)
			}
			encodedValues[i] = encoded
		}

		// For arrays, we concatenate the encoded values and hash the result
		concat := bytes.Join(encodedValues, nil)
		return Keccak256(concat), nil
	}

	// Handle bytes and string
	if typ == "bytes" || typ == "string" {
		var bytesValue []byte
		if v, ok := value.([]byte); ok {
			bytesValue = v
		} else if v, ok := value.(string); ok {
			bytesValue = []byte(v)
		} else {
			return nil, fmt.Errorf("invalid value for type %s", typ)
		}
		return Keccak256(bytesValue), nil
	}

	// Handle custom struct types
	if _, isCustomType := t.Types[typ]; isCustomType {
		mapVal, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid value for custom type %s", typ)
		}
		encoded, err := t.HashStruct(typ, mapVal)
		if err != nil {
			return nil, fmt.Errorf("failed to encode custom type %s: %w", typ, err)
		}
		return PadZeros(encoded, 32)
	}

	// Handle primitive types
	packed, err := SolidityPack([]string{typ}, []interface{}{value})
	if err != nil {
		return nil, err
	}
	return PadZeros(packed, 32)
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
	messageHash, err := t.HashStruct(t.PrimaryType, t.Message)
	if err != nil {
		return nil, nil, err
	}

	encodedMessage, err := SolidityPack([]string{"bytes", "bytes32", "bytes32"}, []interface{}{eip191Header, domainHash, messageHash})
	if err != nil {
		return nil, nil, err
	}

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
