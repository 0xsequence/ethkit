package ethreceipts_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethreceipts"
	"github.com/0xsequence/ethkit/ethtest"
	"github.com/0xsequence/ethkit/ethtxn"
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

	// log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	// 	Level: slog.LevelInfo,
	// }))
	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// Test fetching the chain id to ensure we can connect to the testchain properly
func TestTestchainID(t *testing.T) {
	assert.Equal(t, testchain.ChainID().Uint64(), uint64(1337))
}

func TestFetchTransactionReceiptBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// Setup ReceiptsListener
	//
	provider := testchain.Provider

	monitorOptions := ethmonitor.DefaultOptions
	// monitorOptions.Logger = log
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

	require.Zero(t, monitor.NumSubscribers())

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
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

	// numTxns := 1
	// numTxns := 2
	// numTxns := 10
	numTxns := 40
	lastNonce, err := wallet.GetNonce(ctx)
	require.NoError(t, err)
	wallet2, _ := testchain.DummyWallet(2)

	txns := []*types.Transaction{}
	txnHashes := []common.Hash{}

	for i := 0; i < numTxns; i++ {
		to := wallet2.Address()
		txr := &ethtxn.TransactionRequest{
			To:       &to,
			ETHValue: ethtest.ETHValue(0.1),
			GasLimit: 120_000,
			Nonce:    big.NewInt(int64(lastNonce + uint64(i))),
		}

		txn, err := wallet.NewTransaction(ctx, txr)
		require.NoError(t, err)

		txns = append(txns, txn)
		txnHashes = append(txnHashes, txn.Hash())
	}

	// dispatch txns in the background
	go func() {
		for _, txn := range txns {
			_, _, err = wallet.SendTransaction(ctx, txn)
			require.NoError(t, err)
			// time.Sleep(500 * time.Millisecond)
		}
	}()

	// ensure all txns made it
	// delay processing if we want to make sure SearchCache works
	// time.Sleep(2 * time.Second)
	// for _, txnHash := range txnHashes {
	// 	receipt, err := provider.TransactionReceipt(context.Background(), txnHash)
	// 	require.NoError(t, err)
	// 	require.True(t, receipt.Status == 1)
	// }

	// Let's listen for all the txns
	var wg sync.WaitGroup
	for i, txnHash := range txnHashes {
		wg.Add(1)
		go func(i int, txnHash common.Hash) {
			defer wg.Done()

			receipt, waitFinality, err := receiptsListener.FetchTransactionReceipt(ctx, txnHash, 7)
			require.NoError(t, err)
			require.NotNil(t, receipt)
			require.True(t, receipt.Status() == types.ReceiptStatusSuccessful)
			require.False(t, receipt.Final)

			t.Logf("=> MINED %d :: %s", i, receipt.TransactionHash().String())

			_ = waitFinality
			finalReceipt, err := waitFinality(context.Background())
			require.NoError(t, err)
			require.NotNil(t, finalReceipt)
			require.True(t, finalReceipt.Status() == types.ReceiptStatusSuccessful)
			require.True(t, finalReceipt.Final)

			t.Logf("=> FINAL %d :: %s", i, receipt.TransactionHash().String())
		}(i, txnHash)
	}
	wg.Wait()

	time.Sleep(2 * time.Second)

	// Check subscribers
	require.Zero(t, receiptsListener.NumSubscribers())
	require.Equal(t, 1, monitor.NumSubscribers())

	// Testing exhausted filter after maxWait period is unable to find non-existant txn hash
	receipt, waitFinality, err := receiptsListener.FetchTransactionReceipt(ctx, ethkit.Hash{1, 2, 3, 4}, 5)
	require.Error(t, err)
	require.True(t, errors.Is(err, ethreceipts.ErrFilterExhausted))
	require.Nil(t, receipt)
	finalReceipt, err := waitFinality(context.Background())
	require.Error(t, err)
	require.True(t, errors.Is(err, ethreceipts.ErrFilterExhausted), "received error %v", err)
	require.Nil(t, finalReceipt)

	// Check subscribers
	time.Sleep(1 * time.Second)
	require.Zero(t, receiptsListener.NumSubscribers())
	require.Equal(t, 1, monitor.NumSubscribers())

	// Clear monitor retention, and lets try to find an old txnHash which is on the chain
	// and will force to use SearchOnChain method.
	monitor.PurgeHistory()
	receiptsListener.PurgeHistory()

	receipt, waitFinality, err = receiptsListener.FetchTransactionReceipt(ctx, txnHashes[0])
	require.NoError(t, err)
	require.NotNil(t, receipt)
	finalReceipt, err = waitFinality(context.Background())
	require.NoError(t, err)
	require.NotNil(t, finalReceipt)
	require.True(t, finalReceipt.Final)

	// wait enough time, so that the fetched receipt will come as finalized right away
	time.Sleep(5 * time.Second)

	receipt, waitFinality, err = receiptsListener.FetchTransactionReceipt(ctx, txnHashes[1])
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.True(t, receipt.Final)
	finalReceipt, err = waitFinality(context.Background())
	require.NoError(t, err)
	require.NotNil(t, finalReceipt)
	require.True(t, finalReceipt.Final)

	// Check subscribers
	time.Sleep(1 * time.Second)
	require.Zero(t, receiptsListener.NumSubscribers())
	require.Equal(t, 1, monitor.NumSubscribers())
}

