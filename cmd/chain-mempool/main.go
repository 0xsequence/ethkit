package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/0xsequence/ethkit/ethmempool"
	"github.com/0xsequence/ethkit/util"
)

var (
	ETH_NODE_URL     = ""
	ETH_NODE_WSS_URL = ""
)

func init() {
	testConfig, err := util.ReadTestConfig("../../ethkit-test.json")
	if err != nil {
		panic(err)
	}

	if testConfig["POLYGON_MAINNET_WSS_URL"] != "" {
		ETH_NODE_URL = testConfig["POLYGON_MAINNET_URL"]
		ETH_NODE_WSS_URL = testConfig["POLYGON_MAINNET_WSS_URL"]
	}
	// if testConfig["MAINNET_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["MAINNET_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["MAINNET_WSS_URL"]
	// }
}

func main() {
	mempool, err := ethmempool.NewMempool(ETH_NODE_WSS_URL)
	if err != nil {
		log.Fatal(err)
	}

	err = mempool.Start(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	sub := mempool.Subscribe()

	// sub := mempool.SubscribeWithFilter(func(pendingTxnHash string) bool {
	// 	// random example, where we can filter the list before we notify
	// 	// our subscriber
	// 	if strings.HasPrefix(pendingTxnHash, "0x1") || strings.HasPrefix(pendingTxnHash, "0x0") {
	// 		return true
	// 	}
	// 	return false
	// })

	defer sub.Unsubscribe()

	go func() {
		for {
			select {
			case pendingTxnHash := <-sub.PendingTransactionHash():
				fmt.Println("newPendingTxn:", pendingTxnHash)

			case <-sub.Done():
				return
			}
		}
	}()

	time.Sleep(2 * time.Minute)
}

// func main() {
// 	client, err := rpc.Dial(ETH_NODE_WSS_URL)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	ch := make(chan string)
// 	sub, err := client.EthSubscribe(context.Background(), ch, "newPendingTransactions")
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	go func() {
// 		for {
// 			select {

// 			case err := <-sub.Err():
// 				fmt.Println("sub err!", err)
// 				os.Exit(1)

// 			case txnHash := <-ch:
// 				fmt.Println("===> new pending txn:", txnHash)

// 			}
// 		}
// 	}()

// 	time.Sleep(2 * time.Minute)
// 	sub.Unsubscribe()
// }
