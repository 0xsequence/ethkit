package ethcoder

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
)

type ABI struct {
	rawABI     abi.ABI
	eventSigs  map[string]ABISignature
	methodSigs map[string]ABISignature
	mu         sync.RWMutex
}

func NewABI() ABI {
	return ABI{
		rawABI: abi.ABI{
			Methods: make(map[string]abi.Method),
			Events:  make(map[string]abi.Event),
			Errors:  make(map[string]abi.Error),
		},
		eventSigs:  make(map[string]ABISignature),
		methodSigs: make(map[string]ABISignature),
	}
}

func (d *ABI) RawABI() abi.ABI {
	return d.rawABI
}

func (d *ABI) AddEvent(eventSig string) (string, error) {
	abiSig, err := ParseABISignature(eventSig)
	if err != nil {
		return "", nil
	}
	return d.AddABISignature(abiSig, true)
}

func (d *ABI) AddMethod(methodSig string) (string, error) {
	abiSig, err := ParseABISignature(methodSig)
	if err != nil {
		return "", nil
	}
	return d.AddABISignature(abiSig, false)
}

func (d *ABI) AddABISignature(abiSig ABISignature, isEvent bool) (string, error) {
	contractABI, name, err := abiSig.ToABI(isEvent)
	if err != nil {
		return name, err
	}

	if isEvent {
		d.mu.Lock()
		d.rawABI.Events[name] = contractABI.Events[name]
		d.eventSigs[name] = abiSig
		d.mu.Unlock()
	} else {
		d.mu.Lock()
		d.rawABI.Methods[name] = contractABI.Methods[name]
		d.methodSigs[name] = abiSig
		d.mu.Unlock()
	}

	return name, nil
}

func (d *ABI) GetMethodABI(methodName string) (abi.Method, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	method, ok := d.rawABI.Methods[methodName]
	return method, ok
}

func (d *ABI) GetEventABI(eventName string) (abi.Event, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	event, ok := d.rawABI.Events[eventName]
	return event, ok
}

func (d *ABI) GetEventABISignature(eventName string) (ABISignature, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	abiSig, ok := d.eventSigs[eventName]
	return abiSig, ok
}

func (d *ABI) GetMethodABISignature(methodName string) (ABISignature, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	abiSig, ok := d.methodSigs[methodName]
	return abiSig, ok
}

func (d *ABI) SetABI(rawABI abi.ABI) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.rawABI = rawABI

	// Add method and event signatures to the cache
	for _, method := range d.rawABI.Methods {
		abiSig, err := ParseABISignature(method.Sig)
		if err != nil {
			return err
		}
		d.methodSigs[method.Name] = abiSig
	}
	for _, event := range d.rawABI.Events {
		abiSig, err := ParseABISignature(event.Sig)
		if err != nil {
			return err
		}
		d.eventSigs[event.Name] = abiSig
	}

	return nil
}

func (d *ABI) AddABIFromJSON(abiJSON string) error {
	if !strings.HasPrefix(abiJSON, "[") && !strings.HasPrefix(abiJSON, "{") {
		return fmt.Errorf("ethcoder: abiJSON must be a valid JSON array or object")
	}
	if strings.HasPrefix(abiJSON, "{") {
		abiJSON = "[" + abiJSON + "]"
	}
	rawABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return err
	}
	return d.SetABI(rawABI)
}

func (d *ABI) AddABIBySigOrJSON(abiSigOrJSON string, isEvent bool) ([]string, error) {
	if strings.HasPrefix(abiSigOrJSON, "[") || strings.HasPrefix(abiSigOrJSON, "{") {
		err := d.AddABIFromJSON(abiSigOrJSON)
		if err != nil {
			return nil, err
		}
		names := []string{}
		if isEvent {
			for _, event := range d.rawABI.Events {
				names = append(names, event.Name)
			}
		} else {
			for _, method := range d.rawABI.Methods {
				names = append(names, method.Name)
			}
		}
		return names, nil
	} else {
		if isEvent {
			event, err := d.AddEvent(abiSigOrJSON)
			if err != nil {
				return nil, err
			}
			return []string{event}, nil
		} else {
			method, err := d.AddMethod(abiSigOrJSON)
			if err != nil {
				return nil, err
			}
			return []string{method}, nil
		}
	}
}

// EncodeMethodCalldata encodes the abi method arguments into calldata bytes. It's expected
// that `argValues` are runtime types, ie. common.Address, or big.Int, etc.
func (d *ABI) EncodeMethodCalldata(methodName string, argValues []interface{}) ([]byte, error) {
	_, ok := d.rawABI.Methods[methodName]
	if !ok {
		return nil, fmt.Errorf("method %s not found", methodName)
	}
	return d.rawABI.Pack(methodName, argValues...)
}

// EncodeMethodCalldataFromStringValuesAny decodes the abi method argument string values into
// runtime types, and then encodes the abi method arguments into calldata bytes.
//
// NOTE: also see the EncodeContractCall() function, which has more capabilities and supports
// nested encoding.
func (d *ABI) EncodeMethodCalldataFromStringValuesAny(methodName string, argStringValues []any) ([]byte, error) {
	_, ok := d.rawABI.Methods[methodName]
	if !ok {
		return nil, fmt.Errorf("method %s not found", methodName)
	}
	abiSig, ok := d.methodSigs[methodName]
	if !ok {
		return nil, fmt.Errorf("method %s not found", methodName)
	}
	argValues, err := ABIUnmarshalStringValuesAny(abiSig.ArgTypes, argStringValues)
	if err != nil {
		return nil, err
	}
	return d.rawABI.Pack(methodName, argValues...)
}

