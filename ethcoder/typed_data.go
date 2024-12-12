package ethcoder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

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

func TypedDataFromJSON(typedDataJSON string) (*TypedData, error) {
	var typedData TypedData
	err := json.Unmarshal([]byte(typedDataJSON), &typedData)
	if err != nil {
		return nil, err
	}
	return &typedData, nil
}

func (t *TypedData) UnmarshalJSON(data []byte) error {
	// Create an intermediate structure using json.Number
	type TypedDataRaw struct {
		Types       TypedDataTypes         `json:"types"`
		PrimaryType string                 `json:"primaryType"`
		Domain      TypedDataDomain        `json:"domain"`
		Message     map[string]interface{} `json:"message"`
	}

	// Create a decoder that will preserve number strings
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	// First unmarshal into the intermediate structure
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

	// ..
	primaryDomainType, ok := raw.Types[raw.PrimaryType]
	if !ok {
		return fmt.Errorf("primary type %s is not defined", raw.PrimaryType)
	}
	primaryDomainTypeMap := typedDataTypeMap(primaryDomainType)
	fmt.Println("===> primaryDomainType", primaryDomainTypeMap)

	// Process the Message map to convert values to desired types
	processedMessage := make(map[string]interface{})
	for k, v := range raw.Message {
		fmt.Println("===> k", k, "v", v)

		typ, ok := primaryDomainTypeMap[k]
		if !ok {
			return fmt.Errorf("type %s is not defined", k)
		}
		fmt.Println("===> typ", k, typ)

		// TODO: its possible that the type is a struct, and we need to do another call to get the typedData map, etc

		switch val := v.(type) {
		case json.Number:
			// TODO: we will check the domain, etc.........

			if typ == "uint8" {
				num, err := val.Int64()
				if err != nil {
					return fmt.Errorf("failed to parse uint8 value %s, because %w", val, err)
				}
				// TODO: is this okay ... int64 to uint8 ..???...
				processedMessage[k] = uint8(num)
			} else {
				// Try parsing as big.Int first
				if n, ok := new(big.Int).SetString(string(val), 10); ok {
					processedMessage[k] = n
				} else {
					// If it's not a valid integer, keep the original value
					processedMessage[k] = v
				}
			}

		case string:
			if typ == "address" {
				addr := common.HexToAddress(val)
				processedMessage[k] = addr
			} else if len(val) > 2 && (val[:2] == "0x" || val[:2] == "0X") {
				// Convert hex strings to *big.Int
				n := new(big.Int)
				n.SetString(val[2:], 16)
				processedMessage[k] = n
			} else {
				processedMessage[k] = val
			}

		default:
			// TODO: prob needs to be recursive.. cuz might be some array or object ..
			return fmt.Errorf("unsupported type %T for value %v", v, v)
		}
	}

	t.Types = raw.Types
	t.PrimaryType = raw.PrimaryType
	t.Domain = raw.Domain
	t.Message = processedMessage

	return nil
}

func typedDataTypeMap(typ []TypedDataArgument) map[string]string {
	m := map[string]string{}
	for _, arg := range typ {
		m[arg.Name] = arg.Type
	}
	return m
}
