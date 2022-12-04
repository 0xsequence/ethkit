package ethreceipts_test

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethreceipts"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/go-sequence/testutil"
	"github.com/go-chi/httplog"
	"github.com/stretchr/testify/assert"
)

var testChain *testutil.TestChain

// func TestReceiptsBasic(t *testing.T) {
// 	provider, err := ethrpc.NewProvider("http://localhost:8545")
// 	assert.NoError(t, err)

// 	// Initializing Monitor
// 	monitorOptions := ethmonitor.DefaultOptions

// 	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
// 	assert.NoError(t, err)

// 	go func(t *testing.T) {
// 		err := monitor.Run(context.Background())
// 		if err != nil {
// 			panic(err)
// 		}
// 	}(t)
// 	defer monitor.Stop()

// 	logger := httplog.NewLogger("ethReceipts_test")

// 	receipts, err := ethreceipts.NewReceipts(logger, provider, monitor)
// 	assert.NoError(t, err)

// 	go func(t *testing.T) {
// 		err := receipts.Run(context.Background())
// 		if err != nil {
// 			panic(err)
// 		}
// 	}(t)
// 	defer receipts.Stop()

// 	// Ensure test-chain is running
// 	testChain, err = testutil.NewTestChain()
// 	if err != nil {
// 		log.Fatal(fmt.Errorf("NewTestChain failed: %w", err))
// 	}

// 	wallet, err := testChain.Wallet()
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	toAddress := testutil.DummyAddr()
// 	signed, err := wallet.NewTransaction(context.Background(), &ethtxn.TransactionRequest{
// 		From: wallet.Address(),
// 		To:   &toAddress,
// 	})
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	assert.NoError(t, err)

// 	//
// 	time.Sleep(5 * time.Second)

// 	txn, _, err := wallet.SendTransaction(context.Background(), signed)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	assert.NoError(t, err)

// 	receipt, err := receipts.GetTransactionReceipt(context.Background(), txn.Hash())
// 	assert.NoError(t, err)

// 	assert.True(t, receipt.Status == types.ReceiptStatusSuccessful, "Transaction couldn't found or failed.")
// }

func TestReceiptsFinality(t *testing.T) {
	provider, err := ethrpc.NewProvider("http://localhost:8545")
	assert.NoError(t, err)

	// Initializing Monitor
	monitorOptions := ethmonitor.DefaultOptions

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	assert.NoError(t, err)

	go func(t *testing.T) {
		err := monitor.Run(context.Background())
		if err != nil {
			panic(err)
		}
	}(t)
	defer monitor.Stop()

	logger := httplog.NewLogger("ethReceipts_test")

	receipts, err := ethreceipts.NewReceipts(logger, provider, monitor, ethreceipts.Options{
		NumBlocksUntilTxnFinality:     int(8),
		WaitNumBlocksBeforeExhaustion: int(3),
	})
	assert.NoError(t, err)

	go func(t *testing.T) {
		err := receipts.Run(context.Background())
		if err != nil {
			panic(err)
		}
	}(t)
	defer receipts.Stop()

	// Ensure test-chain is running
	testChain, err = testutil.NewTestChain()
	if err != nil {
		log.Fatal(fmt.Errorf("NewTestChain failed: %w", err))
	}

	wallet, err := testChain.Wallet()
	if err != nil {
		log.Fatal(err)
	}

	toAddress := testutil.DummyAddr()
	signed, err := wallet.NewTransaction(context.Background(), &ethtxn.TransactionRequest{
		From: wallet.Address(),
		To:   &toAddress,
	})
	if err != nil {
		log.Fatal(err)
	}
	assert.NoError(t, err)

	time.Sleep(5 * time.Second)

	txn, _, err := wallet.SendTransaction(context.Background(), signed)
	if err != nil {
		log.Fatal(err)
	}
	assert.NoError(t, err)

	receipt, event, err := receipts.GetFinalTransactionReceipt(context.Background(), txn.Hash())
	assert.NoError(t, err)
	assert.True(t, event == ethreceipts.Finalized, "Transaction couldn't found or failed.")

	currentBlockNumber, err := provider.BlockNumber(context.Background())
	assert.NoError(t, err)

	assert.True(t, currentBlockNumber-receipt.BlockNumber.Uint64() >= uint64(receipts.Options().NumBlocksUntilTxnFinality), "Transaction not finalized.")
	assert.True(t, receipt.Status == types.ReceiptStatusSuccessful, "Transaction couldn't found or failed.")

}

// // // TestReceiptsWithReorg tests the receipt retrieval with a reorg. This is not a deterministic test, but it can be used to check
// // // if the receipt retrieval is working correctly in various cases of reorg. Can be used for updated logic of relayer
// // func TestReceiptsWithReorg(t *testing.T) {

// // 	var testConfig = &config.Config{}

// // 	// Reading the config to retrieve private key
// // 	err := config.NewFromFile(os.Getenv("CONFIG"), "../../etc/relayer.test.conf", testConfig)
// // 	if err != nil {
// // 		log.Fatal(err)
// // 	}

// // 	wallet, err := relayer.InstantiateWallet(testConfig.Relayer.Wallet)
// // 	if err != nil {
// // 		log.Fatal(err)
// // 	}
// // 	assert.NoError(t, err)

// // 	fmt.Println("Wallet address used for testing: ", wallet.Address())

// // 	// Retrieving test-chain IP
// // 	ip := GetIp()
// // 	fmt.Println("IP of the chain is: ", ip)

// // 	provider, err := ethrpc.NewProvider("http://" + ip + ":8545/")
// // 	assert.NoError(t, err)
// // 	wallet.SetProvider(provider)