func TestFetchTransactionReceiptBlast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// Setup ReceiptsListener
	//
	provider := testchain.Provider

	monitorOptions := ethmonitor.DefaultOptions
	// monitorOptions.Logger = log
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

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
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

	var count uint64

	var wg sync.WaitGroup
	for i, txnHash := range txnHashes {
		wg.Add(1)
		go func(i int, txnHash common.Hash) {
			defer wg.Done()

			receipt, receiptFinality, err := receiptsListener.FetchTransactionReceipt(ctx, txnHash)
			assert.NoError(t, err)
			assert.NotNil(t, receipt)
			assert.True(t, receipt.Status() == types.ReceiptStatusSuccessful)

			finalReceipt, err := receiptFinality(context.Background())
			require.NoError(t, err)
			require.True(t, finalReceipt.Status() == types.ReceiptStatusSuccessful)

			t.Logf("=> %d :: %s", i, receipt.TransactionHash().String())

			atomic.AddUint64(&count, 1)
		}(i, txnHash)
	}
	wg.Wait()

	require.Equal(t, int(count), len(txnHashes))
}

func TestReceiptsListenerFilters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// Setup ReceiptsListener
	//
	provider := testchain.Provider

	monitorOptions := ethmonitor.DefaultOptions
	// monitorOptions.Logger = log
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

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
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

	// send the txns -- these will be async, so we can just blast synchronously
	// and not have to do it in a goroutine
	for _, txn := range txns {
		_, _, err := ethtxn.SendTransaction(ctx, provider, txn)
		assert.NoError(t, err)
	}

	//
	// Subscribe to a filter on the receipt listener
	//
	fmt.Println("listening for txns..")

	sub := receiptsListener.Subscribe(
		ethreceipts.FilterFrom(fromWallets[1].Address()).LimitOne(true),
		ethreceipts.FilterTo(toWallets[1].Address()),
		ethreceipts.FilterTxnHash(txns[2].Hash()).ID(2222), //.Finalize(true) is set by default for FilterTxnHash
	)

	sub2 := receiptsListener.Subscribe()
	sub2.AddFilter(ethreceipts.FilterTxnHash(txns[3].Hash()))

	sub3 := receiptsListener.Subscribe(
		ethreceipts.FilterTxnHash(txns[2].Hash()).ID(3333),

		// will end up not being found and timeout after MaxWait
		ethreceipts.FilterFrom(ethkit.Address{4, 2, 4, 2}).MaxWait(4),
	)

	go func() {
		time.Sleep(5 * time.Second)
		fmt.Println("==> delaying to find", txns[4].Hash().String())
		sub.AddFilter(ethreceipts.FilterTxnHash(txns[4].Hash()).ID(4444))
	}()

	go func() {
		for r := range sub2.TransactionReceipt() {
			fmt.Println("sub2, got receipt", r.TransactionHash(), "final?", r.Final)
		}
	}()

	go func() {
		for r := range sub3.TransactionReceipt() {
			fmt.Println("sub3, got receipt", r.TransactionHash(), "final?", r.Final, "id?", r.FilterID()) //, "maxWait hit?", r.Filter.IsExpired())
		}
	}()

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

			fmt.Println("=> sub, got receipt", receipt.TransactionHash(), "final?", receipt.Final, "id?", receipt.FilterID(), "status?", receipt.Status())

			// txn := receipt.Transaction
			// txnMsg := receipt.Message

			fmt.Println("=> filter matched!", receipt.From(), receipt.TransactionHash())
			fmt.Println("=> receipt status?", receipt.Status())

			fmt.Println("==> len filters", len(sub.Filters()))
			if receipt.TransactionHash() == txns[2].Hash() {
				sub.RemoveFilter(receipt.Filter)
			}
			fmt.Println("==> len filters", len(sub.Filters()))

			fmt.Println("")

		// expecting to be finished with listening for events after a few seconds
		case <-time.After(15 * time.Second):
			sub.Unsubscribe()

		}
	}
}

