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
	request    jsonrpc.Message
	response   *jsonrpc.Message
	resultFn   func(message json.RawMessage) error
	err        error
	strictness StrictnessLevel
}

func NewCall(method string, params ...any) Call {
	return NewCallBuilder[any](method, nil, params...).Into(nil)
}

func (c Call) Strict(strictness StrictnessLevel) Call {
	c.strictness = strictness
	return c
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

type IntoFn[T any] func(raw json.RawMessage, ret *T, strictness StrictnessLevel) error

type CallBuilder[T any] struct {
	err        error
	method     string
	params     []any
	intoFn     IntoFn[T]
	strictness StrictnessLevel
}

func NewCallBuilder[T any](method string, intoFn IntoFn[T], params ...any) CallBuilder[T] {
	return CallBuilder[T]{
		method: method,
		params: params,
		intoFn: intoFn,
	}
}

func (b CallBuilder[T]) Strict(strictness StrictnessLevel) CallBuilder[T] {
	b.strictness = strictness
	return b
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
				return b.intoFn(message, ret, b.strictness)
			}
			return json.Unmarshal(message, ret)
		},
	}
}

type CallBuilder2[T1, T2 any] struct {
	method     string
	params     []any
	intoFn     func(message json.RawMessage, ret1 *T1, ret2 *T2, strictness StrictnessLevel) error
	strictness StrictnessLevel
}

func (b CallBuilder2[T1, T2]) Strict(strictness StrictnessLevel) CallBuilder2[T1, T2] {
	b.strictness = strictness
	return b
}

func (b CallBuilder2[T1, T2]) Into(ret1 *T1, ret2 *T2) Call {
	return Call{
		request: jsonrpc.NewRequest(0, b.method, b.params),
		resultFn: func(message json.RawMessage) error {
			if b.intoFn == nil {
				panic("CallBuilder2 must have a non-nil intoFn")
			}
			return b.intoFn(message, ret1, ret2, b.strictness)
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

func hexIntoBigInt(message json.RawMessage, ret **big.Int, strictness StrictnessLevel) error {
	var result hexutil.Big
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = (*big.Int)(&result)
	return nil
}

func hexIntoUint64(message json.RawMessage, ret *uint64, strictness StrictnessLevel) error {
	if len(message) == 4 && string(message) == "null" {
		*ret = 0
		return nil
	}

	var result hexutil.Uint64
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = uint64(result)
	return nil
}

func hexIntoUint(message json.RawMessage, ret *uint, strictness StrictnessLevel) error {
	if len(message) == 4 && string(message) == "null" {
		*ret = 0
		return nil
	}

	var result hexutil.Uint
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = uint(result)
	return nil
}

func hexIntoBigUint64(message json.RawMessage, ret **big.Int, strictness StrictnessLevel) error {
	var result hexutil.Uint64
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = big.NewInt(int64(result))
	return nil
}

func hexIntoBytes(message json.RawMessage, ret *[]byte, strictness StrictnessLevel) error {
	var result hexutil.Bytes
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = result
	return nil
}

func hexIntoHash(message json.RawMessage, ret *common.Hash, strictness StrictnessLevel) error {
	var result common.Hash
	if err := json.Unmarshal(message, &result); err != nil {
		return err
	}
	*ret = result
	return nil
}