// // 	// Initializing Monitor
// // 	monitorOptions := ethmonitor.DefaultOptions
// // 	monitorOptions.PollingInterval = 5 * time.Millisecond

// // 	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
// // 	assert.NoError(t, err)

// // 	go func(t *testing.T) {
// // 		err := monitor.Run(context.Background())
// // 		if err != nil {
// // 			panic(err)
// // 		}
// // 	}(t)
// // 	defer monitor.Stop()

// // 	logger := httplog.NewLogger("ethReceipts_test_with_reorg")

// // 	// Initializing Receipt
// // 	receipts, err := ethreceipts.NewReceipts(logger, provider, monitor)
// // 	assert.NoError(t, err)

// // 	go func(t *testing.T) {
// // 		err := receipts.Run(context.Background())
// // 		if err != nil {
// // 			panic(err)
// // 		}
// // 	}(t)
// // 	defer receipts.Stop()

// // 	sub := monitor.Subscribe()
// // 	defer sub.Unsubscribe()

// // 	// Preparing Tx
// // 	toAddress := common.HexToAddress("0xEe92efac46804405D4F1B93053A289985BD86962")

// // 	txnRequest := ethtxn.TransactionRequest{
// // 		From: wallet.Address(),
// // 		To:   &toAddress,
// // 	}

// // 	newTxn, err := wallet.NewTransaction(context.Background(), &txnRequest)
// // 	if err != nil {
// // 		log.Fatal(err)
// // 	}
// // 	assert.NoError(t, err)

// // 	events := make([]*ethmonitor.Block, 0)

// // 	// Sending Transaction
// // 	fmt.Println("Sending Tx")
// // 	txn, _, err := wallet.SendTransaction(context.Background(), newTxn)
// // 	if err != nil {
// // 		log.Fatal(err)
// // 	}
// // 	assert.NoError(t, err)
// // 	fmt.Println("txn.Hash(): ", txn.Hash())

// // 	go func() {
// // 		for {
// // 			select {
// // 			case blocks := <-sub.Blocks():
// // 				for _, b := range blocks {
// // 					events = append(events, b)
// // 					fmt.Println("event:", b.Event, "block:", b.NumberU64(), b.Hash().Hex(), "parent:", b.ParentHash().Hex(), "# logs:", len(b.Logs))
// // 					fmt.Println("Number of transactions in the block: ", b.Transactions().Len())
// // 				}
// // 			case <-sub.Done():
// // 				return
// // 			}
// // 		}
// // 	}()

// // 	go func(t *testing.T) {

// // 		Fork()
// // 		events = make([]*ethmonitor.Block, 0)

// // 		WaitBlock(context.Background(), provider)
// // 		WaitBlock(context.Background(), provider)

// // 		time.Sleep(2 * time.Second)

// // 		fmt.Println("Forked Events are: ", events)
// // 		for _, b := range events {
// // 			fmt.Println("Forked Events b.Event is: ", b.Event)
// // 			fmt.Println("Forked Events b.Block.NumberU64(): ", b.Block.NumberU64())
// // 			// assert.Equal(t, b.Event, ethmonitor.Added)
// // 		}

// // 		events = make([]*ethmonitor.Block, 0)
// // 		Join()

// // 		// Wait for reorg
// // 		WaitBlock(context.Background(), provider)
// // 		WaitBlock(context.Background(), provider)

// // 		time.Sleep(2 * time.Second)

// // 		fmt.Println("3rd Events are: ")
// // 		for _, b := range events {
// // 			fmt.Println("After joining b.Event is: ", b.Event)
// // 			fmt.Println("After joining b.Block.NumberU64(): ", b.Block.NumberU64())
// // 		}

// // 	}(t)

// // 	// go func(t *testing.T) {
// // 	receipt, err := receipts.GetTransactionReceiptAfterBlock(context.Background(), txn.Hash())
// // 	assert.NoError(t, err)
// // 	assert.True(t, receipt.Status == 1, "Transaction couldn't found or failed.")
// // 	// }(t)

// // 	monitor.Stop()
// // 	receipts.Stop()
// // }

// // func GetIp() string {
// // 	output, err := exec.Command("yarn", "--silent", "--cwd", "../../../tools/test-chain-reorgme", "chain:ip", "0").CombinedOutput()

// // 	if err != nil {
// // 		os.Stderr.WriteString(err.Error())
// // 	}

// // 	return strings.Replace(string(output), "\n", "", 1)
// // }

// // func WaitBlock(ctx context.Context, provider *ethrpc.Provider) error {
// // 	var lastBlock = uint64(0)

// // 	fmt.Println("Waiting a block")

// // 	for {
// // 		block, err := provider.BlockNumber(ctx)
// // 		if err != nil {
// // 			return err
// // 		}

// // 		if lastBlock == 0 {
// // 			lastBlock = block
// // 		}

// // 		if block != lastBlock {
// // 			return nil
// // 		}
// // 	}
// // }

// // func Fork() string {
// // 	fmt.Println("Forking...")
// // 	output, err := exec.Command("yarn", "--silent", "--cwd", "../../../tools/test-chain-reorgme", "chain:fork").CombinedOutput()

// // 	if err != nil {
// // 		os.Stderr.WriteString(err.Error())
// // 	}

// // 	fmt.Println("Forked!")

// // 	return string(output)
// // }

// // func Join() string {
// // 	fmt.Println("Joining...")
// // 	output, err := exec.Command("yarn", "--silent", "--cwd", "../../../tools/test-chain-reorgme", "chain:join").CombinedOutput()

// // 	if err != nil {
// // 		os.Stderr.WriteString(err.Error())
// // 	}

// // 	fmt.Println("Joined!")

// // 	return string(output)
// // }