// EncodeMethodCalldataFromStringValues decodes the abi method argument string values into
// runtime types, and then encodes the abi method arguments into calldata bytes.
//
// The difference between this method and EncodeMethodCalldataFromStringValuesAny are just argument
// types for convenience.
func (d *ABI) EncodeMethodCalldataFromStringValues(methodName string, argStringValues []string) ([]byte, error) {
	_, ok := d.rawABI.Methods[methodName]
	if !ok {
		return nil, fmt.Errorf("method %s not found", methodName)
	}
	abiSig, ok := d.methodSigs[methodName]
	if !ok {
		return nil, fmt.Errorf("method %s not found", methodName)
	}
	argValues, err := ABIUnmarshalStringValues(abiSig.ArgTypes, argStringValues)
	if err != nil {
		return nil, err
	}
	return d.rawABI.Pack(methodName, argValues...)
}

// ContractCallDef is a definition for a contract call. It can be used to encode a contract call
// as a hex encoded string.
type ContractCallDef struct {
	// ABI can be an abi signature ie. "transfer(address,uint256)"
	// or it can be a JSON encoded ABI string
	ABI string `json:"abi"`

	// Func is the name of the function / method in the call.
	// NOTE: this is optional if the abi signature is used, or if the abi
	// only has one method.
	Func string `json:"func"`

	// Args is the arguments to the call, which can be nested.
	Args []any `json:"args"`
}

// EncodeContractCall encodes a contract call as a hex encoded calldata.
func EncodeContractCall(callDef ContractCallDef) (string, error) {
	abi := NewABI()

	names, err := abi.AddABIBySigOrJSON(callDef.ABI, false)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no method names added")
	}
	methodName := names[0]

	abiSig, ok := abi.GetMethodABISignature(methodName)
	if !ok {
		return "", fmt.Errorf("method %s not found", methodName)
	}

	// Prepare the arguments, which may be nested
	argStringValues, err := prepareContractCallArgs(callDef.Args)
	if err != nil {
		return "", err
	}

	// Decode argument string values into runtime types based on the abi argument types.
	argValues, err := ABIUnmarshalStringValuesAny(abiSig.ArgTypes, argStringValues)
	if err != nil {
		return "", err
	}

	rawABI := abi.RawABI()

	// Pre-process all argValues to be in the format that the geth abi encoder expects.
	// argValues are runtime types.
	args, err := packableArgValues(rawABI, methodName, argValues)
	if err != nil {
		return "", err
	}

	// Encode abi method arguments into calldata bytes
	packed, err := rawABI.Pack(methodName, args...)
	if err != nil {
		return "", err
	}

	// Return as hex encoded string, with 0x prefix
	return HexEncode(packed), nil
}

func packableArgValues(mabi abi.ABI, method string, argValues []any) ([]any, error) {
	m, ok := mabi.Methods[method]
	if !ok {
		return nil, errors.New("method not found in abi")
	}

	if len(m.Inputs) != len(argValues) {
		return nil, errors.New("method inputs length does not match arg values length")
	}

	out := make([]any, len(argValues))

	for i, input := range m.Inputs {
		isTuple := false
		typ := input.Type.String()
		if len(typ) >= 2 && typ[0] == '(' && typ[len(typ)-1] == ')' {
			isTuple = true
		}

		if !isTuple {
			out[i] = argValues[i]
		} else {
			// build struct for the tuple, as that is what the geth abi encoder expects
			// NOTE: in future we could fork or modify it if we want to avoid the need for this,
			// as it means decoding tuples will be more intensive the necessary.
			fields := []reflect.StructField{}

			v, ok := argValues[i].([]any)
			if !ok {
				vv, ok := argValues[i].([]string)
				if !ok {
					return nil, errors.New("tuple arg values must be an array")
				}
				v = make([]any, len(vv))
				for j, x := range vv {
					v[j] = x
				}
			}

			for j, vv := range v {
				fields = append(fields, reflect.StructField{
					Name: fmt.Sprintf("Name%d", j),
					Type: reflect.TypeOf(vv),
				})
			}

			structType := reflect.StructOf(fields)
			instance := reflect.New(structType).Elem()

			for j, vv := range v {
				instance.Field(j).Set(reflect.ValueOf(vv))
			}
			out[i] = instance.Interface()
		}
	}

	return out, nil
}

func prepareContractCallArgs(args []any) ([]any, error) {
	var err error
	out := make([]any, len(args))

	for i, arg := range args {
		switch arg := arg.(type) {
		case string, []string, []any:
			out[i] = arg

		case map[string]interface{}:
			nst := arg

			var funcName string
			if v, ok := nst["func"].(string); ok {
				funcName = v
			}

			args, ok := nst["args"].([]interface{})
			if !ok {
				return nil, fmt.Errorf("nested encode expects the 'args' field to be an array")
			}

			abi, ok := nst["abi"].(string)
			if !ok {
				return nil, fmt.Errorf("nested encode expects an 'abi' field")
			}

			out[i], err = EncodeContractCall(ContractCallDef{
				ABI:  abi,
				Func: funcName,
				Args: args,
			})
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("abi encoding fail due to invalid arg type, '%T'", arg)
		}
	}

	return out, nil
}
