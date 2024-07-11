package ethcoder

import (
	"fmt"
	"strings"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// EventTopicHash returns the keccak256 hash of the event signature
//
// e.g. "Transfer(address indexed from, address indexed to, uint256 value)"
// will return 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
func EventTopicHash(event string) (ethkit.Hash, string, error) {
	eventDef, err := ParseEventDef(event)
	if err != nil {
		return ethkit.Hash{}, "", fmt.Errorf("ethcoder: %w", err)
	}
	topicHash := common.HexToHash(eventDef.TopicHash)
	return topicHash, eventDef.Sig, nil
}

func ValidateEventSig(eventSig string) (bool, error) {
	selector, err := abi.ParseSelector(eventSig)
	if err != nil {
		return false, err
	}
	for _, arg := range selector.Inputs {
		// NOTE: strangely the abi.NewType is't very strict,
		// and if you pass it "uint2ffff" it will consider it a valid type
		_, err := abi.NewType(arg.Type, "", arg.Components)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// ..
func DecodeTransactionLogByContractABIJSON(txnLog types.Log, contractABIJSON string) (EventDef, []interface{}, bool, error) {
	contractABI, err := abi.JSON(strings.NewReader(contractABIJSON))
	if err != nil {
		return EventDef{}, nil, false, fmt.Errorf("invalid contract ABI definition: %w", err)
	}

	return DecodeTransactionLogByContractABI(txnLog, contractABI)
}

// ..
func DecodeTransactionLogByContractABI(txnLog types.Log, contractABI abi.ABI) (EventDef, []interface{}, bool, error) {
	eventDef := EventDef{}
	topicHash := txnLog.Topics[0]
	eventDef.TopicHash = topicHash.String()

	abiEvent, err := contractABI.EventByID(topicHash)
	if err != nil {
		return EventDef{}, nil, false, nil
	}

	eventDef.Name = abiEvent.Name

	args := []string{}
	typs := []string{}
	for _, arg := range abiEvent.Inputs {
		args = append(args, arg.Name)
		typs = append(typs, arg.Type.String())
	}
	eventDef.ArgNames = args
	eventDef.ArgTypes = typs

	bc := bind.NewBoundContract(txnLog.Address, contractABI, nil, nil, nil)

	eventMap := map[string]interface{}{}
	err = bc.UnpackLogIntoMap(eventMap, abiEvent.Name, txnLog)
	if err != nil {
		return EventDef{}, nil, false, fmt.Errorf("DecodeLogEventByContractABI: %w", err)
	}

	eventDef.Sig = fmt.Sprintf("%s(%s)", eventDef.Name, strings.Join(typs, ","))

	eventValues := []interface{}{}
	for _, arg := range args {
		eventValues = append(eventValues, eventMap[arg])
	}

	return eventDef, eventValues, true, nil
}

func DecodeTransactionLogByEventSig(txnLog types.Log, eventSig string, returnHexValues bool) (EventDef, []interface{}, bool, error) {
	eventDef, err := ParseEventDef(eventSig)
	if err != nil {
		return eventDef, nil, false, fmt.Errorf("ParseEventDef: %w", err)
	}

	// Lets build a mini abi on-demand, and decode it
	abiArgs := abi.Arguments{}
	numIndexedArgs := len(txnLog.Topics) - 1
	if numIndexedArgs < 0 {
		numIndexedArgs = 0 // for anonymous events
	}

	// fast decode if were not parsing any dynamic types
	var fastDecode bool
	if !strings.Contains(eventSig, "[") && strings.Count(eventSig, "(") == 1 {
		fastDecode = true
	}

	// only parse selector if its a dynamic type
	var selector abi.SelectorMarshaling
	if !fastDecode {
		selector, err = abi.ParseSelector(eventDef.Sig)
		if err != nil {
			return eventDef, nil, false, fmt.Errorf("ParseSelector: %w", err)
		}
	}

	for i, argType := range eventDef.ArgTypes {
		var selectorArg abi.ArgumentMarshaling
		selectorArg.Type = argType
		if !fastDecode {
			selectorArg = selector.Inputs[i]
		}

		argName := eventDef.ArgNames[i]
		if argName == "" {
			argName = fmt.Sprintf("arg%d", i)
		}

		typ, err := abi.NewType(selectorArg.Type, "", selectorArg.Components)
		if err != nil {
			return eventDef, nil, false, fmt.Errorf("invalid abi argument type '%s': %w", argType, err)
		}

		// NOTE: if we do not know which arguments are indexed (which is the case for many), then we assume
		// the indexed arguments are in order, which is not a great assumption to make. In this case,
		// we're unable to properly decode the event log without knowing the event signature.
		abiArgs = append(abiArgs, abi.Argument{Name: argName, Type: typ, Indexed: i < numIndexedArgs})
	}

	// Fast decode
	if returnHexValues && fastDecode {
		// Decode into hex values, which means []interface{} will always return array of strings.
		// This is useful in cases when you want to return the hex values of the values instead
		// of decoding to runtime types.

		// fast decode
		eventValues := []interface{}{}
		dataPos := 0

		idx := 0
		for _, arg := range abiArgs {
			if arg.Indexed {
				byteSize := abi.GetTypeSize(arg.Type)
				if byteSize > arg.Type.Size {
					byteSize = arg.Type.Size // for case of address type
				}
				if idx+1 > len(txnLog.Topics)-1 {
					return eventDef, nil, false, fmt.Errorf("indexed argument out of range: %d > %d", idx+1, len(txnLog.Topics)-1)
				}
				data := txnLog.Topics[idx+1].Bytes()
				arg := data[32-byteSize:]
				eventValues = append(eventValues, HexEncode(arg))
				idx++
			} else {
				byteSize := abi.GetTypeSize(arg.Type)
				data := txnLog.Data[dataPos : dataPos+byteSize]
				dataPos += byteSize
				eventValues = append(eventValues, HexEncode(data))
			}
		}

		return eventDef, eventValues, true, nil
	}

	// Decode via abi
	abiEvent := abi.NewEvent(eventDef.Name, eventDef.Name, false, abiArgs)
	contractABI := abi.ABI{
		Events: map[string]abi.Event{},
	}
	contractABI.Events[eventDef.Name] = abiEvent

	args := []string{}
	for _, arg := range abiEvent.Inputs {
		args = append(args, arg.Name)
	}

	bc := bind.NewBoundContract(txnLog.Address, contractABI, nil, nil, nil)

	// NOTE: UnpackLogIntoMap can fail if the eventSig does not include which
	// arguments are indexed. In this case, we're unable to properly decode the event log.
	eventMap := map[string]interface{}{}
	err = bc.UnpackLogIntoMap(eventMap, abiEvent.Name, txnLog)
	if err != nil {
		return eventDef, nil, false, fmt.Errorf("UnpackLogIntoMap: decoding failed due to %w", err)
	}

	eventValues := []interface{}{}
	for _, arg := range args {
		eventValues = append(eventValues, eventMap[arg])
	}

	// Return native values
	if !returnHexValues {
		return eventDef, eventValues, true, nil
	}

	// Re-encode back to hex values
	// TODO: perhaps there is a faster way to do this to just extract the hex values from the log
	if len(eventValues) != len(abiArgs) {
		return eventDef, nil, false, fmt.Errorf("event values length mismatch: %d != %d", len(eventValues), len(abiArgs))
	}

	out := []interface{}{}
	for i, abiArg := range abiArgs {
		x := abi.Arguments{abiArg}
		data, err := x.Pack(eventValues[i])
		if err != nil {
			return eventDef, nil, false, fmt.Errorf("PackValues: %w", err)
		}
		out = append(out, hexutil.Encode(data))
	}
	return eventDef, out, true, nil
}
