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

	values := []any{}

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

			var size int64
			var err error
			if len(match[2]) > 0 {
				size, err = strconv.ParseInt(match[2], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting %s. reason: %w", i, typ, err)
				}
				if (size%8 != 0) || size > 256 {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid. invalid number type '%s'", i, typ)
				}
			}

			base := 10
			if strings.HasPrefix(s, "0x") {
				base = 16
				s = s[2:]
			}

			isUint := strings.HasPrefix(match[0], "uint")
			switch size {
			case 8:
				if isUint {
					val, err := strconv.ParseUint(s, base, 8)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, uint8(val))
				} else {
					val, err := strconv.ParseInt(s, base, 8)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, int8(val))
				}
			case 16:
				if isUint {
					val, err := strconv.ParseUint(s, base, 16)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, uint16(val))
				} else {
					val, err := strconv.ParseInt(s, base, 16)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, int16(val))
				}
			case 32:
				if isUint {
					val, err := strconv.ParseUint(s, base, 32)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, uint32(val))
				} else {
					val, err := strconv.ParseInt(s, base, 32)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, int32(val))
				}
			case 64:
				if isUint {
					val, err := strconv.ParseUint(s, base, 64)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, val) // val is already uint64
				} else {
					val, err := strconv.ParseInt(s, base, 64)
					if err != nil {
						return nil, fmt.Errorf("ethcoder: value '%s' at position %d for type %s is invalid: %w", s, i, typ, err)
					}
					values = append(values, val) // val is already int64
				}
			case 0, 128, 256:
				num := big.NewInt(0)
				num, ok = num.SetString(s, base)
				if !ok {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid. expecting number. unable to set value of '%s'", i, s)
				}
				values = append(values, num)
			default:
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, number type '%s' is invalid", i, typ)
			}

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

			// convert to fixed array size, as runtime encoder expects it so
			switch size {
			case 1:
				var v [1]byte
				copy(v[:], val)
				values = append(values, v)
			case 2:
				var v [2]byte
				copy(v[:], val)
				values = append(values, v)
			case 3:
				var v [3]byte
				copy(v[:], val)
				values = append(values, v)
			case 4:
				var v [4]byte
				copy(v[:], val)
				values = append(values, v)
			case 5:
				var v [5]byte
				copy(v[:], val)
				values = append(values, v)
			case 6:
				var v [6]byte
				copy(v[:], val)
				values = append(values, v)
			case 7:
				var v [7]byte
				copy(v[:], val)
				values = append(values, v)
			case 8:
				var v [8]byte
				copy(v[:], val)
				values = append(values, v)
			case 9:
				var v [9]byte
				copy(v[:], val)
				values = append(values, v)
			case 10:
				var v [10]byte
				copy(v[:], val)
				values = append(values, v)
			case 11:
				var v [11]byte
				copy(v[:], val)
				values = append(values, v)
			case 12:
				var v [12]byte
				copy(v[:], val)
				values = append(values, v)
			case 13:
				var v [13]byte
				copy(v[:], val)
				values = append(values, v)
			case 14:
				var v [14]byte
				copy(v[:], val)
				values = append(values, v)
			case 15:
				var v [15]byte
				copy(v[:], val)
				values = append(values, v)
			case 16:
				var v [16]byte
				copy(v[:], val)
				values = append(values, v)
			case 17:
				var v [17]byte
				copy(v[:], val)
				values = append(values, v)
			case 18:
				var v [18]byte
				copy(v[:], val)
				values = append(values, v)
			case 19:
				var v [19]byte
				copy(v[:], val)
				values = append(values, v)
			case 20:
				var v [20]byte
				copy(v[:], val)
				values = append(values, v)
			case 21:
				var v [21]byte
				copy(v[:], val)
				values = append(values, v)
			case 22:
				var v [22]byte
				copy(v[:], val)
				values = append(values, v)
			case 23:
				var v [23]byte
				copy(v[:], val)
				values = append(values, v)
			case 24:
				var v [24]byte
				copy(v[:], val)
				values = append(values, v)
			case 25:
				var v [25]byte
				copy(v[:], val)
				values = append(values, v)
			case 26:
				var v [26]byte
				copy(v[:], val)
				values = append(values, v)
			case 27:
				var v [27]byte
				copy(v[:], val)
				values = append(values, v)
			case 28:
				var v [28]byte
				copy(v[:], val)
				values = append(values, v)
			case 29:
				var v [29]byte
				copy(v[:], val)
				values = append(values, v)
			case 30:
				var v [30]byte
				copy(v[:], val)
				values = append(values, v)
			case 31:
				var v [31]byte
				copy(v[:], val)
				values = append(values, v)
			case 32:
				var v [32]byte
				copy(v[:], val)
				values = append(values, v)
			default:
				values = append(values, val)
			}
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
			switch v := v.(type) {
			case string:
				// v is string array, ie. `["1","2","3"]`
				err = json.Unmarshal([]byte(v), &stringValues)
				if err != nil {
					return nil, fmt.Errorf("ethcoder: value at position %d is invalid. failed to unmarshal json string array '%s'", i, v)
				}
			case []string:
				// v is array of strings, ie. ["1","2","3"]
				stringValues = v
			case []any:
				// v is array of any runtime type, but still strings ie. ["1","2","3"]
				var ok bool
				s := make([]string, len(v))
				for j, x := range v {
					s[j], ok = x.(string)
					if !ok {
						return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting string array", i)
					}
				}
				stringValues = s
			default:
				return nil, fmt.Errorf("ethcoder: value at position %d is invalid, expecting string array", i)
			}

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
	in := []any{}
	for _, v := range stringValues {
		in = append(in, v)
	}
	return ABIUnmarshalStringValuesAny(argTypes, in)
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
		isTuple := strings.Contains(argType, "(") && strings.Contains(argType, ")")
		if isTuple {
			// NOTE: if you come across this error, see abi_test.go TestABIEncodeTuple example.
			return nil, fmt.Errorf("ethcoder: tuples are not supported by this encoder, please try another method")
		}
		abiType, err := abi.NewType(argType, "", nil)
		if err != nil {
			return nil, err
		}
		args = append(args, abi.Argument{Type: abiType})
	}
	return args, nil
}
