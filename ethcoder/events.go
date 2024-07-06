package ethcoder

import (
	"fmt"
	"strings"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
	"github.com/0xsequence/ethkit/go-ethereum/common"
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

type EventDef struct {
	TopicHash string   `json:"topicHash"` // the event topic hash, ie. 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
	Name      string   `json:"name"`      // the event name, ie. Transfer
	Sig       string   `json:"sig"`       // the event sig, ie. Transfer(address,address,uint256)
	ArgTypes  []string `json:"argTypes"`  // the event arg types, ie. [address, address, uint256]
	ArgNames  []string `json:"argNames"`  // the event arg names, ie. [from, to, value] or ["","",""]
}

func ParseEventDef(event string) (EventDef, error) {
	eventDef := EventDef{
		ArgTypes: []string{},
		ArgNames: []string{},
	}

	var errInvalid = fmt.Errorf("event format is invalid, expecting Method(arg1,arg2,..)")

	if !strings.Contains(event, "(") || !strings.Contains(event, ")") {
		return eventDef, errInvalid
	}
	p := strings.Split(event, "(")
	if len(p) != 2 {
		return eventDef, errInvalid
	}
	method := strings.TrimSpace(p[0])
	eventDef.Name = method

	args := strings.TrimSuffix(p[1], ")")
	if args == "" {
		eventDef.Sig = fmt.Sprintf("%s()", method)
	} else {
		typs := []string{}
		names := []string{}

		p = strings.Split(args, ",")
		for _, a := range p {
			arg := strings.Split(strings.TrimSpace(a), " ")

			typ := arg[0]
			typs = append(typs, typ)

			var name string
			if len(arg) > 1 {
				name = arg[len(arg)-1]
			}
			names = append(names, name)
		}

		eventDef.ArgTypes = typs
		eventDef.ArgNames = names

		eventDef.Sig = fmt.Sprintf("%s(%s)", method, strings.Join(typs, ","))
	}

	eventDef.TopicHash = Keccak256Hash([]byte(eventDef.Sig)).String()

	return eventDef, nil
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

	for i, argType := range eventDef.ArgTypes {
		argName := eventDef.ArgNames[i]
		if argName == "" {
			argName = fmt.Sprintf("arg%d", i)
		}

		typ, err := abi.NewType(argType, "", nil)
		if err != nil {
			return eventDef, nil, false, fmt.Errorf("invalid abi argument type '%s': %w", argType, err)
		}

		abiArgs = append(abiArgs, abi.Argument{Name: argName, Type: typ, Indexed: i < numIndexedArgs})
	}

	if !returnHexValues {

		// Decode into native runtime types
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

		eventMap := map[string]interface{}{}
		err = bc.UnpackLogIntoMap(eventMap, abiEvent.Name, txnLog)
		if err != nil {
			return eventDef, nil, false, fmt.Errorf("UnpackLogIntoMap: %w", err)
		}

		eventValues := []interface{}{}
		for _, arg := range args {
			eventValues = append(eventValues, eventMap[arg])
		}

		return eventDef, eventValues, true, nil

	} else {
		// Decode into hex values, which means []interface{} will always return array of strings.
		// This is useful in cases when you want to return the hex values of the values instead
		// of decoding to runtime types.

		eventValues := []interface{}{}
		dataPos := 0

		for i, arg := range abiArgs {
			if arg.Indexed {
				byteSize := abi.GetTypeSize(arg.Type)
				if byteSize > arg.Type.Size {
					byteSize = arg.Type.Size // for case of address type
				}
				data := txnLog.Topics[i+1].Bytes()[32-byteSize:]
				eventValues = append(eventValues, HexEncode(data))
			} else {
				byteSize := abi.GetTypeSize(arg.Type)
				data := txnLog.Data[dataPos : dataPos+byteSize]
				dataPos += byteSize
				eventValues = append(eventValues, HexEncode(data))
			}
		}

		return eventDef, eventValues, true, nil
	}
}
