package ethtest_test

import (
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/ethtest"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// yes, we even have to test the testutil

var (
	testchain *ethtest.Testchain
)

func init() {
	var err error
	testchain, err = ethtest.NewTestchain()
	if err != nil {
		panic(err)
	}
}

func TestTestchainID(t *testing.T) {
	assert.Equal(t, testchain.ChainID().Uint64(), uint64(1337))
}

func TestContractHelpers(t *testing.T) {
	callmockContract, receipt := testchain.Deploy(t, "CallReceiverMock")
	assert.NotNil(t, callmockContract)
	assert.NotNil(t, receipt)

	// Update contract value on CallReceiver by calling 'testCall' contract function
	receipt, err := ethtest.ContractTransact(
		testchain.MustWallet(2),
		callmockContract.Address, callmockContract.ABI,
		"testCall", big.NewInt(143), ethcoder.MustHexDecode("0x112233"),
	)
	assert.NoError(t, err)
	assert.NotNil(t, receipt)
	assert.Equal(t, types.ReceiptStatusSuccessful, receipt.Status)

	// Query the value ensuring its been updated on-chain
	res, err := ethtest.ContractQuery(testchain.Provider, callmockContract.Address, "lastValA()", "uint256", nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(res))
	ret, ok := res[0].(*big.Int)
	assert.True(t, ok)
	assert.Equal(t, "143", ret.String())

	// Query the value using different method, where we unpack the value
	var result *big.Int
	_, err = ethtest.ContractCall(testchain.Provider, callmockContract.Address, callmockContract.ABI, &result, "lastValA")
	assert.NoError(t, err)
	assert.Equal(t, uint64(143), result.Uint64())
}
