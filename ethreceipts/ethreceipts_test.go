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

func TestReceiptsListenerBasic(t *testing.T) {
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
			ETHValue: ethtest.ETHValue(0.1),
			GasLimit: 80_000,
			Nonce:    big.NewInt(int64(lastNonce + uint64(i))),
		}

		txn, err := wallet.NewTransaction(ctx, txr)
		assert.NoError(t, err)

		txn, _, err = wallet.SendTransaction(ctx, txn)
		assert.NoError(t, err)

		txnHashes = append(txnHashes, txn.Hash())
	}

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

			t.Logf("=> %d :: %s", i, receipt.TxHash.String())
		}(i, txnHash)
	}

	wg.Wait()
}

func TestReceiptsListenerBlast(t *testing.T) {
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
	// Setup wallets
	//

	// create and fund a few wallets to send from
	fromWallets, _ := testchain.DummyWallets(5, 100)
	testchain.FundAddresses(ethtest.WalletAddresses(fromWallets), 10)

	// create a few wallets to send to
	toWallets, _ := testchain.DummyWallets(3, 200)

	// prepare and sign bunch of txns
	values := []*big.Int{}
	for range fromWallets {
		values = append(values, ethtest.ETHValue(0.1))
	}

	_, txns, err := ethtest.PrepareBlastSendTransactions(ctx, fromWallets, ethtest.WalletAddresses(toWallets), values)
	assert.NoError(t, err)

	// send the txns -- these will be async, so we can just blast synchronously
	// and not have to do it in a goroutine
	for _, txn := range txns {
		_, _, err := ethtxn.SendTransaction(ctx, provider, txn)
		assert.NoError(t, err)
	}

	// lets use receipt listener to listen on txns from just one of the wallets
	txnHashes := []common.Hash{
		txns[5].Hash(), txns[2].Hash(), txns[8].Hash(), txns[3].Hash(),
	}

	var wg sync.WaitGroup
	for i, txnHash := range txnHashes {
		wg.Add(1)
		go func(i int, txnHash common.Hash) {
			defer wg.Done()

			receipt, err := receiptsListener.FetchTransactionReceipt(ctx, txnHash)
			assert.NoError(t, err)
			assert.NotNil(t, receipt)
			assert.True(t, receipt.Status == types.ReceiptStatusSuccessful)

			t.Logf("=> %d :: %s", i, receipt.TxHash.String())
		}(i, txnHash)
	}

	wg.Wait()
}

func TestReceiptsListenerFilterFromAddress(t *testing.T) {
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
	// Setup wallets
	//

	// create and fund a few wallets to send from
	fromWallets, _ := testchain.DummyWallets(3, 100)
	testchain.FundAddresses(ethtest.WalletAddresses(fromWallets), 10)

	// create a few wallets to send to
	toWallets, _ := testchain.DummyWallets(3, 200)

	// prepare and sign bunch of txns
	values := []*big.Int{}
	for range fromWallets {
		values = append(values, ethtest.ETHValue(0.1))
	}

	_, txns, err := ethtest.PrepareBlastSendTransactions(ctx, fromWallets, ethtest.WalletAddresses(toWallets), values)
	assert.NoError(t, err)

	_ = txns

	// send the txns -- these will be async, so we can just blast synchronously
	// and not have to do it in a goroutine
	for _, txn := range txns {
		_, _, err := ethtxn.SendTransaction(ctx, provider, txn)
		assert.NoError(t, err)
	}

	// Subscribe to a filter on the listener

	fmt.Println("listening for txns from wallet..", fromWallets[1].Address())

	sub := receiptsListener.Subscribe(ethreceipts.FilterFrom{fromWallets[1].Address()})

	// we can have .Wait(filter) ..
	// which will wait for once event, then it will exit.
	// we could also update GetTransactionReceipt(filter) too, and it will go back in time..
	// cuz we do need to check that too.. the issue is, it would be limited to txn hash for going back in time..

	// num of transactions were expecting until we call it quits
	num := len(values)
	count := 0

loop:
	for {
		select {

		case <-ctx.Done():
			fmt.Println("ctx done")
			break loop

		case <-sub.Done():
			fmt.Println("sub done")
			break loop

		case receipt, ok := <-sub.TransactionReceipt():
			if !ok {
				continue
			}

			fmt.Println("=> got receipt", receipt.Transaction.Hash()) //, "status:", receipt.Status)

			txn := receipt.Transaction

			txnMsg, err := txn.AsMessage(types.NewLondonSigner(txn.ChainId()), nil)
			if err != nil {
				// TODO ..
				panic(err)
			}
			fmt.Println("=> from..", txnMsg.From(), txn.Hash())

			count++
			if num == count {
				sub.Unsubscribe()
			}

		}

	}

	// lets use receipt listener to listen on txns from just one of the wallets
	// txnHashes := []common.Hash{
	// 	txns[5].Hash(), txns[2].Hash(), txns[8].Hash(), txns[3].Hash(),
	// }

	// var wg sync.WaitGroup
	// for i, txnHash := range txnHashes {
	// 	wg.Add(1)
	// 	go func(i int, txnHash common.Hash) {
	// 		defer wg.Done()

	// 		receipt, err := receiptsListener.FetchTransactionReceipt(ctx, txnHash)
	// 		assert.NoError(t, err)
	// 		assert.NotNil(t, receipt)
	// 		assert.True(t, receipt.Status == types.ReceiptStatusSuccessful)

	// 		t.Logf("=> %d :: %s", i, receipt.TxHash.String())
	// 	}(i, txnHash)
	// }

	// wg.Wait()
}
