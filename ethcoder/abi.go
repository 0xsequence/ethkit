package ethcoder

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

func EncodeMethodCall(methodExpr string, args []interface{}) ([]byte, error) {
	mabi, methodName, err := ParseMethodABI(methodExpr, "")
	if err != nil {
		return nil, err
	}
	data, err := mabi.Pack(methodName, args...)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func DecodeAbiExpr(expr string, input []byte, argValues []interface{}) error {
	argsList := parseArgumentList(expr)
	argTypes := []string{}
	for _, v := range argsList {
		argTypes = append(argTypes, v.Type)
	}
	return AbiDecoder(argTypes, input, argValues)
}

func DecodeAbiExprAndStringify(expr string, input []byte) ([]string, error) {
	argsList := parseArgumentList(expr)
	argTypes := []string{}
	for _, v := range argsList {
		argTypes = append(argTypes, v.Type)
	}

	values, err := AbiDecodeValues(argTypes, input)
	if err != nil {
		return nil, err
	}

	return StringifyValues(values)
}

// ParseMethodABI will return an `abi.ABI` object from the short-hand method string expression,
// for example, methodExpr: `balanceOf(address)` returnsExpr: `uint256`
func ParseMethodABI(methodExpr, returnsExpr string) (*abi.ABI, string, error) {
	// parse short-hand representations
	methodExpr = strings.Trim(methodExpr, " ")
	idx := strings.Index(methodExpr, "(")
	if idx < 0 {
		return nil, "", errors.New("ethcoder: invalid input expr. expected format is: methodName(arg1Type, arg2Type)")
	}
	methodName := methodExpr[0:idx]
	methodExpr = methodExpr[idx:]
	if methodExpr[0] != '(' || methodExpr[len(methodExpr)-1] != ')' {
		return nil, "", errors.New("ethcoder: invalid input expr. expected format is: methodName(arg1Type, arg2Type)")
	}

	var inputArgs, outputArgs []abiArgument
	inputArgs = parseArgumentList(methodExpr)
	if returnsExpr != "" {
		outputArgs = parseArgumentList(returnsExpr)
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

func AbiDecoder(argTypes []string, input []byte, argValues []interface{}) error {
	if len(argTypes) != len(argValues) {
		return errors.New("invalid arguments - types and values do not match")
	}
	args, err := buildArgumentsFromTypes(argTypes)
	if err != nil {
		return fmt.Errorf("failed to build abi: %v", err)
	}
	if len(args) > 1 {
		return args.Unpack(&argValues, input)
	} else {
		argValue := argValues[0]
		return args.Unpack(&argValue, input)
	}
}

func AbiDecodeValues(argTypes []string, input []byte) ([]interface{}, error) {
	args, err := buildArgumentsFromTypes(argTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to build abi: %v", err)
	}
	return args.UnpackValues(input)
}

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

func StringifyValues(argValues []interface{}) ([]string, error) {
	strs := []string{}

	for _, argValue := range argValues {
		stringer, ok := argValue.(fmt.Stringer)
		if ok {
			strs = append(strs, stringer.String())
			continue
		}

		switch v := argValue.(type) {
		case nil:
			strs = append(strs, "")
			break

		case string:
			strs = append(strs, v)
			break

		default:
			strs = append(strs, fmt.Sprintf("%v", argValue))
			break
		}
	}

	return strs, nil
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

func parseArgumentList(argumentExpr string) []abiArgument {
	args := []abiArgument{}
	argumentExpr = strings.Trim(argumentExpr, "() ")
	p := strings.Split(argumentExpr, ",")
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
