package ethreceipts_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethreceipts"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtest"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

var (
	testchain        *ethtest.Testchain
	testchainOptions ethtest.TestchainOptions

	log *slog.Logger
)

type flakyRoundTripper struct {
	started     time.Time
	rt          http.RoundTripper
	failureRate float32
	failures    uint64
	times       uint64
}

func newFlakyRoundTripper(rt http.RoundTripper, failureRate float32) *flakyRoundTripper {
	return &flakyRoundTripper{
		rt:          rt,
		started:     time.Now(),
		failureRate: failureRate,
	}
}

func (f *flakyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	times := atomic.AddUint64(&f.times, 1)

	// Introduce forced delay
	delay := time.Duration(rand.Intn(200)) * time.Millisecond
	time.Sleep(delay)

	if rand.Float32() < f.failureRate {
		failures := atomic.AddUint64(&f.failures, 1)
		return nil, fmt.Errorf("round trip network error. failed %d times out of %d", failures, times)
	}

	// Proceed with the actual request
	return f.rt.RoundTrip(req)
}

func newProvider(t *testing.T) *ethrpc.Provider {
	provider, err := ethrpc.NewProvider(testchainOptions.NodeURL)
	require.NoError(t, err)

	return provider
}

// Service errors cancel receipt waits and are checked after the worker exits.
func runReceiptTestService(t *testing.T, ctx context.Context, cancel context.CancelFunc, name string, run func(context.Context) error) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		err := run(ctx)
		if ctx.Err() != nil && (err == nil || errors.Is(err, ctx.Err())) {
			done <- nil
			return
		}
		if err == nil {
			err = fmt.Errorf("%s stopped unexpectedly", name)
		} else {
			err = fmt.Errorf("%s: %w", name, err)
		}
		done <- err
		cancel()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Errorf("%s did not stop after cancellation", name)
		}
	})
}

func restoreReceiptTestProvider(t *testing.T, ctx context.Context, cancel context.CancelFunc, provider *ethrpc.Provider) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
		case <-time.After(20 * time.Second):
			provider.SetHTTPClient(newFlakyHTTPClient(0.0))
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func waitForFlakyProviderReceipts(t *testing.T, ctx context.Context, provider *ethrpc.Provider, listener *ethreceipts.ReceiptsListener, sender *ethwallet.Wallet, recipient common.Address) {
	t.Helper()
	nonce, err := sender.GetNonce(ctx)
	require.NoError(t, err)

	txns := make([]*types.Transaction, 50)
	for i := range txns {
		txns[i], err = sender.NewTransaction(ctx, &ethtxn.TransactionRequest{
			To:       &recipient,
			ETHValue: ethtest.ETHValue(0.01),
			GasLimit: 120_000,
			Nonce:    big.NewInt(int64(nonce + uint64(i))),
		})
		require.NoError(t, err)
	}

	g, workerCtx := errgroup.WithContext(ctx)
	for _, txn := range txns {
		ready := make(chan struct{})
		g.Go(func() error {
			sub := listener.Subscribe(ethreceipts.FilterTxnHash(txn.Hash()))
			defer sub.Unsubscribe()
			close(ready)

			select {
			case <-workerCtx.Done():
				return workerCtx.Err()
			case <-sub.Done():
				return fmt.Errorf("subscription closed before receipt for %s", txn.Hash())
			case receipt, ok := <-sub.TransactionReceipt():
				if !ok {
					return fmt.Errorf("receipt channel closed for %s", txn.Hash())
				}
				if receipt.TransactionHash() != txn.Hash() || receipt.Status() != types.ReceiptStatusSuccessful || receipt.Reorged {
					return fmt.Errorf("unexpected receipt for %s: hash=%s status=%d reorged=%t", txn.Hash(), receipt.TransactionHash(), receipt.Status(), receipt.Reorged)
				}
				return nil
			case <-time.After(300 * time.Second):
				return fmt.Errorf("timeout waiting for receipt for %s", txn.Hash())
			}
		})
		g.Go(func() error {
			select {
			case <-workerCtx.Done():
				return workerCtx.Err()
			case <-ready:
			}
			_, _, err := ethtxn.SendTransaction(workerCtx, provider, txn)
			if err != nil {
				return fmt.Errorf("send transaction %s: %w", txn.Hash(), err)
			}
			return nil
		})
	}
	require.NoError(t, g.Wait())
}

