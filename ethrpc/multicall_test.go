package ethrpc_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethrpc/multicall"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

var multicallAddress = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")

func TestMulticall(t *testing.T) {
	ctx := context.Background()

	provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet")
	assert.NoError(t, err)

	height, err := provider.BlockNumber(ctx)
	assert.NoError(t, err)

	var (
		baseFee         *big.Int
		blockHash       common.Hash
		blockNumber     *big.Int
		chainID         *big.Int
		coinbase        common.Address
		blockDifficulty *big.Int
		blockGasLimit   *big.Int
		blockTimestamp  *big.Int
		balance         *big.Int
		lastBlockHash   common.Hash
	)

	results, options, err := provider.Multicall(ctx, []multicall.Call{
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.BaseFee(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &baseFee,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.BlockHash(big.NewInt(int64(height) - 100)),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{bytes32()},
			Outputs:     &blockHash,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.BlockNumber(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &blockNumber,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.ChainID(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &chainID,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.BlockCoinbase(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{address()},
			Outputs:     &coinbase,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.BlockDifficulty(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &blockDifficulty,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.BlockGasLimit(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &blockGasLimit,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.BlockTimestamp(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &blockTimestamp,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.Balance(common.HexToAddress("0xc06145782F31030dB1C40B203bE6B0fD53410B6d")),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &balance,
		},
		{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.LastBlockHash(),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{bytes32()},
			Outputs:     &lastBlockHash,
		},
	})
	assert.NoError(t, err)
	for _, result := range results {
		assert.True(t, result.Success)
	}

	spew.Dump(options)

	fmt.Println("base fee:", baseFee)
	fmt.Printf("block %v: %v\n", height-100, blockHash)
	fmt.Println("block number:", blockNumber)
	fmt.Println("chain id:", chainID)
	fmt.Println("block coinbase:", coinbase)
	fmt.Println("block difficulty:", blockDifficulty)
	fmt.Println("block gas limit:", blockGasLimit)
	fmt.Println("block timestamp:", blockTimestamp)
	fmt.Println("balance:", balance)
	fmt.Println("last block:", lastBlockHash)
}

func TestMulticallBig(t *testing.T) {
	const N = 1023

	ctx := context.Background()

	provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet")
	assert.NoError(t, err)

	var balance *big.Int

	calls := make([]multicall.Call, 0, N)
	for range N {
		calls = append(calls, multicall.Call{
			Multicall3Call3Value: multicall.Multicall3Call3Value{
				Target:       multicallAddress,
				CallData:     multicall.Balance(common.HexToAddress("0xc06145782F31030dB1C40B203bE6B0fD53410B6d")),
				AllowFailure: true,
			},
			ReturnTypes: abi.Arguments{uint256()},
			Outputs:     &balance,
		})
	}

	results, options, err := provider.Multicall(ctx, calls)
	assert.NoError(t, err)
	for _, result := range results {
		assert.True(t, result.Success)
	}

	spew.Dump(options)

	fmt.Println("balance:", balance)
}

func address() abi.Argument {
	type_, _ := abi.NewType("address", "", nil)
	return abi.Argument{Type: type_}
}

func bytes32() abi.Argument {
	type_, _ := abi.NewType("bytes32", "", nil)
	return abi.Argument{Type: type_}
}

func uint256() abi.Argument {
	type_, _ := abi.NewType("uint256", "", nil)
	return abi.Argument{Type: type_}
}
