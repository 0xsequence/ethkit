package ethtest

import (
	"context"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

func SendTransaction(t *testing.T, wallet *ethwallet.Wallet, to common.Address, data []byte, ethValue *big.Int) (*types.Transaction, ethtxn.WaitReceipt) {
	txr := &ethtxn.TransactionRequest{
		To:       &to,
		Data:     data,
		ETHValue: ethValue,
		GasLimit: 200_000,
	}

	txn, err := wallet.NewTransaction(context.Background(), txr)
	assert.NoError(t, err)

	txn, waitReceipt, err := wallet.SendTransaction(context.Background(), txn)
	assert.NoError(t, err)

	return txn, waitReceipt
}

func SendTransactionAndWaitForReceipt(t *testing.T, wallet *ethwallet.Wallet, to common.Address, data []byte, ethValue *big.Int) (*types.Transaction, *types.Receipt) {
	txn, waitReceipt := SendTransaction(t, wallet, to, data, ethValue)

	receipt, err := waitReceipt(context.Background())
	assert.NoError(t, err)
	assert.True(t, receipt.Status == types.ReceiptStatusSuccessful)

	return txn, receipt
}

// RandomSeed will generate a random seed
func RandomSeed() uint64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Uint64()
}

func FromETH(ether *big.Int) *big.Int {
	oneEth := big.NewInt(10)
	oneEth.Exp(oneEth, big.NewInt(18), nil)
	return ether.Mul(ether, oneEth)
}

func FromETHInt64(ether int64) *big.Int {
	return FromETH(big.NewInt(ether))
}

func GetBalance(t *testing.T, wallet *ethwallet.Wallet) *big.Int {
	balance, err := wallet.GetBalance(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, balance)
	return balance
}
