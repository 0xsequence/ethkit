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

func ABIPackArguments(argTypes []string, argValues []interface{}) ([]byte, error) {
	if len(argTypes) != len(argValues) {
		return nil, errors.New("invalid arguments - types and values do not match")
	}
	args, err := buildArgumentsFromTypes(argTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to build abi: %v", err)
	}
	return args.Pack(argValues...)
}

func ABIPackArgumentsHex(argTypes []string, argValues []interface{}) (string, error) {
	b, err := ABIPackArguments(argTypes, argValues)
	if err != nil {
		return "", err
	}
	h := hexutil.Encode(b)
	return h, nil
}

func ABIUnpackArgumentsByRef(argTypes []string, input []byte, outArgValues []interface{}) error {
	if len(argTypes) != len(outArgValues) {
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
		return args.Copy(&outArgValues, values)
	} else {
		return args.Copy(&outArgValues[0], values)
	}
}

func ABIUnpackArguments(argTypes []string, input []byte) ([]interface{}, error) {
	args, err := buildArgumentsFromTypes(argTypes)
	if err != nil {
		return nil, fmt.Errorf("failed to build abi: %v", err)
	}
	return args.UnpackValues(input)
}

// TODO: change expr argument to abiXX like abiExprOrJSON
func ABIUnpack(exprSig string, input []byte, argValues []interface{}) error {
	if len(exprSig) == 0 {
		return errors.New("ethcoder: exprSig is required")
	}
	if exprSig[0] != '(' {
		exprSig = "(" + exprSig + ")"
	}
	abiSig, err := ParseABISignature(exprSig)
	if err != nil {
		return err
	}
	return ABIUnpackArgumentsByRef(abiSig.ArgTypes, input, argValues)
}

// TODO: change expr argument to abiXX like abiExprOrJSON
func ABIUnpackAndStringify(exprSig string, input []byte) ([]string, error) {
	if len(exprSig) == 0 {
		return nil, errors.New("ethcoder: exprSig is required")
	}
	if exprSig[0] != '(' {
		exprSig = "(" + exprSig + ")"
	}
	abiSig, err := ParseABISignature(exprSig)
	if err != nil {
		return nil, err
	}
	return ABIMarshalStringValues(abiSig.ArgTypes, input)
}

func ABIMarshalStringValues(argTypes []string, input []byte) ([]string, error) {
	values, err := ABIUnpackArguments(argTypes, input)
	if err != nil {
		return nil, err
	}
	return StringifyValues(values)
}

