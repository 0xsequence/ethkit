package ethcoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/common"
)

func TypedDataFromJSON(typedDataJSON string) (*TypedData, error) {
	var typedData TypedData
	err := json.Unmarshal([]byte(typedDataJSON), &typedData)
	if err != nil {
		return nil, err
	}
	return &typedData, nil
}

func (t *TypedData) MarshalJSON() ([]byte, error) {
	type TypedDataJSON struct {
		Types       TypedDataTypes         `json:"types"`
		PrimaryType string                 `json:"primaryType"`
		Domain      TypedDataDomain        `json:"domain"`
		Message     map[string]interface{} `json:"message"`
	}

	encodedMessage, err := t.jsonEncodeMessageValues(t.PrimaryType, t.Message)
	if err != nil {
		return nil, err
	}

	return json.Marshal(TypedDataJSON{
		Types:       t.Types,
		PrimaryType: t.PrimaryType,
		Domain:      t.Domain,
		Message:     encodedMessage,
	})
}

func (t *TypedData) jsonEncodeMessageValues(typeName string, message map[string]interface{}) (map[string]interface{}, error) {
	typeFields, ok := t.Types[typeName]
	if !ok {
		return nil, fmt.Errorf("type '%s' not found in types", typeName)
	}

	encodedMessage := make(map[string]interface{})

	for _, field := range typeFields {
		val, exists := message[field.Name]
		if !exists {
			continue
		}

		// Handle arrays
		if strings.HasSuffix(field.Type, "[]") {
			baseType := field.Type[:len(field.Type)-2]
			if arr, ok := val.([]interface{}); ok {
				encodedArr := make([]interface{}, len(arr))
				for i, item := range arr {
					encoded, err := t.jsonEncodeValue(baseType, item)
					if err != nil {
						return nil, err
					}
					encodedArr[i] = encoded
				}
				encodedMessage[field.Name] = encodedArr
				continue
			}
		}

		// Handle single values
		encoded, err := t.jsonEncodeValue(field.Type, val)
		if err != nil {
			return nil, err
		}
		encodedMessage[field.Name] = encoded
	}

	return encodedMessage, nil
}

func (t *TypedData) jsonEncodeValue(fieldType string, value interface{}) (interface{}, error) {
	// Handle bytes/bytes32
	if strings.HasPrefix(fieldType, "bytes") {
		switch v := value.(type) {
		case []byte:
			return "0x" + common.Bytes2Hex(v), nil
		case [8]byte:
			return "0x" + common.Bytes2Hex(v[:]), nil
		case [16]byte:
			return "0x" + common.Bytes2Hex(v[:]), nil
		case [24]byte:
			return "0x" + common.Bytes2Hex(v[:]), nil
		case [32]byte:
			return "0x" + common.Bytes2Hex(v[:]), nil
		}
		return value, nil
	}

	// Handle nested custom types
	if _, isCustomType := t.Types[fieldType]; isCustomType {
		if nestedMsg, ok := value.(map[string]interface{}); ok {
			return t.jsonEncodeMessageValues(fieldType, nestedMsg)
		}
		return nil, fmt.Errorf("value for custom type '%s' is not a map", fieldType)
	}

	// Return primitive values as-is
	return value, nil
}

func (t *TypedData) UnmarshalJSON(data []byte) error {
	// Intermediary structure to decode message field
	type TypedDataRaw struct {
		Types       TypedDataTypes `json:"types"`
		PrimaryType string         `json:"primaryType"`
		Domain      struct {
			Name              string          `json:"name,omitempty"`
			Version           string          `json:"version,omitempty"`
			ChainID           interface{}     `json:"chainId,omitempty"`
			VerifyingContract *common.Address `json:"verifyingContract,omitempty"`
			Salt              *common.Hash    `json:"salt,omitempty"`
		} `json:"domain"`
		Message map[string]interface{} `json:"message"`
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
		// detect primary type if its unspecified
		primaryType, err := typedDataDetectPrimaryType(raw.Types.Map(), raw.Message)
		if err != nil {
			return err
		}
		raw.PrimaryType = primaryType
	}
	_, ok = raw.Types[raw.PrimaryType]
	if !ok {
		return fmt.Errorf("primary type '%s' is not defined", raw.PrimaryType)
	}

	// Decode the domain, which is mostly decooded except the chainId is an interface{} type
	// because the value may be a number of a hex encoded number. We want it in a big.Int.
	domain := TypedDataDomain{
		Name:              raw.Domain.Name,
		Version:           raw.Domain.Version,
		ChainID:           nil,
		VerifyingContract: raw.Domain.VerifyingContract,
		Salt:              raw.Domain.Salt,
	}
	if raw.Domain.ChainID != nil {
		chainID := big.NewInt(0)
		if val, ok := raw.Domain.ChainID.(float64); ok {
			chainID.SetInt64(int64(val))
		} else if val, ok := raw.Domain.ChainID.(json.Number); ok {
			chainID.SetString(val.String(), 10)
		} else if val, ok := raw.Domain.ChainID.(string); ok {
			if strings.HasPrefix(val, "0x") {
				chainID.SetString(val[2:], 16)
			} else {
				chainID.SetString(val, 10)
			}
		}
		domain.ChainID = chainID
	}

	// Decode the raw message into Go runtime types
	message, err := typedDataDecodeRawMessageMap(raw.Types.Map(), raw.PrimaryType, raw.Message)
	if err != nil {
		return err
	}

	t.Types = raw.Types
	t.PrimaryType = raw.PrimaryType
	t.Domain = domain

	m, ok := message.(map[string]interface{})
	if !ok {
		return fmt.Errorf("resulting message is not a map")
	}
	t.Message = m

	return nil
}

func typedDataDetectPrimaryType(typesMap map[string]map[string]string, message map[string]interface{}) (string, error) {
	// If there are only two types, and one is the EIP712Domain, then the other is the primary type
	if len(typesMap) == 2 {
		_, ok := typesMap["EIP712Domain"]
		if ok {
			for typ := range typesMap {
				if typ == "EIP712Domain" {
					continue
				}
				return typ, nil
			}
		}
	}

	// Otherwise search for the primary type by looking for the first type that has a message field keys
	messageKeys := []string{}
	for k := range message {
		messageKeys = append(messageKeys, k)
	}
	sort.Strings(messageKeys)

	for typ := range typesMap {
		if typ == "EIP712Domain" {
			continue
		}
		if len(typesMap[typ]) != len(messageKeys) {
			continue
		}

		typKeys := []string{}
		for k := range typesMap[typ] {
			typKeys = append(typKeys, k)
		}
		sort.Strings(typKeys)

		if !slices.Equal(messageKeys, typKeys) {
			continue
		}
		return typ, nil
	}

	return "", fmt.Errorf("no primary type found")
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
		return nil, fmt.Errorf("typedDataDecodePrimitiveValue: %w", err)
	}
	return out[0], nil
}
