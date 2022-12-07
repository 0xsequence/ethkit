package ethtest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// parseTestWalletMnemonic parses the wallet mnemonic from ./package.json, the same
// key used to start the test chain server.
func parseTestWalletMnemonic() (string, error) {
	_, filename, _, _ := runtime.Caller(0)
	cwd := filepath.Dir(filename)

	packageJSONFile := filepath.Join(cwd, "./testchain/package.json")
	data, err := os.ReadFile(packageJSONFile)
	if err != nil {
		return "", fmt.Errorf("ParseTestWalletMnemonic, read: %w", err)
	}

	var dict struct {
		Config struct {
			Mnemonic string `json:"mnemonic"`
		} `json:"config"`
	}
	err = json.Unmarshal(data, &dict)
	if err != nil {
		return "", fmt.Errorf("ParseTestWalletMnemonic, unmarshal: %w", err)
	}

	return dict.Config.Mnemonic, nil
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