func TestReceiptsListenerERC20(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	//
	// Setup wallets and deploy erc20mock contract
	//
	wallet, _ := testchain.DummyWallet(1)
	wallet2, _ := testchain.DummyWallet(2)
	testchain.FundWallets(10, wallet, wallet2)

	erc20Mock, _ := ethtest.DeployERC20Mock(t, testchain)

	//
	// Setup ReceiptsListener
	//
	provider := testchain.Provider

	monitorOptions := ethmonitor.DefaultOptions
	// monitorOptions.Logger = log
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 1000
	monitorOptions.PollingInterval = 1000 * time.Millisecond

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	go func() {
		err := monitor.Run(ctx)
		if err != nil {
			t.Error(err)
		}
	}()

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
	assert.NoError(t, err)

	go func() {
		err := receiptsListener.Run(ctx)
		if err != nil {
			t.Error(err)
		}
	}()

	//
	// Subscribe to a filter on the receipt listener
	//
	fmt.Println("listening for txns..")

	erc20TransferTopic, err := erc20Mock.Contract.EventTopicHash("Transfer")
	require.NoError(t, err)
	_ = erc20TransferTopic

	sub := receiptsListener.Subscribe(
		ethreceipts.FilterLogTopic(erc20TransferTopic).Finalize(true).ID(9999).MaxWait(3),

		// won't be found..
		ethreceipts.FilterFrom(ethkit.Address{}).MaxWait(0).ID(8888),

		// ethreceipts.FilterLogs(func(logs []*types.Log) bool {
		// 	for _, log := range logs {
		// 		if log.Address == erc20Mock.Contract.Address {
		// 			return true
		// 		}
		// 		if log.Topics[0] == erc20TransferTopic {
		// 			return true
		// 		}

		// 		// event := ethabi.DecodeERC20Log(log)
		// 		// if event.From == "XXX"
		// 	}
		// 	return false
		// }),
	)

	//
	// Send some erc20 tokens
	//
	num := int64(2000)

	erc20Receipts := make([]*types.Receipt, 0)
	var erc20ReceiptsMu sync.Mutex

	receipt := erc20Mock.Mint(t, wallet, num)
	erc20Receipts = append(erc20Receipts, receipt)
	erc20Mock.GetBalance(t, wallet.Address(), num)

	go func() {
		total := int64(0)
		for i := 0; i < 5; i++ {
			n := int64(40 + i)
			total += n

			erc20ReceiptsMu.Lock()
			receipt := erc20Mock.Transfer(t, wallet, wallet2.Address(), n)
			erc20Receipts = append(erc20Receipts, receipt)
			erc20ReceiptsMu.Unlock()

			erc20Mock.GetBalance(t, wallet2.Address(), total)
		}
	}()

	//
	// Listener loop
	//
	matchedCount := 0
	matchedReceipts := make([]ethreceipts.Receipt, 0)

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

			matchedCount += 1
			matchedReceipts = append(matchedReceipts, receipt)

			fmt.Println("=> sub, got receipt", receipt.TransactionHash(), "final?", receipt.Final, "id?", receipt.FilterID(), "status?", receipt.Status())

			// txn := receipt.Transaction
			// txnMsg := receipt.Message

			fmt.Println("=> filter matched!", receipt.From(), receipt.TransactionHash())
			fmt.Println("=> receipt status?", receipt.Status())

			fmt.Println("")

		// expecting to be finished with listening for events after a few seconds
		case <-time.After(25 * time.Second):
			// NOTE: this should return 1 as there is a filter above with nolimit
			fmt.Println("number of filters still remaining:", len(sub.Filters()))
			sub.Unsubscribe()
		}
	}

	// NOTE: expecting receipts twice. Once on mine, once on finalize.
	for _, mr := range matchedReceipts {
		found := false
		for _, r := range erc20Receipts {
			if mr.TransactionHash() == r.TxHash {
				found = true
			}
		}
		assert.True(t, found, "looking for matched receipt %s", mr.TransactionHash().String())
	}

	require.Equal(t, matchedCount, len(erc20Receipts)*2)
}
