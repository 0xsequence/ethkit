package ethreceipts_test

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethreceipts"
	"github.com/0xsequence/ethkit/ethtest"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/goware/logger"
	"github.com/stretchr/testify/assert"
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

func TestReceiptsListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// Setup ReceiptsListener
	//
	provider := testchain.Provider

	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.Logger = log
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 1000

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	go func() {
		err := monitor.Run(ctx)
		if err != nil {
			t.Error(err)
		}
	}()

	receiptsListener, err := ethreceipts.NewReceiptListener(log, provider, monitor)
	assert.NoError(t, err)

	go func() {
		err := receiptsListener.Run(ctx)
		if err != nil {
			t.Error(err)
		}
	}()

	//
	// Setup test wallet
	//
	wallet, _ := testchain.DummyWallet(1)
	testchain.MustFundAddress(wallet.Address())

	numTxns := 10
	lastNonce, _ := wallet.GetNonce(ctx)
	wallet2, _ := testchain.DummyWallet(2)

	txnHashes := []common.Hash{}

	for i := 0; i < numTxns; i++ {
		to := wallet2.Address()

		txr := &ethtxn.TransactionRequest{
			To:       &to,
			ETHValue: ethtest.FromETHInt64(1),
			GasLimit: 80_000,
			Nonce:    big.NewInt(int64(lastNonce + uint64(i))),
		}

		txn, err := wallet.NewTransaction(ctx, txr)
		assert.NoError(t, err)

		txn, _, err = wallet.SendTransaction(ctx, txn)
		assert.NoError(t, err)

		txnHashes = append(txnHashes, txn.Hash())
	}

	// txn, _ := ethtest.SendTransaction(t, wallet, wallet2.Address(), nil, ethtest.FromETHInt64(1))

	// Let's listen for all the txns
	var wg sync.WaitGroup
	for i, txnHash := range txnHashes {
		wg.Add(1)
		go func(i int, txnHash common.Hash) {
			defer wg.Done()

			receipt, err := receiptsListener.FetchTransactionReceipt(ctx, txnHash)
			assert.NoError(t, err)
			assert.NotNil(t, receipt)
			assert.True(t, receipt.Status == types.ReceiptStatusSuccessful)

			fmt.Println("==>", i, "::", receipt.TxHash.String())
		}(i, txnHash)
	}

	wg.Wait()

	//---

	// balance, err := wallet.GetBalance(context.Background())
	// assert.NoError(t, err)
	// fmt.Println("balance?", balance)

	// _, receipt := ethtest.SendTransactionAndWaitForReceipt(t, wallet, wallet2.Address(), nil, ethtest.FromETHInt64(1))

	// assert.NotNil(t, receipt)
	// assert.GreaterOrEqual(t, ethtest.GetBalance(t, wallet2).Uint64(), ethtest.FromETHInt64(1).Uint64())
	// fmt.Println("receipt..?", receipt.TxHash)

	// time.Sleep(5 * time.Second)
}
