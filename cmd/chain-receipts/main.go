package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/0xsequence/ethkit"
	"github.com/0xsequence/ethkit/ethcoder"
	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethreceipts"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
	"github.com/0xsequence/ethkit/util"
	"github.com/goware/logger"
)

var ETH_NODE_URL = "http://localhost:8545"

func init() {
	testConfig, err := util.ReadTestConfig("../../ethkit-test.json")
	if err != nil {
		panic(err)
	}

	if testConfig["POLYGON_MAINNET_URL"] != "" {
		ETH_NODE_URL = testConfig["POLYGON_MAINNET_URL"]
	}
	// if testConfig["MAINNET_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["MAINNET_URL"]
	// }
}

func main() {
	fmt.Println("chain-receipts start")

	// Provider
	provider, err := ethrpc.NewProvider(ETH_NODE_URL)
	if err != nil {
		log.Fatal(err)
	}

	// Monitor options
	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.PollingInterval = time.Duration(1000 * time.Millisecond)
	// monitorOptions.DebugLogging = true
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 400
	monitorOptions.StartBlockNumber = nil // track the head

	receiptListenerOptions := ethreceipts.DefaultOptions
	receiptListenerOptions.NumBlocksToFinality = 20
	receiptListenerOptions.FilterMaxWaitNumBlocks = 5

	err = listener(provider, monitorOptions, receiptListenerOptions)
	if err != nil {
		log.Fatal(err)
	}
}

func listener(provider *ethrpc.Provider, monitorOptions ethmonitor.Options, receiptListenerOptions ethreceipts.Options) error {
	ctx := context.Background()

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		err = monitor.Run(ctx)
		if err != nil {
			panic(err)
		}
	}()
	defer monitor.Stop()

	// monitorSub := monitor.Subscribe()
	// defer monitorSub.Unsubscribe()

	receiptListener, err := ethreceipts.NewReceiptListener(logger.NewLogger(logger.LogLevel_INFO), provider, monitor, receiptListenerOptions)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		err := receiptListener.Run(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}()
	defer receiptListener.Stop()

	// Find specific meta transaction -- note: this is not the "transaction hash",
	// this is a sub-transaction where the id is emitted as an event.
	FilterMetaTransactionID := func(metaTxnID ethkit.Hash) ethreceipts.FilterQuery {
		return ethreceipts.FilterLogs(func(logs []*types.Log) bool {
			for _, log := range logs {
				if len(log.Data) != 32 {
					continue
				}
				if common.BytesToHash(log.Data) != metaTxnID {
					continue
				}
				isTxExecuted := IsTxExecutedEvent(log, metaTxnID)
				failedHash, _, err := DecodeTxFailedEvent(log)
				isTxFailed := err == nil && failedHash == metaTxnID
				if isTxExecuted || isTxFailed {
					// found the sequence meta txn
					return true
				}
			}
			return false
		})
	}
	_ = FilterMetaTransactionID

	// Find any Sequence meta txns
	FilterMetaTransactionAny := func() ethreceipts.FilterQuery {
		return ethreceipts.FilterLogs(func(logs []*types.Log) bool {
			foundNonceEvent := false
			for _, log := range logs {
				if len(log.Topics) > 0 && log.Topics[0] == NonceChangeEventSig {
					foundNonceEvent = true
					break
				}
			}
			if !foundNonceEvent {
				return false
			}

			for _, log := range logs {
				if len(log.Topics) == 1 && log.Topics[0] == TxFailedEventSig {
					// failed sequence txn
					return true
				} else if len(log.Topics) == 0 && len(log.Data) == 32 {
					// possibly a successful sequence txn -- but not for certain
					return true
				}
			}

			return false
		})
	}
	_ = FilterMetaTransactionAny

	sub := receiptListener.Subscribe(
		// FilterMetaTransactionID(common.HexToHash("0xef43f357598c989c3d7fc11d0608a3487923679850dc86d7c66709749918c5c3")).LimitOne(true),
		FilterMetaTransactionAny().MaxWait(0), // listen on all sequence txns
	)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case receipt := <-sub.TransactionReceipt():

				fmt.Println("=> sequence txn receipt:", receipt.TransactionHash())

			case <-sub.Done():
				return
			}
		}
	}()

	wg.Wait()

	return nil
}

// Transaction events as defined in wallet-contracts IModuleCalls.sol
var (
	// NonceChangeEventSig is the signature event emitted as the first event on the batch execution
	// 0x1f180c27086c7a39ea2a7b25239d1ab92348f07ca7bb59d1438fcf527568f881
	NonceChangeEventSig = MustEncodeSig("NonceChange(uint256,uint256)")

	// TxFailedEventSig is the signature event emitted in a failed smart-wallet meta-transaction batch
	// 0x3dbd1590ea96dd3253a91f24e64e3a502e1225d602a5731357bc12643070ccd7
	TxFailedEventSig = MustEncodeSig("TxFailed(bytes32,bytes)")

	// TxExecutedEventSig is the signature of the event emitted in a successful transaction
	// 0x0639b0b186d373976f8bb98f9f7226ba8070f10cb6c7f9bd5086d3933f169a25
	TxExecutedEventSig = MustEncodeSig("TxExecuted(bytes32)")
)

func MustEncodeSig(str string) common.Hash {
	return crypto.Keccak256Hash([]byte(str))
}

func IsTxExecutedEvent(log *types.Log, hash common.Hash) bool {
	return len(log.Topics) == 0 && bytes.Equal(log.Data, hash[:])
}

func DecodeTxFailedEvent(log *types.Log) (common.Hash, string, error) {
	if len(log.Topics) != 1 || log.Topics[0] != TxFailedEventSig {
		return common.Hash{}, "", fmt.Errorf("not a TxFailed event")
	}

	var hash common.Hash
	var revert []byte
	if err := ethcoder.AbiDecoder([]string{"bytes32", "bytes"}, log.Data, []interface{}{&hash, &revert}); err != nil {
		return common.Hash{}, "", err
	}

	reason, err := abi.UnpackRevert(revert)
	if err != nil {
		return common.Hash{}, "", err
	}

	return hash, reason, nil
}
