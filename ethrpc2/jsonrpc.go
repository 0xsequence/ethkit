package ethrpc2

import (
	"encoding/json"
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
)

type jsonrpcMessage struct {
	Version string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int             `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type Call struct {
	request  *jsonrpcMessage
	resultFn func(message json.RawMessage) error
	err      error
}

type CallBuilder[T any] struct {
	err    error
	method string
	params []any
	intoFn func(message json.RawMessage, ret *T) error
}

func (b CallBuilder[T]) Into(ret *T) Call {
	if b.err != nil {
		return Call{err: b.err}
	}
	return Call{
		request: makeMessage(b.method, b.params),
		resultFn: func(message json.RawMessage) error {
			if b.intoFn != nil {
				return b.intoFn(message, ret)
			}
			return json.Unmarshal(message, ret)
		},
	}
}

type CallBuilder2[T1, T2 any] struct {
	method string
	params []any
	intoFn func(message json.RawMessage, ret1 *T1, ret2 *T2) error
}

func (b CallBuilder2[T1, T2]) Into(ret1 *T1, ret2 *T2) Call {
	return Call{
		request: makeMessage(b.method, b.params),
		resultFn: func(message json.RawMessage) error {
			if b.intoFn == nil {
				panic("CallBuilder2 must have a non-nil intoFn")
			}
			return b.intoFn(message, ret1, ret2)
		},
	}
}

var Pending = big.NewInt(-1)

func makeMessage(method string, params []any) *jsonrpcMessage {
	return &jsonrpcMessage{
		Version: "2.0",
		Method:  method,
		Params:  params,
	}
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	if number.Cmp(Pending) == 0 {
		return "pending"
	}
	return hexutil.EncodeBig(number)
}

func toCallArg(msg ethereum.CallMsg) any {
	arg := map[string]any{
		"from": msg.From,
		"to":   msg.To,
	}
	if len(msg.Data) > 0 {
		arg["data"] = hexutil.Bytes(msg.Data)
	}
	if msg.Value != nil {
		arg["value"] = (*hexutil.Big)(msg.Value)
	}
	if msg.Gas != 0 {
		arg["gas"] = hexutil.Uint64(msg.Gas)
	}
	if msg.GasPrice != nil {
		arg["gasPrice"] = (*hexutil.Big)(msg.GasPrice)
	}
	return arg
}

func hexIntoBigInt(message json.RawMessage, ret **big.Int) error {
	var result hexutil.Big
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = (*big.Int)(&result)
	return nil
}

func hexIntoUint64(message json.RawMessage, ret *uint64) error {
	var result hexutil.Uint64
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = uint64(result)
	return nil
}

func hexIntoUint(message json.RawMessage, ret *uint) error {
	var result hexutil.Uint
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = uint(result)
	return nil
}

func hexIntoBigUint64(message json.RawMessage, ret **big.Int) error {
	var result hexutil.Uint64
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = big.NewInt(int64(result))
	return nil
}

func hexIntoBytes(message json.RawMessage, ret *[]byte) error {
	var result hexutil.Bytes
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = result
	return nil
}
