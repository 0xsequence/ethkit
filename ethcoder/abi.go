package ethcoder

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
)

func AbiCoder(argTypes []string, argValues []interface{}) ([]byte, error) {
	if len(argTypes) != len(argValues) {
		return nil, errors.New("invalid arguments - types and values do not match")
	}
	args, err := buildArgumentsFromTypes(argTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to build abi: %v", err)
	}
	return args.Pack(argValues...)
}

func AbiCoderHex(argTypes []string, argValues []interface{}) (string, error) {
	b, err := AbiCoder(argTypes, argValues)
	if err != nil {
		return "", err
	}
	h := hexutil.Encode(b)
	return h, nil
}

func AbiDecoder(argTypes []string, input []byte, argValues []interface{}) error {
	if len(argTypes) != len(argValues) {
		return errors.New("invalid arguments - types and values do not match")
	}
	args, err := buildArgumentsFromTypes(argTypes)
	if err != nil {
		return fmt.Errorf("failed to build abi: %v", err)
	}
	values, err := args.Unpack(input)
	if err != nil {
		return err
	}
	if len(args) > 1 {
		return args.Copy(&argValues, values)
	} else {
		return args.Copy(&argValues[0], values)
	}
}

func AbiDecoderWithReturnedValues(argTypes []string, input []byte) ([]interface{}, error) {
	args, err := buildArgumentsFromTypes(argTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to build abi: %v", err)
	}
	return args.UnpackValues(input)
}

func AbiEncodeMethodCalldata(methodExpr string, argValues []interface{}) ([]byte, error) {
	mabi, methodName, err := ParseMethodABI(methodExpr, "")
	if err != nil {
		return nil, err
	}
	data, err := mabi.Pack(methodName, argValues...)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func AbiEncodeMethodCalldataFromStringValues(methodExpr string, argStringValues []string) ([]byte, error) {
	_, argsList, err := parseMethodExpr(methodExpr)
	if err != nil {
		return nil, err
	}
	argTypes := []string{}
	for _, v := range argsList {
		argTypes = append(argTypes, v.Type)
	}

	argValues, err := AbiUnmarshalStringValues(argTypes, argStringValues)
	if err != nil {
		return nil, err
	}
	return AbiEncodeMethodCalldata(methodExpr, argValues)
}

func AbiDecodeExpr(expr string, input []byte, argValues []interface{}) error {
	argsList := parseArgumentExpr(expr)
	argTypes := []string{}
	for _, v := range argsList {
		argTypes = append(argTypes, v.Type)
	}
	return AbiDecoder(argTypes, input, argValues)
}

func AbiDecodeExprAndStringify(expr string, input []byte) ([]string, error) {
	argsList := parseArgumentExpr(expr)
	argTypes := []string{}
	for _, v := range argsList {
		argTypes = append(argTypes, v.Type)
	}

	return AbiMarshalStringValues(argTypes, input)
}

func AbiMarshalStringValues(argTypes []string, input []byte) ([]string, error) {
	values, err := AbiDecoderWithReturnedValues(argTypes, input)
	if err != nil {
		return nil, err
	}
	return StringifyValues(values)
}

// AbiDecodeStringValues will take an array of ethereum types and string values, and decode
// the string values to runtime objects.
func AbiUnmarshalStringValues(argTypes []string, stringValues []string) ([]interface{}, error) {
	if len(argTypes) != len(stringValues) {
		return nil, fmt.Errorf("ethcoder: argTypes and stringValues must be of equal length")
	}

	values := []interface{}{}

	for i, typ := range argTypes {
		s := stringValues[i]

		switch typ {
		case "address":
			// expected "0xabcde......"
			if s[0:2] != "0x" {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting address in hex", i)
			}
			values = append(values, common.HexToAddress(s))
			continue

		case "string":
			// expected: string value
			values = append(values, s)
			continue

		case "bytes":
			// expected: bytes in hex encoding with 0x prefix
			if s[0:2] != "0x" {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting bytes in hex", i)
			}
			values = append(values, common.Hex2Bytes(s[2:]))
			continue

		case "bool":
			// expected: "true" | "false"
			if s == "true" {
				values = append(values, true)
			} else if s == "false" {
				values = append(values, false)
			} else {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting bool as 'true' or 'false'", i)
			}
			continue
		}

		// numbers
		if match := regexArgNumber.FindStringSubmatch(typ); len(match) > 0 {
			size, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting %s. reason: %w", i, typ, err)
			}
			if (size%8 != 0) || size == 0 || size > 256 {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. invalid number type '%s'", i, typ)
			}

			num := big.NewInt(0)
			num, ok := num.SetString(s, 10)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting number. unable to set value of '%s'", i, s)
			}
			values = append(values, num)
			continue
		}

		// bytesXX (fixed)
		if match := regexArgBytes.FindStringSubmatch(typ); len(match) > 0 {
			if s[0:2] != "0x" {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting bytes in hex", i)
			}
			size, err := strconv.ParseInt(match[1], 10, 64)
			if err != nil {
				return nil, err
			}
			if size == 0 || size > 32 {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. bytes type '%s' is invalid", i, typ)
			}
			val := common.Hex2Bytes(s[2:])
			if int64(len(val)) != size {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. %s type expects a %d byte value but received %d", i, typ, size, len(val))
			}
			values = append(values, val)
			continue
		}

		// arrays
		if match := regexArgArray.FindStringSubmatch(typ); len(match) > 0 {
			baseTyp := match[1]
			if match[2] == "" {
				match[2] = "0"
			}
			count, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				return nil, err
			}

			submatch := regexArgNumber.FindStringSubmatch(baseTyp)
			if len(submatch) == 0 {
				return nil, fmt.Errorf("ethcoder: value at position %d of type %s is unsupported. Only number string arrays are presently supported.", i, typ)
			}

			var stringValues []string
			err = json.Unmarshal([]byte(s), &stringValues)
			if err != nil {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. failed to unmarshal json string array '%s'", i, s)
			}
			if count > 0 && len(stringValues) != int(count) {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. array size does not match required size of %d", i, count)
			}

			var arrayArgs []string
			for i := 0; i < len(stringValues); i++ {
				arrayArgs = append(arrayArgs, baseTyp)
			}

			arrayValues, err := AbiUnmarshalStringValues(arrayArgs, stringValues)
			if err != nil {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. failed to get string values for array - %w", i, err)
			}

			var bnArray []*big.Int
			for _, n := range arrayValues {
				bn, ok := n.(*big.Int)
				if !ok {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting array element to be *big.Int", i)
				}
				bnArray = append(bnArray, bn)
			}
			values = append(values, bnArray)
		}
	}

	return values, nil
}

