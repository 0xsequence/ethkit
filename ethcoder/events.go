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
	eventDef, err := ParseABISignature(event)
	if err != nil {
		return ethkit.Hash{}, "", fmt.Errorf("ethcoder: %w", err)
	}
	topicHash := common.HexToHash(eventDef.Hash)
	return topicHash, eventDef.Signature, nil
}

func ValidateEventSig(eventSig string) (bool, error) {
	// First parse with eventDef to normalize
	eventDef, err := ParseABISignature(eventSig)
	if err != nil {
		return false, err
	}

	// Then check against the selector to confirm
	selector, err := abi.ParseSelector(eventDef.Signature)
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

func DecodeTransactionLogByEventSig(txnLog types.Log, eventSig string) (ABISignature, []interface{}, bool, error) {
	decoder := NewEventDecoder()
	err := decoder.RegisterEventSig(eventSig)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("DecodeTransactionLogByEventSig: %w", err)
	}
	return decoder.DecodeLog(txnLog)
}

func DecodeTransactionLogByEventSigAsHex(txnLog types.Log, eventSig string) (ABISignature, []string, bool, error) {
	decoder := NewEventDecoder()
	err := decoder.RegisterEventSig(eventSig)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("DecodeTransactionLogByEventSigAsHex: %w", err)
	}
	return decoder.DecodeLogAsHex(txnLog)
}

// ..
func DecodeTransactionLogByContractABIJSON(txnLog types.Log, contractABIJSON string) (ABISignature, []interface{}, bool, error) {
	contractABI, err := abi.JSON(strings.NewReader(contractABIJSON))
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("invalid contract ABI definition: %w", err)
	}
	return DecodeTransactionLogByContractABI(txnLog, contractABI)
}

func DecodeTransactionLogByContractABI(txnLog types.Log, contractABI abi.ABI) (ABISignature, []interface{}, bool, error) {
	decoder := NewEventDecoder()
	err := decoder.RegisterContractABI(contractABI)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("DecodeTransactionLogByContractABI: %w", err)
	}
	return decoder.DecodeLog(txnLog)
}

func DecodeTransactionLogByContractABIAsHex(txnLog types.Log, contractABI abi.ABI) (ABISignature, []string, bool, error) {
	decoder := NewEventDecoder()
	err := decoder.RegisterContractABI(contractABI)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("DecodeTransactionLogByContractABIAsHex: %w", err)
	}
	return decoder.DecodeLogAsHex(txnLog)
}

type EventDecoder struct {
	// options  EventDecoderOptions
	decoders map[string][]eventDecoderDef
}

// type EventDecoderOptions struct {
// 	// BruteForceIndexedArgs will attempt to decode logs even if the number of indexed
// 	// arguments does not match the number of indexed arguments in the event definition.
//  // The logic is to try to decode the log with number of topics - 1, in the argument
//  // serial order, which should work for most cases, but certainly not all.
// 	//
// 	// Default: false
// 	BruteForceIndexedArgs bool
// }

// eventDecoderDef is the decoder definition for a single event
type eventDecoderDef struct {
	ABISignature
	abi abi.ABI
}

func NewEventDecoder() *EventDecoder {
	return &EventDecoder{
		decoders: map[string][]eventDecoderDef{},
	}
}

// func NewEventDecoder(options ...EventDecoderOptions) *EventDecoder {
// 	var opts EventDecoderOptions
// 	if len(options) > 0 {
// 		opts = options[0]
// 	} else {
// 		// defaults
// 		opts = EventDecoderOptions{
// 			// BruteForceIndexedArgs: false,
// 		}
// 	}

// 	return &EventDecoder{
// 		decoders: map[string][]eventDecoderDef{},
// 		options:  opts,
// 	}
// }

func (d *EventDecoder) RegisterEventSig(eventSig ...string) error {
LOOP:
	for _, sig := range eventSig {
		eventDef, err := ParseABISignature(sig)
		if err != nil {
			return fmt.Errorf("ethcoder: %w", err)
		}

		_, ok := d.decoders[eventDef.Hash]
		if !ok {
			d.decoders[eventDef.Hash] = []eventDecoderDef{}
		}

		// Dedupe check
		dds := d.decoders[eventDef.Hash]
		for _, dd := range dds {
			if dd.Signature == eventDef.Signature && dd.NumIndexed == eventDef.NumIndexed {
				continue LOOP
			}
		}

		// Register new
		abi, _, err := eventDef.ToABI(true)
		if err != nil {
			return fmt.Errorf("ethcoder: %w", err)
		}
		d.decoders[eventDef.Hash] = append(dds, eventDecoderDef{
			ABISignature: eventDef,
			abi:          abi,
		})
	}

	return nil
}

