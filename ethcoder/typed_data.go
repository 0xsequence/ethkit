package ethcoder

import (
	"bytes"
	"encoding/json"
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
		_, ok := t[arg.Type]
		if ok {
			set := false
			for _, v := range subTypes {
				if v == arg.Type {
					set = true
				}
			}
			if !set {
				subTypes = append(subTypes, arg.Type)
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
	Salt              *[32]byte       `json:"salt,omitempty"`
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
		panic(err)
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

	abiTypes := []string{}
	abiValues := []interface{}{}

	for _, arg := range args {
		dataValue, ok := data[arg.Name]
		if !ok {
			return nil, fmt.Errorf("data value missing for type %s with argument name %s", primaryType, arg.Name)
		}

		switch arg.Type {
		case "bytes", "string":
			var bytesValue []byte
			if v, ok := dataValue.([]byte); ok {
				bytesValue = v
			} else if v, ok := dataValue.(string); ok {
				bytesValue = []byte(v)
			} else {
				return nil, fmt.Errorf("data value invalid for type %s with argument name %s", primaryType, arg.Name)
			}
			abiTypes = append(abiTypes, "bytes32")
			abiValues = append(abiValues, BytesToBytes32(Keccak256(bytesValue)))

		default:
			dataValueString, isString := dataValue.(string)
			if isString {
				v, err := ABIUnmarshalStringValues([]string{arg.Type}, []string{dataValueString})
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal string value for type %s with argument name %s, because %w", primaryType, arg.Name, err)
				}
				abiValues = append(abiValues, v[0])
			} else {
				abiValues = append(abiValues, dataValue)
			}
			abiTypes = append(abiTypes, arg.Type)
		}
	}

	if len(args) != len(abiTypes) || len(args) != len(abiValues) {
		return nil, fmt.Errorf("argument encoding failed to encode all values")
	}

	// NOTE: each part must be bytes32
	var err error
	encodedTypes := make([]string, len(args))
	encodedValues := make([]interface{}, len(args))
	for i := 0; i < len(args); i++ {
		pack, err := SolidityPack([]string{abiTypes[i]}, []interface{}{abiValues[i]})
		if err != nil {
			return nil, err
		}
		encodedValues[i], err = PadZeros(pack, 32)
		if err != nil {
			return nil, err
		}
		encodedTypes[i] = "bytes"
	}

	encodedData, err := SolidityPack(encodedTypes, encodedValues)
	if err != nil {
		return nil, err
	}
	return encodedData, nil
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

func TypedDataFromJSON(typedDataJSON string) (*TypedData, error) {
	var typedData TypedData
	err := json.Unmarshal([]byte(typedDataJSON), &typedData)
	if err != nil {
		return nil, err
	}
	return &typedData, nil
}

func (t *TypedData) UnmarshalJSON(data []byte) error {
	// Intermediary structure to decode message field
	type TypedDataRaw struct {
		Types       TypedDataTypes         `json:"types"`
		PrimaryType string                 `json:"primaryType"`
		Domain      TypedDataDomain        `json:"domain"`
		Message     map[string]interface{} `json:"message"`
	}

	// Json decoder with json.Number support, so that we can decode big.Int values
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var raw TypedDataRaw
	if err := dec.Decode(&raw); err != nil {
		return err
	}

	// Ensure the "EIP712Domain" type is defined. In case its not defined
	// we will add it to the types map
	_, ok := raw.Types["EIP712Domain"]
	if !ok {
		raw.Types["EIP712Domain"] = []TypedDataArgument{}
		if raw.Domain.Name != "" {
			raw.Types["EIP712Domain"] = append(raw.Types["EIP712Domain"], TypedDataArgument{Name: "name", Type: "string"})
		}
		if raw.Domain.Version != "" {
			raw.Types["EIP712Domain"] = append(raw.Types["EIP712Domain"], TypedDataArgument{Name: "version", Type: "string"})
		}
		if raw.Domain.ChainID != nil {
			raw.Types["EIP712Domain"] = append(raw.Types["EIP712Domain"], TypedDataArgument{Name: "chainId", Type: "uint256"})
		}
		if raw.Domain.VerifyingContract != nil {
			raw.Types["EIP712Domain"] = append(raw.Types["EIP712Domain"], TypedDataArgument{Name: "verifyingContract", Type: "address"})
		}
		if raw.Domain.Salt != nil {
			raw.Types["EIP712Domain"] = append(raw.Types["EIP712Domain"], TypedDataArgument{Name: "salt", Type: "bytes32"})
		}
	}

	// Ensure primary type is defined
	if raw.PrimaryType == "" {
		return fmt.Errorf("primary type is required")
	}
	_, ok = raw.Types[raw.PrimaryType]
	if !ok {
		return fmt.Errorf("primary type '%s' is not defined", raw.PrimaryType)
	}

	// Decode the raw message into Go runtime types
	message, err := typedDataDecodeRawMessageMap(raw.Types.Map(), raw.PrimaryType, raw.Message)
	if err != nil {
		return err
	}

	t.Types = raw.Types
	t.PrimaryType = raw.PrimaryType
	t.Domain = raw.Domain

	m, ok := message.(map[string]interface{})
	if !ok {
		return fmt.Errorf("resulting message is not a map")
	}
	t.Message = m

	return nil
}

func typedDataDecodeRawMessageMap(typesMap map[string]map[string]string, primaryType string, data interface{}) (interface{}, error) {
	// Handle array types
	if arr, ok := data.([]interface{}); ok {
		results := make([]interface{}, len(arr))
		for i, item := range arr {
			decoded, err := typedDataDecodeRawMessageMap(typesMap, primaryType, item)
			if err != nil {
				return nil, err
			}
			results[i] = decoded
		}
		return results, nil
	}

	// Handle primitive directly
	message, ok := data.(map[string]interface{})
	if !ok {
		return typedDataDecodePrimitiveValue(primaryType, data)
	}

	currentType, ok := typesMap[primaryType]
	if !ok {
		return nil, fmt.Errorf("type %s is not defined", primaryType)
	}

	processedMessage := make(map[string]interface{})
	for k, v := range message {
		typ, ok := currentType[k]
		if !ok {
			return nil, fmt.Errorf("message field '%s' is missing type definition on '%s'", k, primaryType)
		}

		// Extract base type and check if it's an array
		baseType := typ
		isArray := false
		if idx := strings.Index(typ, "["); idx != -1 {
			baseType = typ[:idx]
			isArray = true
		}

		// Process value based on whether it's a custom or primitive type
		if _, isCustomType := typesMap[baseType]; isCustomType {
			decoded, err := typedDataDecodeRawMessageMap(typesMap, baseType, v)
			if err != nil {
				return nil, err
			}
			processedMessage[k] = decoded
		} else {
			var decoded interface{}
			var err error
			if isArray {
				decoded, err = typedDataDecodeRawMessageMap(typesMap, baseType, v)
			} else {
				decoded, err = typedDataDecodePrimitiveValue(baseType, v)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to decode field '%s': %w", k, err)
			}
			processedMessage[k] = decoded
		}
	}

	return processedMessage, nil
}

func typedDataDecodePrimitiveValue(typ string, value interface{}) (interface{}, error) {
	val := fmt.Sprintf("%v", value)
	out, err := ABIUnmarshalStringValuesAny([]string{typ}, []any{val})
	if err != nil {
		return nil, err
	}
	return out[0], nil
}
