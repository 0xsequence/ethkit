package ethtest

import (
	"context"
	"fmt"
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

func RandomSeed() uint64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Uint64()
}

func ETHValue(ether float64) *big.Int {
	x := big.NewInt(10)
	x.Exp(x, big.NewInt(15), nil)
	n := big.NewInt(int64(ether * 1000))
	return n.Mul(n, x)
}

func ETHValueBigInt(ether *big.Int) *big.Int {
	oneEth := big.NewInt(10)
	oneEth.Exp(oneEth, big.NewInt(18), nil)
	return ether.Mul(ether, oneEth)
}

func GetBalance(t *testing.T, wallet *ethwallet.Wallet) *big.Int {
	balance, err := wallet.GetBalance(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, balance)
	return balance
}

// DummyAddr returns a dummy address
func DummyAddr() common.Address {
	addr, _ := ethwallet.NewWalletFromRandomEntropy()
	return addr.Address()
}

// DummyPrivateKey returns random private key in hex used with ethwallet
func DummyPrivateKey(seed uint64) string {
	return fmt.Sprintf("%064x", seed)
}

func WalletAddresses(wallets []*ethwallet.Wallet) []common.Address {
	addrs := []common.Address{}
	for _, w := range wallets {
		addrs = append(addrs, w.Address())
	}
	return addrs
}

func SendTransaction(t *testing.T, wallet *ethwallet.Wallet, to common.Address, data []byte, ethValue *big.Int) (*types.Transaction, ethtxn.WaitReceipt) {
	txr := &ethtxn.TransactionRequest{
		To:       &to,
		Data:     data,
		ETHValue: ethValue,
		GasLimit: 300_000,
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

func PrepareBlastSendTransactions(ctx context.Context, fromWallets []*ethwallet.Wallet, toAddresses []common.Address, values []*big.Int) ([]*ethtxn.TransactionRequest, []*types.Transaction, error) {
	if len(fromWallets) != len(values) {
		return nil, nil, fmt.Errorf("arrays 'fromWallets' and 'values' must be the same length")
	}

	lastNonces := []uint64{}
	for _, w := range fromWallets {
		nonce, err := w.GetNonce(ctx)
		if err != nil {
			return nil, nil, err
		}
		lastNonces = append(lastNonces, nonce)
	}

	txrs := []*ethtxn.TransactionRequest{}
	txns := []*types.Transaction{}

	for i, w := range fromWallets {
		for k, to := range toAddresses {
			nonce := lastNonces[i] + uint64(k)

			txr := &ethtxn.TransactionRequest{
				From:     w.Address(),
				To:       &to,
				ETHValue: ETHValue(0.1),
				GasLimit: 120_000,
				Nonce:    big.NewInt(int64(nonce)),
			}
			txrs = append(txrs, txr)

			txn, err := w.NewTransaction(ctx, txr)
			if err != nil {
				return nil, nil, err
			}
			txns = append(txns, txn)
		}
	}

	return txrs, txns, nil
}