// ParseMethodABI will return an `abi.ABI` object from the short-hand method string expression,
// for example, methodExpr: `balanceOf(address)` returnsExpr: `uint256`
func ParseMethodABI(methodExpr, returnsExpr string) (*abi.ABI, string, error) {
	var methodName string
	var inputArgs, outputArgs []abiArgument
	var err error

	methodName, inputArgs, err = parseMethodExpr(methodExpr)
	if err != nil {
		return nil, "", err
	}

	if returnsExpr != "" {
		outputArgs = parseArgumentExpr(returnsExpr)
	}

	// generate method abi json for parsing
	methodABI := abiJSON{
		Name:    methodName,
		Type:    "function",
		Inputs:  inputArgs,
		Outputs: outputArgs,
	}

	abiJSON, err := json.Marshal(methodABI)
	if err != nil {
		return nil, methodName, err
	}

	mabi, err := abi.JSON(strings.NewReader(fmt.Sprintf("[%s]", string(abiJSON))))
	if err != nil {
		return nil, methodName, err
	}

	return &mabi, methodName, nil
}

func buildArgumentsFromTypes(argTypes []string) (abi.Arguments, error) {
	args := abi.Arguments{}
	for _, argType := range argTypes {
		abiType, err := abi.NewType(argType, "", nil)
		if err != nil {
			return nil, err
		}
		args = append(args, abi.Argument{Type: abiType})
	}
	return args, nil
}

func parseMethodExpr(expr string) (string, []abiArgument, error) {
	expr = strings.Trim(expr, " ")
	idx := strings.Index(expr, "(")
	if idx < 0 {
		return "", nil, errors.New("ethcoder: invalid input expr. expected format is: methodName(arg1Type, arg2Type)")
	}
	methodName := expr[0:idx]
	expr = expr[idx:]
	if expr[0] != '(' || expr[len(expr)-1] != ')' {
		return "", nil, errors.New("ethcoder: invalid input expr. expected format is: methodName(arg1Type, arg2Type)")
	}
	argsList := parseArgumentExpr(expr)
	return methodName, argsList, nil
}

func parseArgumentExpr(expr string) []abiArgument {
	args := []abiArgument{}
	expr = strings.Trim(expr, "() ")
	p := strings.Split(expr, ",")

	if expr == "" {
		return args
	}
	for _, v := range p {
		v = strings.Trim(v, " ")
		n := strings.Split(v, " ")
		arg := abiArgument{Type: n[0]}
		if len(n) > 1 {
			arg.Name = n[1]
		}
		args = append(args, arg)
	}
	return args
}

type abiJSON struct {
	Name    string        `json:"name"`
	Inputs  []abiArgument `json:"inputs"`
	Outputs []abiArgument `json:"outputs"`
	Type    string        `json:"type"`
}

type abiArgument struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
}
