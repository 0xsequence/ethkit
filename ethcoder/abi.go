package ethcoder

import (
	"fmt"
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

// type EventDef struct {
// 	TopicHash  string   `json:"topicHash"`  // the event topic hash, ie. 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
// 	Name       string   `json:"name"`       // the event name, ie. Transfer
// 	Sig        string   `json:"sig"`        // the event sig, ie. Transfer(address,address,uint256)
// 	ArgTypes   []string `json:"argTypes"`   // the event arg types, ie. [address, address, uint256]
// 	ArgNames   []string `json:"argNames"`   // the event arg names, ie. [from, to, value] or ["","",""]
// 	ArgIndexed []bool   `json:"argIndexed"` // the event arg indexed flag, ie. [true, false, true]
// 	NumIndexed int      `json:"-"`
// }

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

func (d *ABI) GetEventABISignature(eventName string) (ABISignature, bool) {
	// but.. do we always have the abiSignature..? maybe.. we can memoize i guess..
	return ABISignature{}, false
}

func (d *ABI) GetMethodABISignature(methodName string) (ABISignature, bool) {
	// but.. do we always have the abiSignature..? maybe.. we can memoize i guess..
	return ABISignature{}, false
}

func (d *ABI) SetABI(rawABI abi.ABI) error {
	d.rawABI = rawABI
	return nil
}

func (d *ABI) AddABIJSON(j string) error {
	rawABI, err := abi.JSON(strings.NewReader(j))
	if err != nil {
		return err
	}
	d.rawABI = rawABI
	return nil
}

func (d *ABI) EncodeMethodCalldata(methodName string, argValues []interface{}) ([]byte, error) {
	_, ok := d.rawABI.Methods[methodName]
	if !ok {
		return nil, fmt.Errorf("method %s not found", methodName)
	}
	return d.rawABI.Pack(methodName, argValues...)
}

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