func newFlakyHTTPClient(failureRate float32) *http.Client {
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: newFlakyRoundTripper(http.DefaultTransport, failureRate),
	}
}

func init() {
	testchainOptions = ethtest.DefaultTestchainOptions

	var err error
	testchain, err = ethtest.NewTestchain(testchainOptions)
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
	//monitorOptions.Logger = log
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 1000

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

	require.Zero(t, monitor.NumSubscribers())

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
	assert.NoError(t, err)

	runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

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

	workers, workerCtx := errgroup.WithContext(ctx)
	workers.Go(func() error {
		for _, txn := range txns {
			if _, _, err := wallet.SendTransaction(workerCtx, txn); err != nil {
				return fmt.Errorf("send transaction %s: %w", txn.Hash(), err)
			}
		}
		return nil
	})

	for _, txnHash := range txnHashes {
		workers.Go(func() error {
			receipt, err := receiptsListener.FetchTransactionReceipt(workerCtx, txnHash, 7)
			if err != nil {
				return fmt.Errorf("fetch receipt %s: %w", txnHash, err)
			}
			if receipt == nil || receipt.TransactionHash() != txnHash || receipt.Status() != types.ReceiptStatusSuccessful || receipt.Final {
				return fmt.Errorf("unexpected mined receipt for %s: %v", txnHash, receipt)
			}
			return nil
		})
	}
	require.NoError(t, workers.Wait())

	time.Sleep(2 * time.Second)

	// Check subscribers
	require.Zero(t, receiptsListener.NumSubscribers())
	require.Equal(t, 1, monitor.NumSubscribers())

	// Testing exhausted filter after maxWait period is unable to find non-existant txn hash
	receipt, waitFinality, err := receiptsListener.FetchTransactionReceiptWithFinality(ctx, ethkit.Hash{1, 2, 3, 4}, 5)
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

	receipt, waitFinality, err = receiptsListener.FetchTransactionReceiptWithFinality(ctx, txnHashes[0])
	require.NoError(t, err)
	require.NotNil(t, receipt)
	finalReceipt, err = waitFinality(context.Background())
	require.NoError(t, err)
	require.NotNil(t, finalReceipt)
	require.True(t, finalReceipt.Final)

	// wait enough time, so that the fetched receipt will come as finalized right away
	time.Sleep(5 * time.Second)

	receipt, waitFinality, err = receiptsListener.FetchTransactionReceiptWithFinality(ctx, txnHashes[1])
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

	runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
	assert.NoError(t, err)

	runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

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

	workers, workerCtx := errgroup.WithContext(ctx)
	for _, txnHash := range txnHashes {
		workers.Go(func() error {
			receipt, waitFinality, err := receiptsListener.FetchTransactionReceiptWithFinality(workerCtx, txnHash)
			if err != nil {
				return fmt.Errorf("fetch receipt %s: %w", txnHash, err)
			}
			if receipt == nil || receipt.TransactionHash() != txnHash || receipt.Status() != types.ReceiptStatusSuccessful {
				return fmt.Errorf("unexpected receipt for %s: %v", txnHash, receipt)
			}
			finalReceipt, err := waitFinality(workerCtx)
			if err != nil {
				return fmt.Errorf("final receipt %s: %w", txnHash, err)
			}
			if finalReceipt == nil || finalReceipt.TransactionHash() != txnHash || finalReceipt.Status() != types.ReceiptStatusSuccessful || !finalReceipt.Final {
				return fmt.Errorf("unexpected final receipt for %s: %v", txnHash, finalReceipt)
			}
			return nil
		})
	}
	require.NoError(t, workers.Wait())
}

