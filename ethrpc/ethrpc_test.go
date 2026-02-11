package ethrpc_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtest"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testchain *ethtest.Testchain
	log       *slog.Logger
)

func init() {
	var err error
	testchain, err = ethtest.NewTestchain()
	if err != nil {
		panic(err)
	}

	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
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
	ret, err := provider.ContractQuery(ctx, erc20Mock.Address.Hex(), "balanceOf(address)", "uint256", []string{wallet.Address().Hex()})
	require.NoError(t, err)
	require.Equal(t, 1, len(ret))
	require.Equal(t, "2000", ret[0])

	// Transfer token to another wallet
	wallet2, _ := testchain.DummyWallet(600)
	require.NoError(t, testchain.FundAddress(wallet2.Address()))

	calldata, err = erc20Mock.Encode("transfer", wallet2.Address(), big.NewInt(42))
	require.NoError(t, err)

	txn, receipt = ethtest.SendTransactionAndWaitForReceipt(t, wallet, erc20Mock.Address, calldata, nil)
	require.NotNil(t, txn)
	require.NotNil(t, receipt)

	ret, err = provider.ContractQuery(ctx, erc20Mock.Address.Hex(), "balanceOf(address)", "uint256", []string{wallet2.Address().Hex()})
	require.NoError(t, err)
	require.Equal(t, 1, len(ret))
	require.Equal(t, "42", ret[0])
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

func TestBlocskByNumbers(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	{
		blocks, err := p.BlocksByNumbers(context.Background(), []*big.Int{
			big.NewInt(1_000_000),
			big.NewInt(1_000_001),
		})
		require.NoError(t, err)
		require.NotNil(t, blocks)
		require.Len(t, blocks, 2)
		require.Equal(t, uint64(1_000_000), blocks[0].NumberU64())
		require.Equal(t, uint64(1_000_001), blocks[1].NumberU64())
	}
}

func TestBlocksByNumberRange(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	{
		blocks, err := p.BlocksByNumberRange(context.Background(),
			big.NewInt(1_000_000),
			big.NewInt(1_000_005),
		)
		require.NoError(t, err)
		require.NotNil(t, blocks)
		require.Len(t, blocks, 5)
		require.Equal(t, uint64(1_000_000), blocks[0].NumberU64())
		require.Equal(t, uint64(1_000_001), blocks[1].NumberU64())
		require.Equal(t, uint64(1_000_002), blocks[2].NumberU64())
		require.Equal(t, uint64(1_000_003), blocks[3].NumberU64())
		require.Equal(t, uint64(1_000_004), blocks[4].NumberU64())
	}
}

func TestHeadsByNumbers(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	{
		headers, err := p.HeadersByNumbers(context.Background(), []*big.Int{big.NewInt(1_000_000), big.NewInt(1_000_001)})
		require.NoError(t, err)
		require.NotNil(t, headers)
		require.Len(t, headers, 2)
		require.Equal(t, uint64(1_000_000), headers[0].Number.Uint64())
		require.Equal(t, uint64(1_000_001), headers[1].Number.Uint64())
	}
}

func TestHeadsByNumberRange(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	{
		headers, err := p.HeadersByNumberRange(context.Background(), big.NewInt(1_000_000), big.NewInt(1_000_005))
		require.NoError(t, err)
		require.NotNil(t, headers)
		require.Len(t, headers, 5)
		require.Equal(t, uint64(1_000_000), headers[0].Number.Uint64())
		require.Equal(t, uint64(1_000_001), headers[1].Number.Uint64())
		require.Equal(t, uint64(1_000_002), headers[2].Number.Uint64())
		require.Equal(t, uint64(1_000_003), headers[3].Number.Uint64())
		require.Equal(t, uint64(1_000_004), headers[4].Number.Uint64())
	}
}

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
	_, err = p.Do(
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
		_, err = p.Do(
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

func TestRaw(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	// p, err := ethrpc.NewProvider("http://localhost:8887/polygon")
	require.NoError(t, err)

	// block exists
	{
		ctx := context.Background()
		payload, err := p.RawBlockByNumber(ctx, big.NewInt(38470000))

		require.NoError(t, err)
		require.NotEmpty(t, payload)

		var block *types.Block
		err = ethrpc.IntoBlock(payload, &block, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(38470000), block.NumberU64())
	}

	// block does not exist
	{
		ctx := context.Background()
		n, _ := big.NewInt(0).SetString("ffffffffffff", 16)
		payload, err := p.RawBlockByNumber(ctx, n)

		require.True(t, errors.Is(err, ethereum.NotFound))
		require.Empty(t, payload)
	}
}

// todo: uncomment when those call are available on node-gateway
/*func TestDebugTraceBlockByNumber(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	ctx := context.Background()
	payload, err := p.DebugTraceBlockByNumber(ctx, big.NewInt(38470000))

	require.NoError(t, err)
	require.NotEmpty(t, payload)
}

func TestDebugTraceBlockByHash(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	ctx := context.Background()
	payload, err := p.DebugTraceBlockByHash(ctx, common.HexToHash("0x4ab1c5d23e74dc9ec309c5e9c44dc5cf4d3739085747d35ab7d7a76983e1d1f0"))

	require.NoError(t, err)
	require.NotEmpty(t, payload)
}

func TestDebugTraceTransaction(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	ctx := context.Background()
	payload, err := p.DebugTraceTransaction(ctx, common.HexToHash("0x971329c0a49974ba20f7cafb1404610ea712aabbd164a66314050d62a1829eb5"))

	require.NoError(t, err)
	require.NotEmpty(t, payload)
}*/

func TestDoRequest_SeqChainHealth(t *testing.T) {
	p, err := ethrpc.NewProvider("https://dev-nodes.sequence.app/polygon")
	require.NoError(t, err)

	result, err := ethrpc.DoRequest(context.Background(), p, "seq_chainHealth")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Expect isHealthy to be true
	isHealthy, ok := result["isHealthy"].(bool)
	require.True(t, ok, "expected isHealthy to be a bool")
	assert.True(t, isHealthy, "expected isHealthy to be true")

	// Expect expiresAt to be present as a string (RFC3339 timestamp)
	expiresAt, ok := result["expiresAt"].(string)
	require.True(t, ok, "expected expiresAt to be a string")
	assert.NotEmpty(t, expiresAt)
}

func TestFetchBlockWithInvalidVRS(t *testing.T) {
	url := "https://rpc.telos.net"
	// url := "https://node.mainnet.etherlink.com"

	p, err := ethrpc.NewProvider(url)
	require.NoError(t, err)

	block, err := p.BlockByNumber(context.Background(), big.NewInt(373117692))
	require.NoError(t, err)
	require.NotNil(t, block)

	for _, tx := range block.Transactions() {
		require.Equal(t, uint8(0), tx.Type())
		require.Equal(t, big.NewInt(0), tx.GasFeeCap())
		require.Equal(t, big.NewInt(0), tx.GasPrice())
	}
}
