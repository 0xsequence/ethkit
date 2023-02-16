package ethrpc

import (
	"encoding/json"
	"math/big"

	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
)

type Call struct {
	request  jsonrpc.Message
	response *jsonrpc.Message
	resultFn func(message json.RawMessage) error
	err      error
}

func NewCall(method string, params ...any) Call {
	return NewCallBuilder[any](method, nil, params...).Into(nil)
}

func (c *Call) Error() string {
	if c == nil || c.err == nil {
		return ""
	}
	return c.err.Error()
}

func (c *Call) Unwrap() error {
	return c.err
}

type IntoFn[T any] func(raw json.RawMessage, ret *T) error

type CallBuilder[T any] struct {
	err    error
	method string
	params []any
	intoFn IntoFn[T]
}

func NewCallBuilder[T any](method string, intoFn IntoFn[T], params ...any) CallBuilder[T] {
	return CallBuilder[T]{
		method: method,
		params: params,
		intoFn: intoFn,
	}
}

func (b CallBuilder[T]) Into(ret *T) Call {
	if b.err != nil {
		return Call{err: b.err}
	}
	return Call{
		request: jsonrpc.NewRequest(0, b.method, b.params),
		resultFn: func(message json.RawMessage) error {
			if ret == nil {
				return nil
			}
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
		request: jsonrpc.NewRequest(0, b.method, b.params),
		resultFn: func(message json.RawMessage) error {
			if b.intoFn == nil {
				panic("CallBuilder2 must have a non-nil intoFn")
			}
			return b.intoFn(message, ret1, ret2)
		},
	}
}

var Pending = big.NewInt(-1)

func toBlockNumArg(blockNum *big.Int) string {
	if blockNum == nil {
		return "latest"
	}
	if blockNum.Cmp(Pending) == 0 {
		return "pending"
	}
	return hexutil.EncodeBig(blockNum)
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

func hexIntoHash(message json.RawMessage, ret *common.Hash) error {
	var result common.Hash
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = result
	return nil
}