func TestReceiptsListenerFilters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	provider := testchain.Provider
	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 1000

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	require.NoError(t, err)
	runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4
	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
	require.NoError(t, err)
	runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

	fromWallets, err := testchain.DummyWallets(3, 100)
	require.NoError(t, err)
	require.NoError(t, testchain.FundAddresses(ethtest.WalletAddresses(fromWallets), 10))
	toWallets, err := testchain.DummyWallets(3, 200)
	require.NoError(t, err)
	values := []*big.Int{ethtest.ETHValue(0.1), ethtest.ETHValue(0.1), ethtest.ETHValue(0.1)}
	_, txns, err := ethtest.PrepareBlastSendTransactions(ctx, fromWallets, ethtest.WalletAddresses(toWallets), values)
	require.NoError(t, err)

	fromFilter := ethreceipts.FilterFrom(fromWallets[1].Address()).LimitOne(true).ID(1111).MaxWait(0)
	toFilter := ethreceipts.FilterTo(toWallets[1].Address()).ID(1112).MaxWait(0)
	removedFilter := ethreceipts.FilterTxnHash(txns[2].Hash()).ID(2222)
	addedFilter := ethreceipts.FilterTxnHash(txns[4].Hash()).ID(4444)
	sub := receiptsListener.Subscribe(fromFilter, toFilter, removedFilter)
	defer sub.Unsubscribe()

	sub2 := receiptsListener.Subscribe()
	secondFilter := ethreceipts.FilterTxnHash(txns[3].Hash())
	sub2.AddFilter(secondFilter)
	defer sub2.Unsubscribe()

	thirdFilter := ethreceipts.FilterTxnHash(txns[2].Hash()).ID(3333)
	expiringFilter := ethreceipts.FilterFrom(ethkit.Address{4, 2, 4, 2}).ID(8888).MaxWait(4)
	sub3 := receiptsListener.Subscribe(thirdFilter, expiringFilter)
	defer sub3.Unsubscribe()

	// All live filters are installed before any transaction can be mined.
	require.Eventually(t, func() bool {
		return receiptsListener.IsRunning() && monitor.NumSubscribers() == 1
	}, 5*time.Second, 10*time.Millisecond)
	for _, txn := range txns {
		_, _, err := ethtxn.SendTransaction(ctx, provider, txn)
		require.NoError(t, err)
	}

	type delivery struct {
		subscription string
		hash         common.Hash
		filterID     uint64
		final        bool
	}
	expected := make(map[delivery]ethreceipts.FilterQuery)
	expect := func(subscription string, txnIndex int, filter ethreceipts.FilterQuery, states ...bool) {
		for _, final := range states {
			key := delivery{subscription, txns[txnIndex].Hash(), filter.(ethreceipts.Filterer).FilterID(), final}
			expected[key] = filter
		}
	}
	// LimitOne selects the first matching nonce; the recipient filter selects all three senders.
	expect("sub", 3, fromFilter, false)
	for _, i := range []int{1, 4, 7} {
		expect("sub", i, toFilter, false)
	}
	expect("sub", 2, removedFilter, false)
	expect("sub", 4, addedFilter, false, true)
	expect("sub2", 3, secondFilter, false, true)
	expect("sub3", 2, thirdFilter, false, true)

	seen := make(map[delivery]bool)
	added := false
	removed := false
	record := func(subscription string, receipt ethreceipts.Receipt) {
		key := delivery{subscription, receipt.TransactionHash(), receipt.FilterID(), receipt.Final}
		filter, ok := expected[key]
		require.True(t, ok, "unexpected delivery: %+v", key)
		require.False(t, seen[key], "duplicate delivery: %+v", key)
		require.Same(t, filter, receipt.Filter, "wrong filter owner: %+v", key)
		require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status())
		require.False(t, receipt.Reorged)
		require.NotZero(t, receipt.BlockHash())
		seen[key] = true

		if subscription == "sub" && receipt.FilterID() == 2222 {
			// Remove only this owner after its mined event, before it can finalize.
			sub.RemoveFilter(receipt.Filter)
			require.NotContains(t, sub.Filters(), receipt.Filter)
			removed = true
		}
		if subscription == "sub" && receipt.FilterID() == 1112 && receipt.TransactionHash() == txns[4].Hash() {
			// This transaction is now in the monitor cache; registration must find it there.
			sub.AddFilter(addedFilter)
			require.Contains(t, sub.Filters(), addedFilter)
			added = true
		}
	}

	exhausted := expiringFilter.(ethreceipts.Filterer).Exhausted()
	expired := false
	for len(seen) != len(expected) || !expired {
		select {
		case <-ctx.Done():
			missing := make([]delivery, 0)
			for key := range expected {
				if !seen[key] {
					missing = append(missing, key)
				}
			}
			t.Fatalf("missing receipt deliveries: %v; expired=%t; context=%v", missing, expired, ctx.Err())
		case <-sub.Done():
			t.Fatal("subscription closed before expected deliveries")
		case <-sub2.Done():
			t.Fatal("second subscription closed before expected deliveries")
		case <-sub3.Done():
			t.Fatal("third subscription closed before expected deliveries")
		case receipt, ok := <-sub.TransactionReceipt():
			require.True(t, ok, "receipt channel closed before expected deliveries")
			record("sub", receipt)
		case receipt, ok := <-sub2.TransactionReceipt():
			require.True(t, ok, "second receipt channel closed before expected deliveries")
			record("sub2", receipt)
		case receipt, ok := <-sub3.TransactionReceipt():
			require.True(t, ok, "third receipt channel closed before expected deliveries")
			record("sub3", receipt)
		case <-exhausted:
			expired = true
			exhausted = nil
		}
	}

	require.True(t, added, "cached transaction filter was not added")
	require.True(t, removed, "mined transaction filter was not removed")
	require.Eventually(t, func() bool {
		return len(sub.Filters()) == 1 && len(sub2.Filters()) == 0 && len(sub3.Filters()) == 0
	}, 5*time.Second, 10*time.Millisecond)
	require.Same(t, toFilter, sub.Filters()[0])

	// The unbounded subscription queue forwards asynchronously; observe it long enough
	// to reject queued duplicates or finalization of the removed owner.
	quiet := time.NewTimer(2 * time.Second)
	defer quiet.Stop()
	select {
	case <-ctx.Done():
		t.Fatalf("context ended while checking for extra deliveries: %v", ctx.Err())
	case receipt, ok := <-sub.TransactionReceipt():
		require.True(t, ok, "receipt channel closed unexpectedly")
		record("sub", receipt)
	case receipt, ok := <-sub2.TransactionReceipt():
		require.True(t, ok, "second receipt channel closed unexpectedly")
		record("sub2", receipt)
	case receipt, ok := <-sub3.TransactionReceipt():
		require.True(t, ok, "third receipt channel closed unexpectedly")
		record("sub3", receipt)
	case <-quiet.C:
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

	runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10
	listenerOptions.FilterMaxWaitNumBlocks = 4

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
	assert.NoError(t, err)

	runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

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
		ethreceipts.FilterFrom(ethkit.Address{1, 2, 3, 4}).MaxWait(0).ID(8888),

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

	receipt := erc20Mock.Mint(t, wallet, num)
	erc20Receipts = append(erc20Receipts, receipt)
	erc20Mock.GetBalance(t, wallet.Address(), num)

	total := int64(0)
	for i := 0; i < 5; i++ {
		n := int64(40 + i)
		total += n

		receipt := erc20Mock.Transfer(t, wallet, wallet2.Address(), n)
		erc20Receipts = append(erc20Receipts, receipt)
		erc20Mock.GetBalance(t, wallet2.Address(), total)
	}

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

	// Each submitted transaction must have one mined and one final event, even
	// when MaxWait expires before the queued finality threshold is reached.
	expectedHashes := make(map[common.Hash]bool, len(erc20Receipts))
	for _, receipt := range erc20Receipts {
		expectedHashes[receipt.TxHash] = true
	}
	type receiptState struct {
		hash  common.Hash
		final bool
	}
	counts := make(map[receiptState]int)
	for _, receipt := range matchedReceipts {
		require.True(t, expectedHashes[receipt.TransactionHash()], "unexpected transaction %s", receipt.TransactionHash())
		require.Equal(t, uint64(9999), receipt.FilterID())
		require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status())
		require.False(t, receipt.Reorged)
		counts[receiptState{receipt.TransactionHash(), receipt.Final}]++
	}
	for hash := range expectedHashes {
		require.Equal(t, 1, counts[receiptState{hash, false}], "mined delivery for %s", hash)
		require.Equal(t, 1, counts[receiptState{hash, true}], "final delivery for %s", hash)
	}
	require.Equal(t, len(erc20Receipts)*2, matchedCount)
}

func TestFiltersAddDeadlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := testchain.Provider

	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 100

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

	listenerOptions := ethreceipts.DefaultOptions
	listenerOptions.NumBlocksToFinality = 10

	receiptsListener, err := ethreceipts.NewReceiptsListener(log, provider, monitor, listenerOptions)
	assert.NoError(t, err)

	// Don't start the listener's Run() to make registerFiltersCh not be consumed
	// This simulates a slow consumer scenario

	deadlockDetected := make(chan bool, 1)

	go func() {
		// Wait 5 minutes before assuming deadlock
		time.Sleep(300 * time.Second)

		select {
		case deadlockDetected <- true:
		default:
		}
	}()

	// Create many subscribers that will all try to add filters
	var wg sync.WaitGroup

	// First, fill up the registerFiltersCh buffer (capacity 1000)
	sub := receiptsListener.Subscribe()
	for i := 0; i < 1001; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			// This should block on the 1001st call while holding s.mu
			hash := ethkit.Hash{byte(i / 256), byte(i % 256)}
			sub.AddFilter(ethreceipts.FilterTxnHash(hash))
		}(i)
	}

	// Now try to access the subscriber's filters from another goroutine
	// This should deadlock if AddFilter is stuck holding the lock
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)

		// This will try to acquire s.mu.Lock()
		filters := sub.Filters()
		t.Logf("Got %d filters", len(filters))
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Test completed without deadlock")
	case <-deadlockDetected:
		t.Fatal("Deadlock detected - AddFilter blocked while holding lock")
	}
}

