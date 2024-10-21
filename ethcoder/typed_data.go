package ethcoder

import (
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

func (t *TypedData) EncodeDigest() ([]byte, error) {
	EIP191_HEADER := "0x1901"
	eip191Header, err := HexDecode(EIP191_HEADER)
	if err != nil {
		return nil, err
	}

	// Prepare hash struct for the domain
	domainHash, err := t.HashStruct("EIP712Domain", t.Domain.Map())
	if err != nil {
		return nil, err
	}

	// Prepare hash struct for the message object
	messageHash, err := t.HashStruct(t.PrimaryType, t.Message)
	if err != nil {
		return nil, err
	}

	hashPack, err := SolidityPack([]string{"bytes", "bytes32", "bytes32"}, []interface{}{eip191Header, domainHash, messageHash})
	if err != nil {
		return []byte{}, err
	}
	hashBytes := crypto.Keccak256(hashPack)

	return hashBytes, nil
}