// AbiUnmarshalStringValuesAny will take an array of ethereum types as string values, and decode
// the string values to runtime objects. This allows simple string value input from an app
// or user, and converts them to the appropriate runtime objects.
//
// NOTE: this is a variant of AbiUnmarshalStringValues but the `stringValues` argument type
// is []any, in order to support input types of array of strings for abi types like `address[]`
// and tuples.
//
// For example, some valid inputs:
//   - AbiUnmarshalStringValuesAny([]string{"address","uint256"}, []any{"0x1234...", "543"})
//     returns []interface{}{common.HexToAddress("0x1234..."), big.NewInt(543)}
//   - AbiUnmarshalStringValuesAny([]string{"address[]", []any{[]string{"0x1234...", "0x5678..."}})
//     returns []interface{}{[]common.Address{common.HexToAddress("0x1234..."), common.HexToAddress("0x5678...")}}
//   - AbiUnmarshalStringValuesAny([]string{"(address,uint256)"}, []any{[]any{"0x1234...", "543"}})
//     returns []interface{}{[]interface{}{common.HexToAddress("0x1234..."), big.NewInt(543)}}
//
// The common use for this method is to pass a JSON object of string values for an abi method
// and have it properly encode to the native abi types.
func ABIUnmarshalStringValuesAny(argTypes []string, stringValues []any) ([]any, error) {
	if len(argTypes) != len(stringValues) {
		return nil, fmt.Errorf("ethcoder: argTypes and stringValues must be of equal length")
	}

	values := []interface{}{}

	for i, typ := range argTypes {
		v := stringValues[i]

		switch typ {
		case "address":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting address in hex", i)
			}

			// expected "0xabcde......"
			if !strings.HasPrefix(s, "0x") {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting address in hex", i)
			}
			values = append(values, common.HexToAddress(s))
			continue

		case "string":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting string", i)
			}

			// expected: string value
			values = append(values, s)
			continue

		case "bytes":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting bytes in hex", i)
			}

			// expected: bytes in hex encoding with 0x prefix
			if strings.HasPrefix(s, "0x") {
				values = append(values, common.Hex2Bytes(s[2:]))
				continue
			} else {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting bytes in hex", i)
			}

		case "bool":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting bool as 'true' or 'false'", i)
			}

			// expected: "true" | "false"
			if s == "true" {
				values = append(values, true)
			} else if s == "false" {
				values = append(values, false)
			} else {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting bool as 'true' or 'false'", i)
			}
			continue
		}

		// numbers
		if match := regexArgNumber.FindStringSubmatch(typ); len(match) > 0 {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting number as string", i)
			}

			size, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting %s. reason: %w", i, typ, err)
			}
			if (size%8 != 0) || size == 0 || size > 256 {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. invalid number type '%s'", i, typ)
			}

			num := big.NewInt(0)
			num, ok = num.SetString(s, 10)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting number. unable to set value of '%s'", i, s)
			}
			values = append(values, num)
			continue
		}

		// bytesXX (fixed)
		if match := regexArgBytes.FindStringSubmatch(typ); len(match) > 0 {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting bytes in hex", i)
			}

			if !strings.HasPrefix(s, "0x") {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting bytes in hex", i)
			}
			size, err := strconv.ParseInt(match[1], 10, 64)
			if err != nil {
				return nil, err
			}
			if size == 0 || size > 32 {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, bytes type '%s' is invalid", i, typ)
			}
			val := common.Hex2Bytes(s[2:])
			if int64(len(val)) != size {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, %s type expects a %d byte value but received %d", i, typ, size, len(val))
			}
			values = append(values, val)
			continue
		}

		// arrays
		// TODO: can we avoid regexp...?
		if match := regexArgArray.FindStringSubmatch(typ); len(match) > 0 {
			baseTyp := match[1]
			if match[2] == "" {
				match[2] = "0"
			}
			count, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				return nil, err
			}

			if baseTyp != "address" {
				submatch := regexArgNumber.FindStringSubmatch(baseTyp)
				if len(submatch) == 0 {
					return nil, fmt.Errorf("ethcoder: value at position %d of type %s is unsupported. Only number string arrays are presently supported", i, typ)
				}
			}

			s, ok := v.([]string)
			if !ok {
				vv, ok := v.([]any)
				if !ok {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting string array", i)
				}
				s = make([]string, len(vv))
				for j, x := range vv {
					s[j], ok = x.(string)
					if !ok {
						return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting string array", i)
					}
				}
			}

			stringValues := s
			if count > 0 && len(stringValues) != int(count) {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, array size does not match required size of %d", i, count)
			}

			var arrayArgs []string
			for i := 0; i < len(stringValues); i++ {
				arrayArgs = append(arrayArgs, baseTyp)
			}

			arrayValues, err := ABIUnmarshalStringValues(arrayArgs, stringValues)
			if err != nil {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, failed to get string values for array - %w", i, err)
			}

			if baseTyp == "address" {
				var addresses []common.Address
				for _, element := range arrayValues {
					address, ok := element.(common.Address)
					if !ok {
						return nil, fmt.Errorf("ethcoder: expected common.Address, got %v", element)
					}
					addresses = append(addresses, address)
				}
				values = append(values, addresses)
			} else {
				var bnArray []*big.Int
				for _, n := range arrayValues {
					bn, ok := n.(*big.Int)
					if !ok {
						return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting array element to be *big.Int", i)
					}
					bnArray = append(bnArray, bn)
				}
				values = append(values, bnArray)
			}
		}

		// tuples
		idx := strings.Index(typ, "(")
		idx2 := strings.Index(typ, ")")
		if idx >= 0 && idx2 > 0 {

			// TODO: add nested tuple support in the future:
			// need to encode inner parts first, find the next '(' after idx .. etc.. and call AbiUnmarshalStringValuesAny recursively

			t := typ[idx+1 : idx2]
			idx := strings.Index(t, "(")
			if idx >= 0 {
				return nil, fmt.Errorf("ethcoder: value at position %d has found a nested tuple, which is unsupported currently, please contact support and make a request", i)
			}
			args := strings.Split(t, ",")

			var vv []any
			switch v := v.(type) {
			case []any:
				if len(v) != len(args) {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid, tuple size does not match required size of %d", i, len(args))
				}
				vv = v
			case []string:
				if len(v) != len(args) {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid, tuple size does not match required size of %d", i, len(args))
				}
				vv = make([]any, len(v))
				for i, x := range v {
					vv[i] = x
				}
			default:
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting array of any or string", i)
			}

			out, err := ABIUnmarshalStringValuesAny(args, vv)
			if err != nil {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, failed to get string values for tuple: %w", i, err)
			}
			values = append(values, out)
		}
	}

	return values, nil
}

