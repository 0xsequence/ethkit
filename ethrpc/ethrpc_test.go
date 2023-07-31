package ethrpc_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtest"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/goware/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testchain *ethtest.Testchain
	log       logger.Logger
)

func init() {
	var err error
	testchain, err = ethtest.NewTestchain()
	if err != nil {
		panic(err)
	}

	log = logger.NewLogger(logger.LogLevel_INFO)
}

// Test fetching the chain id to ensure we can connect to the testchain properly
func TestTestchainID(t *testing.T) {
	assert.Equal(t, testchain.ChainID().Uint64(), uint64(1337))
}

func TestERC20MintAndTransfer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wallet, err := testchain.DummyWallet(500)
	require.NoError(t, err)
	require.NoError(t, testchain.FundAddress(wallet.Address()))

	provider := testchain.Provider

	// deploy ERC20Mock contract
	erc20Mock, _ := testchain.Deploy(t, "ERC20Mock")

	// Call mockMint on erc20mock contract
	calldata, err := erc20Mock.Encode("mockMint", wallet.Address(), big.NewInt(2000))
	assert.NoError(t, err)

	txn, receipt := ethtest.SendTransactionAndWaitForReceipt(t, wallet, erc20Mock.Address, calldata, nil)
	require.NotNil(t, txn)
	require.NotNil(t, receipt)

	// Query erc20Mock balance to confirm
	res, err := provider.ContractQuery(ctx, erc20Mock.Address.Hex(), "balanceOf(address)", "uint256", []string{wallet.Address().Hex()})
	require.NoError(t, err)
	require.Equal(t, 1, len(res))
	ret, ok := res[0].(*big.Int)
	require.True(t, ok)
	require.Equal(t, "2000", ret.String())

	// Transfer token to another wallet
	wallet2, _ := testchain.DummyWallet(600)
	require.NoError(t, testchain.FundAddress(wallet2.Address()))

	calldata, err = erc20Mock.Encode("transfer", wallet2.Address(), big.NewInt(42))
	require.NoError(t, err)

	txn, receipt = ethtest.SendTransactionAndWaitForReceipt(t, wallet, erc20Mock.Address, calldata, nil)
	require.NotNil(t, txn)
	require.NotNil(t, receipt)
	res, err = provider.ContractQuery(ctx, erc20Mock.Address.Hex(), "balanceOf(address)", "uint256", []string{wallet2.Address().Hex()})
	require.NoError(t, err)
	require.Equal(t, 1, len(res))
	ret, ok = res[0].(*big.Int)
	require.True(t, ok)
	require.Equal(t, "42", ret.String())
}

func TestBlockByNumber(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	{
		block, err := p.BlockByNumber(context.Background(), big.NewInt(1_000_000))
		require.NoError(t, err)
		require.NotNil(t, block)
		require.Equal(t, uint64(1_000_000), block.NumberU64())
	}
	{
		block, err := p.BlockByNumber(context.Background(), big.NewInt(100_000_000))
		require.Error(t, err)
		require.True(t, errors.Is(err, ethereum.NotFound))
		require.True(t, errors.Is(err, ethrpc.ErrNotFound))
		require.Nil(t, block)
	}
}

// func TestBlockRange(t *testing.T) {
// 	p, err := ethrpc.NewProvider("https://dev-nodes.sequence.app/optimism")
// 	require.NoError(t, err)

// 	{
// 		block, err := p.BlockRange(context.Background(), big.NewInt(1_000_000), big.NewInt(1_000_100))
// 		require.NoError(t, err)
// 		require.NotNil(t, block)
// 	}
// }

func ExampleBatchCall() {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	if err != nil {
		panic(err)
	}

	var (
		chainID  *big.Int
		header   *types.Header
		errBlock *types.Block
	)
	err = p.Do(
		context.Background(),
		ethrpc.ChainID().Into(&chainID),
		ethrpc.HeaderByNumber(big.NewInt(38470000)).Into(&header),
		ethrpc.BlockByHash(common.BytesToHash([]byte("a1b2c3"))).Into(&errBlock),
	)
	fmt.Printf("polygon ID: %s\n", chainID.String())
	if err != nil {
		if batchErr, ok := err.(ethrpc.BatchError); ok {
			for i, err := range batchErr {
				fmt.Printf("error at %d: %s\n", i, err)
			}
		}
	}
	// Output:
	// polygon ID: 137
	// error at 2: not found
}

func TestETHRPC(t *testing.T) {
	t.Run("Single", func(t *testing.T) {
		p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
		require.NoError(t, err)

		chainID, err := p.ChainID(context.Background())
		require.NoError(t, err)
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(137), chainID.Uint64())
	})

	t.Run("Batch", func(t *testing.T) {
		p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
		require.NoError(t, err)

		var (
			chainID     *big.Int
			blockNumber uint64
			header      *types.Header
		)
		err = p.Do(
			context.Background(),
			ethrpc.ChainID().Into(&chainID),
			ethrpc.BlockNumber().Into(&blockNumber),
			ethrpc.HeaderByNumber(big.NewInt(38470000)).Into(&header),
		)
		require.NoError(t, err)
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(137), chainID.Uint64())
		assert.Greater(t, blockNumber, uint64(0))
		assert.Equal(t, uint64(38470000), header.Number.Uint64())
	})
}
