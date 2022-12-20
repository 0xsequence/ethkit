package ethtest

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/ethcontract"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ERC20 helper methods to deploy, mint and transfer between wallets
// using the ERC20Mock contract.

// TODO: perhaps remove use of testing.T and instead just return errors.

type ERC20Mock struct {
	Contract  *ethcontract.Contract
	testchain *Testchain
}

func DeployERC20Mock(t *testing.T, testchain *Testchain) (*ERC20Mock, *types.Receipt) {
	contract, receipt := testchain.Deploy(t, "ERC20Mock")
	return &ERC20Mock{
		Contract:  contract,
		testchain: testchain,
	}, receipt
}

func (c *ERC20Mock) Mint(t *testing.T, wallet *ethwallet.Wallet, amount int64) {
	// Call mockMint on erc20mock contract
	calldata, err := c.Contract.Encode("mockMint", wallet.Address(), big.NewInt(amount))
	assert.NoError(t, err)

	txn, receipt := SendTransactionAndWaitForReceipt(t, wallet, c.Contract.Address, calldata, nil)
	require.NotNil(t, txn)
	require.NotNil(t, receipt)
}

func (c *ERC20Mock) Transfer(t *testing.T, owner *ethwallet.Wallet, to ethkit.Address, amount int64) {
	calldata, err := c.Contract.Encode("transfer", to, big.NewInt(amount))
	require.NoError(t, err)

	txn, receipt := SendTransactionAndWaitForReceipt(t, owner, c.Contract.Address, calldata, nil)
	require.NotNil(t, txn)
	require.NotNil(t, receipt)
}

func (c *ERC20Mock) GetBalance(t *testing.T, account ethkit.Address, expectedAmount int64) {
	provider := c.testchain.Provider

	ret, err := provider.QueryContract(context.Background(), c.Contract.Address.Hex(), "balanceOf(address)", "uint256", []string{account.Hex()})
	require.NoError(t, err)
	require.Equal(t, 1, len(ret))
	require.Equal(t, fmt.Sprintf("%d", expectedAmount), ret[0])
}
