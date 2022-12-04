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

func TestReceiptsBasic(t *testing.T) {
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

	receipts, err := ethreceipts.NewReceipts(logger, provider, monitor)
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

	//
	time.Sleep(5 * time.Second)

	txn, _, err := wallet.SendTransaction(context.Background(), signed)
	if err != nil {
		log.Fatal(err)
	}
	assert.NoError(t, err)

	receipt, err := receipts.GetTransactionReceipt(context.Background(), txn.Hash())
	assert.NoError(t, err)

	assert.True(t, receipt.Status == types.ReceiptStatusSuccessful, "Transaction couldn't found or failed.")
}

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
