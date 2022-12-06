package ethtest

import (
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
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

// func SignAndSend(t *testing.T, wallet *sequence.Wallet, to common.Address, data []byte) error {
// 	stx := &sequence.Transaction{
// 		// DelegateCall:  false,
// 		// RevertOnError: false,
// 		// GasLimit: big.NewInt(800000),
// 		// Value:         big.NewInt(0),
// 		To:   to,
// 		Data: data,
// 	}

// 	return SignAndSendRawTransaction(t, wallet, stx)
// }

// func SignAndSendRawTransaction(t *testing.T, wallet *sequence.Wallet, stx *sequence.Transaction) error {
// 	// Now, we must sign the meta txn
// 	signedTx, err := wallet.SignTransaction(context.Background(), stx)
// 	assert.NoError(t, err)

// 	metaTxnID, _, waitReceipt, err := wallet.SendTransaction(context.Background(), signedTx)
// 	assert.NoError(t, err)
// 	assert.NotEmpty(t, metaTxnID)

// 	receipt, err := waitReceipt(context.Background())
// 	assert.NoError(t, err)
// 	assert.True(t, receipt.Status == types.ReceiptStatusSuccessful)

// 	// TODO: decode the receipt, and lets confirm we have the metaTxnID event in there..
// 	// NOTE: if you start test chain with `make start-test-chain-verbose`, you will see the metaTxnID above
// 	// correctly logged..

// 	return err
// }

// RandomSeed will generate a random seed
func RandomSeed() uint64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Uint64()
}

func fromEther(ether *big.Int) *big.Int {
	oneEth := big.NewInt(10)
	oneEth.Exp(oneEth, big.NewInt(18), nil)

	return ether.Mul(ether, oneEth)
}