// AbiUnmarshalStringValues will take an array of ethereum types as string values, and decode
// the string values to runtime objects. This allows simple string value input from an app
// or user, and converts them to the appropriate runtime objects.
//
// The common use for this method is to pass a JSON object of string values for an abi method
// and have it properly encode to the native abi types.
func ABIUnmarshalStringValues(argTypes []string, stringValues []string) ([]any, error) {
	if len(argTypes) != len(stringValues) {
		return nil, fmt.Errorf("ethcoder: argTypes and stringValues must be of equal length")
	}

	values := []interface{}{}

	for i, typ := range argTypes {
		s := stringValues[i]

		switch typ {
		case "address":
			// expected "0xabcde......"
			if !strings.HasPrefix(s, "0x") {
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
			if strings.HasPrefix(s, "0x") {
				values = append(values, common.Hex2Bytes(s[2:]))
				continue
			} else {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting bytes in hex", i)
			}

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
			if !strings.HasPrefix(s, "0x") {
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
		// TODO: can we avoid regexp...?
		if match := regexArgArray.FindStringSubmatch(typ); len(match) > 0 {
			baseTyp := match[1]
			if match[2] == "" {
				match[2] = "0"
			}
			count, err := strconv.ParseInt(match[2], 10, 64)
			if err != nil {
				return nil, err
			}

			if baseTyp != "address" {
				submatch := regexArgNumber.FindStringSubmatch(baseTyp)
				if len(submatch) == 0 {
					return nil, fmt.Errorf("ethcoder: value at position %d of type %s is unsupported. Only number string arrays are presently supported", i, typ)
				}
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

			arrayValues, err := ABIUnmarshalStringValues(arrayArgs, stringValues)
			if err != nil {
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid. failed to get string values for array - %w", i, err)
			}

			if baseTyp == "address" {
				var addresses []common.Address
				for _, element := range arrayValues {
					address, ok := element.(common.Address)
					if !ok {
						return nil, fmt.Errorf("ethcoder: expected common.Address, got %v", element)
					}
					addresses = append(addresses, address)
				}
				values = append(values, addresses)
			} else {
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

		// tuples
		idx := strings.Index(typ, "(")
		idx2 := strings.Index(typ, ")")
		if idx >= 0 && idx2 > 0 {
			// NOTE: perhaps we can support tuples here too..? but really its better to use
			// AbiUnmarshalStringValuesAny anyways.
			return nil, fmt.Errorf("ethcoder: tuples are not supported by this method, use AbiUnmarshalStringValuesAny instead")
		}
	}

	return values, nil
}

func ABIEncodeMethodCalldata(methodSig string, argValues []interface{}) ([]byte, error) {
	abi := NewABI()
	methodName, err := abi.AddMethod(methodSig)
	if err != nil {
		return nil, err
	}
	return abi.EncodeMethodCalldata(methodName, argValues)
}

func ABIEncodeMethodCalldataFromStringValues(methodSig string, argStringValues []string) ([]byte, error) {
	abi := NewABI()
	methodName, err := abi.AddMethod(methodSig)
	if err != nil {
		return nil, err
	}
	return abi.EncodeMethodCalldataFromStringValues(methodName, argStringValues)
}

func ABIEncodeMethodCalldataFromStringValuesAny(methodSig string, argStringValues []any) ([]byte, error) {
	abi := NewABI()
	methodName, err := abi.AddMethod(methodSig)
	if err != nil {
		return nil, err
	}
	return abi.EncodeMethodCalldataFromStringValuesAny(methodName, argStringValues)
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
