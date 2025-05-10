package ethcoder

import (
	"fmt"

	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
)

type ABISignature struct {
	Name       string   // the method or event name, ie. Transfer
	Signature  string   // the abi signature string, ie. Transfer(address,address,uint256)
	Hash       string   // the method/event topic hash, ie. 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
	ArgTypes   []string // the method/event arg types, ie. [address, address, uint256]
	ArgNames   []string // the method/event arg names, ie. [from, to, value] or ["","",""]
	ArgIndexed []bool   // the event arg indexed flag, ie. [true, false, true]
	NumIndexed int
}

func (e ABISignature) String() string {
	if !(len(e.ArgTypes) == len(e.ArgIndexed) && len(e.ArgTypes) == len(e.ArgNames)) {
		return "<invalid abi signature definition>"
	}
	s := ""
	for i := range e.ArgTypes {
		s += e.ArgTypes[i]
		if e.ArgIndexed[i] {
			s += " indexed"
		}
		if e.ArgNames[i] != "" {
			s += " " + e.ArgNames[i]
		}
		if i < len(e.ArgTypes)-1 {
			s += ","
		}
	}
	return fmt.Sprintf("%s(%s)", e.Name, s)
}

func (s ABISignature) ToABI(isEvent bool) (abi.ABI, string, error) {
	abiArgs := abi.Arguments{}
	selector, err := abi.ParseSelector(s.Signature)
	if err != nil {
		return abi.ABI{}, "", err
	}

	for i, argType := range s.ArgTypes {
		selectorArg := selector.Inputs[i]

		argName := s.ArgNames[i]
		if argName == "" {
			argName = fmt.Sprintf("arg%d", i+1)
		}

		if selectorArg.Type == "uint" {
			selectorArg.Type = "uint256"
		} else if selectorArg.Type == "int" {
			selectorArg.Type = "int256"
		}

		typ, err := abi.NewType(selectorArg.Type, "", selectorArg.Components)
		if err != nil {
			return abi.ABI{}, "", fmt.Errorf("invalid abi argument type '%s': %w", argType, err)
		}

		abiArgs = append(abiArgs, abi.Argument{Name: argName, Type: typ, Indexed: s.ArgIndexed[i]})
	}

	var contractABI abi.ABI
	if isEvent {
		abiEvent := abi.NewEvent(s.Name, s.Name, false, abiArgs)
		contractABI = abi.ABI{
			Events: map[string]abi.Event{},
		}
		contractABI.Events[s.Name] = abiEvent
	} else {
		abiMethod := abi.NewMethod(s.Name, s.Name, abi.Function, "", false, false, abiArgs, nil)
		contractABI = abi.ABI{
			Methods: map[string]abi.Method{},
		}
		contractABI.Methods[s.Name] = abiMethod
	}

	return contractABI, s.Name, nil
}
