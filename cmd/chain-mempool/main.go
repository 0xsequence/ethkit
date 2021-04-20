package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum/rpc"
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

	if testConfig["POLYGON_MAINNET_WSS_URL2"] != "" {
		// ETH_NODE_URL = testConfig["POLYGON_MAINNET_URL"]
		ETH_NODE_WSS_URL = testConfig["POLYGON_MAINNET_WSS_URL2"]
	}
	// if testConfig["MAINNET_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["MAINNET_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["MAINNET_WSS_URL"]
	// }

}

func main() {
	client, err := rpc.Dial(ETH_NODE_WSS_URL)
	if err != nil {
		log.Fatal(err)
	}

	ch := make(chan string)
	sub, err := client.EthSubscribe(context.Background(), ch, "newPendingTransactions")
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {

			case err := <-sub.Err():
				fmt.Println("sub err!", err)
				os.Exit(1)

			case txnHash := <-ch:
				fmt.Println("===> new pending txn:", txnHash)

			}
		}
	}()

	time.Sleep(2 * time.Minute)
	sub.Unsubscribe()
}