func (d *EventDecoder) RegisterContractABIJSON(contractABIJSON string, eventNames ...string) error {
	contractABI, err := abi.JSON(strings.NewReader(contractABIJSON))
	if err != nil {
		return fmt.Errorf("invalid contract ABI definition: %w", err)
	}
	return d.RegisterContractABI(contractABI, eventNames...)
}

func (d *EventDecoder) RegisterContractABI(contractABI abi.ABI, eventNames ...string) error {
	eventDefs, err := abiToABISignatures(contractABI, eventNames)
	if err != nil {
		return fmt.Errorf("RegisterContractABI: %w", err)
	}

LOOP:
	for _, eventDef := range eventDefs {
		_, ok := d.decoders[eventDef.Hash]
		if !ok {
			d.decoders[eventDef.Hash] = []eventDecoderDef{}
		}

		// Dedupe check
		dds := d.decoders[eventDef.Hash]
		for _, dd := range dds {
			if dd.Signature == eventDef.Signature && dd.NumIndexed == eventDef.NumIndexed {
				continue LOOP
			}
		}

		// Register new
		// evABI := abi.ABI{}
		// evABI.Events[eventDef.Name] = contractABI.Events[eventDef.Name]

		d.decoders[eventDef.Hash] = append(dds, eventDecoderDef{
			ABISignature: eventDef,
			// abi:      evABI,
			abi: contractABI,
		})
	}
	return nil
}

func (d *EventDecoder) DecodeLog(log types.Log) (ABISignature, []interface{}, bool, error) {
	dd, err := d.getLogDecoder(log)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("DecodeLog, %w", err)
	}

	if len(log.Topics) == 0 {
		return ABISignature{}, nil, false, fmt.Errorf("log has no topics, unable to decode")
	}

	abiEvent := dd.abi.Events[dd.Name]
	bc := bind.NewBoundContract(log.Address, dd.abi, nil, nil, nil)
	eventMap := map[string]interface{}{}
	err = bc.UnpackLogIntoMap(eventMap, abiEvent.Name, log)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("UnpackLogIntoMap: decoding failed due to %w", err)
	}

	eventValues := []interface{}{}
	for _, arg := range abiEvent.Inputs {
		eventValues = append(eventValues, eventMap[arg.Name])
	}

	return dd.ABISignature, eventValues, true, nil
}

func (d *EventDecoder) DecodeLogAsHex(log types.Log) (ABISignature, []string, bool, error) {
	dd, err := d.getLogDecoder(log)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("DecodeLogAsHex, %w", err)
	}

	abiEvent := dd.abi.Events[dd.Name]
	eventDef := dd.ABISignature

	// fast decode if were not parsing any dynamic types
	var fastDecode bool
	if !strings.Contains(dd.Signature, "[") && strings.Count(dd.Signature, "(") == 1 {
		fastDecode = true
	}

	// fast decode
	if fastDecode {
		eventValues := []string{}
		dataPos := 0
		idx := 0
		for _, arg := range abiEvent.Inputs {
			if arg.Indexed {
				byteSize := abi.GetTypeSize(arg.Type)
				if byteSize > arg.Type.Size {
					byteSize = arg.Type.Size // for case of address type
				}
				if idx+1 > len(log.Topics)-1 {
					return eventDef, nil, false, fmt.Errorf("indexed argument out of range: %d > %d", idx+1, len(log.Topics)-1)
				}
				data := log.Topics[idx+1].Bytes()

				var argVal []byte
				if arg.Type.T == abi.BytesTy || arg.Type.T == abi.FixedBytesTy {
					argVal = data[:byteSize]
				} else {
					argVal = data[32-byteSize:]
				}
				eventValues = append(eventValues, HexEncode(argVal))
				idx++
			} else {
				byteSize := abi.GetTypeSize(arg.Type)
				data := log.Data[dataPos : dataPos+byteSize]
				dataPos += byteSize
				eventValues = append(eventValues, HexEncode(data))
			}
		}

		return eventDef, eventValues, true, nil
	}

	// Decode via abi, which converts to native type, then re-encodes to hex,
	// which is suboptimal, but works.
	eventDef, eventValues, _, err := d.DecodeLog(log)
	if err != nil {
		return ABISignature{}, nil, false, fmt.Errorf("DecodeLogAsHex: %w", err)
	}

	out := []string{}
	for i, arg := range abiEvent.Inputs {
		x := abi.Arguments{arg}
		data, err := x.Pack(eventValues[i])
		if err != nil {
			return ABISignature{}, nil, false, fmt.Errorf("PackValues: %w", err)
		}
		out = append(out, hexutil.Encode(data))
	}

	return eventDef, out, true, nil
}

