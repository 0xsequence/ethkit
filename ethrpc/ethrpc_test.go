package ethrpc_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethtest"
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

	wallet, err := testchain.DummyWallet(1)
	require.NoError(t, err)

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
	ret, err := provider.QueryContract(ctx, erc20Mock.Address.Hex(), "balanceOf(address)", "uint256", []string{wallet.Address().Hex()})
	require.NoError(t, err)
	require.Equal(t, 1, len(ret))
	require.Equal(t, "2000", ret[0])

	// Transfer token to another wallet
	wallet2, _ := testchain.DummyWallet(2)

	calldata, err = erc20Mock.Encode("transfer", wallet2.Address(), big.NewInt(42))
	require.NoError(t, err)

	txn, receipt = ethtest.SendTransactionAndWaitForReceipt(t, wallet, erc20Mock.Address, calldata, nil)
	require.NotNil(t, txn)
	require.NotNil(t, receipt)

	ret, err = provider.QueryContract(ctx, erc20Mock.Address.Hex(), "balanceOf(address)", "uint256", []string{wallet2.Address().Hex()})
	require.NoError(t, err)
	require.Equal(t, 1, len(ret))
	require.Equal(t, "42", ret[0])
}