func TestFlakyProvider(t *testing.T) {
	t.Run("Wait for txn receipts with a healthy monitor and a healthy provider", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		goodProvider := newProvider(t)

		monitorOptions := ethmonitor.DefaultOptions
		monitorOptions.WithLogs = true

		// monitor running with a good provider
		monitor, err := ethmonitor.NewMonitor(goodProvider, monitorOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

		listenerOptions := ethreceipts.DefaultOptions
		listenerOptions.FilterMaxWaitNumBlocks = 1

		// receipts listener running with a healthy provider initially
		receiptsListener, err := ethreceipts.NewReceiptsListener(log, goodProvider, monitor, listenerOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

		// Wait for services to be ready
		time.Sleep(2 * time.Second)

		walletA, err := testchain.DummyWallet(1)
		require.NoError(t, err)
		testchain.MustFundAddress(walletA.Address())

		walletB, err := testchain.DummyWallet(2)
		require.NoError(t, err)
		waitForFlakyProviderReceipts(t, ctx, goodProvider, receiptsListener, walletA, walletB.Address())
	})

	t.Run("Wait for txn receipts with a flaky monitor and a healthy provider", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var (
			goodProvider  = newProvider(t)
			flakyProvider = newProvider(t)
		)

		monitorOptions := ethmonitor.DefaultOptions
		monitorOptions.WithLogs = true

		// monitor running with a flaky provider
		monitor, err := ethmonitor.NewMonitor(flakyProvider, monitorOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

		listenerOptions := ethreceipts.DefaultOptions
		listenerOptions.FilterMaxWaitNumBlocks = 1

		// receipts listener running with a healthy provider initially
		receiptsListener, err := ethreceipts.NewReceiptsListener(log, goodProvider, monitor, listenerOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

		// Wait for services to be ready
		time.Sleep(2 * time.Second)

		// Replace provider's HTTP client with a flaky one
		t.Logf("Setting provider to flaky state")
		flakyProvider.SetHTTPClient(newFlakyHTTPClient(1.0))

		restoreReceiptTestProvider(t, ctx, cancel, flakyProvider)

		walletA, err := testchain.DummyWallet(1)
		require.NoError(t, err)
		testchain.MustFundAddress(walletA.Address())

		walletB, err := testchain.DummyWallet(2)
		require.NoError(t, err)
		waitForFlakyProviderReceipts(t, ctx, goodProvider, receiptsListener, walletA, walletB.Address())
	})

	t.Run("Wait for txn receipts with a healthy monitor and a flaky provider", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var (
			goodProvider  = newProvider(t)
			flakyProvider = newProvider(t)
		)

		monitorOptions := ethmonitor.DefaultOptions
		monitorOptions.WithLogs = true

		// monitor running with a good provider
		monitor, err := ethmonitor.NewMonitor(goodProvider, monitorOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

		listenerOptions := ethreceipts.DefaultOptions
		listenerOptions.FilterMaxWaitNumBlocks = 1

		// receipts listener running with a healthy provider initially
		receiptsListener, err := ethreceipts.NewReceiptsListener(log, flakyProvider, monitor, listenerOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

		// Wait for services to be ready
		time.Sleep(2 * time.Second)

		// Replace provider's HTTP client with a flaky one
		t.Logf("Setting provider to flaky state")
		flakyProvider.SetHTTPClient(newFlakyHTTPClient(1.0))

		restoreReceiptTestProvider(t, ctx, cancel, flakyProvider)

		walletA, err := testchain.DummyWallet(1)
		require.NoError(t, err)
		testchain.MustFundAddress(walletA.Address())

		walletB, err := testchain.DummyWallet(2)
		require.NoError(t, err)
		waitForFlakyProviderReceipts(t, ctx, goodProvider, receiptsListener, walletA, walletB.Address())
	})

	t.Run("Wait for txn receipts with a flaky monitor and a flaky provider", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var (
			flakyProvider = newProvider(t)
			goodProvider  = newProvider(t) // for sending transactions only
		)

		monitorOptions := ethmonitor.DefaultOptions
		monitorOptions.WithLogs = true

		// monitor running with a good provider
		monitor, err := ethmonitor.NewMonitor(flakyProvider, monitorOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "monitor", monitor.Run)

		listenerOptions := ethreceipts.DefaultOptions
		listenerOptions.FilterMaxWaitNumBlocks = 1

		// receipts listener running with a healthy provider initially
		receiptsListener, err := ethreceipts.NewReceiptsListener(log, flakyProvider, monitor, listenerOptions)
		require.NoError(t, err)

		runReceiptTestService(t, ctx, cancel, "receipts listener", receiptsListener.Run)

		// Wait for services to be ready
		time.Sleep(2 * time.Second)

		// Replace provider's HTTP client with a flaky one
		t.Logf("Setting provider to flaky state")
		flakyProvider.SetHTTPClient(newFlakyHTTPClient(1.0))

		restoreReceiptTestProvider(t, ctx, cancel, flakyProvider)

		walletA, err := testchain.DummyWallet(1)
		require.NoError(t, err)
		testchain.MustFundAddress(walletA.Address())

		walletB, err := testchain.DummyWallet(2)
		require.NoError(t, err)
		waitForFlakyProviderReceipts(t, ctx, goodProvider, receiptsListener, walletA, walletB.Address())
	})
}