func (d *EventDecoder) EventDefList() []ABISignature {
	eventDefs := []ABISignature{}
	for _, dds := range d.decoders {
		for _, dd := range dds {
			eventDefs = append(eventDefs, dd.ABISignature)
		}
	}
	return eventDefs
}

func (d *EventDecoder) TopicsList() []string {
	topics := []string{}
	for topic := range d.decoders {
		topics = append(topics, topic)
	}
	return topics
}

func (d *EventDecoder) TopicsMap() map[string]struct{} {
	topics := map[string]struct{}{}
	for topic := range d.decoders {
		topics[topic] = struct{}{}
	}
	return topics
}

func (d *EventDecoder) getLogDecoder(log types.Log) (eventDecoderDef, error) {
	if len(log.Topics) == 0 {
		return eventDecoderDef{}, fmt.Errorf("log has no topics, unable to decode")
	}

	topicHash := log.Topics[0].String()

	decoderDef, ok := d.decoders[topicHash]
	if !ok {
		return eventDecoderDef{}, fmt.Errorf("no decoder found for topic hash: %s", topicHash)
	}

	// TODO: if d.options.BruteForceIndexedArgs is true, we can attempt to decode logs
	// .. we should return the bool though if it failed to decode.. so like returning
	// eventDef, eventValues, false, nil
	// where we dont give an error, but we say false, as we're not sure if it's correct

	logNumIndexed := len(log.Topics) - 1
	for _, dd := range decoderDef {
		if dd.NumIndexed != logNumIndexed {
			continue
		}
		return dd, nil
	}

	return eventDecoderDef{}, fmt.Errorf("no decoder found for topic hash with indexed args: %s", topicHash)
}

// TODO: rename eventNames to names and add isEvent bool, and make it work for both events and methods
func abiToABISignatures(contractABI abi.ABI, eventNames []string) ([]ABISignature, error) {
	eventDefs := []ABISignature{}

	if len(eventNames) == 0 {
		eventNames = []string{}
		for eventName := range contractABI.Events {
			eventNames = append(eventNames, eventName)
		}
	}

	for _, eventName := range eventNames {
		abiEvent, ok := contractABI.Events[eventName]
		if !ok {
			return nil, fmt.Errorf("event not found in contract ABI: %s", eventName)
		}
		if abiEvent.Anonymous {
			return nil, fmt.Errorf("event is anonymous: %s", eventName)
		}

		eventDef := ABISignature{
			Name: eventName,
		}

		typs := []string{}
		indexed := []bool{}
		names := []string{}
		numIndexed := 0
		for _, arg := range abiEvent.Inputs {
			typs = append(typs, arg.Type.String()) // also works with components
			names = append(names, arg.Name)
			indexed = append(indexed, arg.Indexed)

			if arg.Indexed {
				numIndexed++
			}
		}

		eventDef.ArgTypes = typs
		eventDef.ArgNames = names
		eventDef.ArgIndexed = indexed
		eventDef.NumIndexed = numIndexed

		eventDef.Signature = fmt.Sprintf("%s(%s)", eventDef.Name, strings.Join(typs, ","))
		eventDef.Hash = Keccak256Hash([]byte(eventDef.Signature)).String()

		eventDefs = append(eventDefs, eventDef)
	}

	return eventDefs, nil
}
