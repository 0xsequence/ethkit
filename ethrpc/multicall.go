package ethrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/ethrpc/jsonrpc"
	"github.com/0xsequence/ethkit/ethrpc/multicall"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
)

var multicallAddresses = []common.Address{
	common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
	common.HexToAddress("0xae96419a81516f063744206d4b5E36f3168280f8"),
}

func (p *Provider) MulticallAddress(ctx context.Context) (bool, common.Address, error) {
	var err error
	for _, address := range multicallAddresses {
		code, err_ := p.CodeAt(ctx, address, nil)
		if err_ != nil {
			if err == nil {
				err = err_
			}
			continue
		}
		if len(code) != 0 {
			return true, address, nil
		}
	}

	return false, common.Address{}, err
}

type MulticallOptions struct {
	SkipMulticallLookup bool
	MulticallAddress    common.Address

	AccountOverrides map[common.Address]OverrideAccount
}

func (p *Provider) Multicall(ctx context.Context, calls []multicall.Call, options ...MulticallOptions) ([]multicall.Multicall3Result, MulticallOptions, error) {
	if len(options) == 0 {
		options = append(options, MulticallOptions{})
	}
	if options[0].MulticallAddress == (common.Address{}) {
		var found bool
		if !options[0].SkipMulticallLookup {
			var err error
			found, options[0].MulticallAddress, err = p.MulticallAddress(ctx)
			if err != nil {
				return nil, options[0], fmt.Errorf("unable to find multicall deployment: %w", err)
			}
			options[0].SkipMulticallLookup = true
		}
		if !found {
			return p.multicallFallback(ctx, calls, options...)
		}
	}

	abi, err := multicall.IMulticall3MetaData.GetAbi()
	if err != nil {
		return nil, options[0], fmt.Errorf("unable to get multicall abi: %w", err)
	}

	var value big.Int
	calls_ := make([]multicall.Multicall3Call3Value, 0, len(calls))
	for _, call := range calls {
		if call.Value == nil {
			call.Value = new(big.Int)
		}
		calls_ = append(calls_, call.Multicall3Call3Value)
		value.Add(&value, call.Value)
	}

	callData, err := abi.Pack("aggregate3Value", calls_)
	if err != nil {
		return nil, options[0], fmt.Errorf("unable to encode aggregate3Value call: %w", err)
	}

	var returnData []byte
	if len(options[0].AccountOverrides) == 0 {
		returnData, err = p.CallContract(
			ctx,
			ethereum.CallMsg{
				To:    &options[0].MulticallAddress,
				Value: &value,
				Data:  callData,
			},
			nil,
		)
	} else {
		returnData, err = p.CallContractWithOverrides(
			ctx,
			ethereum.CallMsg{
				To:    &options[0].MulticallAddress,
				Value: &value,
				Data:  callData,
			},
			nil,
			options[0].AccountOverrides,
		)
	}
	if err != nil {
		return nil, options[0], fmt.Errorf("unable to eth_call multicall contract %v: %w", options[0].MulticallAddress, err)
	}

	var results []multicall.Multicall3Result
	if err := abi.UnpackIntoInterface(&results, "aggregate3Value", returnData); err != nil {
		return nil, options[0], fmt.Errorf("unable to decode aggregate3Value return data: %w", err)
	}
	if len(results) != len(calls) {
		return nil, options[0], fmt.Errorf("%v results for %v calls", len(results), len(calls))
	}

	for i := range calls {
		if len(calls[i].Outputs) != 0 && results[i].Success {
			if err := multicall.UnpackOutputs(results[i].ReturnData, calls[i].Outputs...); err != nil {
				return nil, options[0], fmt.Errorf("unable to decode result %v: %w", i, err)
			}
		}
	}

	return results, options[0], nil
}

func (p *Provider) multicallFallback(ctx context.Context, calls []multicall.Call, options ...MulticallOptions) ([]multicall.Multicall3Result, MulticallOptions, error) {
	if len(options) == 0 {
		options = append(options, MulticallOptions{})
	}

	results := make([]multicall.Multicall3Result, len(calls))
	calls_ := make([]Call, 0, len(calls))
	for i, call := range calls {
		results[i].Success = true
		to := call.Target
		value := call.Value
		if value == nil {
			value = new(big.Int)
		}
		call_ := ethereum.CallMsg{
			To:    &to,
			Value: value,
			Data:  call.CallData,
		}
		if len(options[0].AccountOverrides) == 0 {
			calls_ = append(calls_, CallContract(call_, nil).Into(&results[i].ReturnData))
		} else {
			calls_ = append(calls_, CallContractWithOverrides(call_, nil, options[0].AccountOverrides).Into(&results[i].ReturnData))
		}
	}

	_, err := p.Do(ctx, calls_...)
	if err != nil {
		if err, ok := err.(BatchError); ok {
			for i, err := range err.ErrorMap() {
				if err != nil {
					if calls[i].AllowFailure {
						if revertData, ok := revertDataFromError(err); ok {
							results[i] = multicall.Multicall3Result{ReturnData: revertData}
						} else {
							results[i] = multicall.Multicall3Result{ReturnData: packRevert(err.Error())}
						}
					} else {
						return nil, options[0], fmt.Errorf("call %v failed: %w", i, err)
					}
				}
			}
		} else {
			return nil, options[0], fmt.Errorf("unable to batch call: %w", err)
		}
	}

	for i := range calls {
		if len(calls[i].Outputs) != 0 && results[i].Success {
			if err := multicall.UnpackOutputs(results[i].ReturnData, calls[i].Outputs...); err != nil {
				return nil, options[0], fmt.Errorf("unable to decode result %v: %w", i, err)
			}
		}
	}

	return results, options[0], nil
}

func revertDataFromError(err error) ([]byte, bool) {
	var jsonrpcErr jsonrpc.Error
	if !errors.As(err, &jsonrpcErr) || len(jsonrpcErr.Data) == 0 {
		return nil, false
	}

	var revert hexutil.Bytes
	if json.Unmarshal(jsonrpcErr.Data, &revert) == nil && len(revert) > 0 {
		return revert, true
	}

	var wrapped struct {
		Data   json.RawMessage `json:"data"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(jsonrpcErr.Data, &wrapped) != nil {
		return nil, false
	}
	if json.Unmarshal(wrapped.Data, &revert) == nil && len(revert) > 0 {
		return revert, true
	}
	if json.Unmarshal(wrapped.Result, &revert) == nil && len(revert) > 0 {
		return revert, true
	}

	return nil, false
}

func packRevert(revert string) []byte {
	type_, _ := abi.NewType("string", "", nil)
	errorABI := abi.NewError("Error", abi.Arguments{{Type: type_}})
	data, _ := errorABI.Inputs.Pack(revert)
	return append(errorABI.ID[:4], data...)
}
