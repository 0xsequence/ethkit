package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum/common/hexutil"
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

	if testConfig["POLYGON_MAINNET_WSS_URL"] != "" {
		ETH_NODE_URL = testConfig["POLYGON_MAINNET_URL"]
		ETH_NODE_WSS_URL = testConfig["POLYGON_MAINNET_WSS_URL"]
	}
	// if testConfig["MAINNET_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["MAINNET_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["MAINNET_WSS_URL"]
	// }
	if testConfig["ARB_NOVA_WSS_URL"] != "" {
		ETH_NODE_URL = testConfig["ARB_NOVA_URL"]
		ETH_NODE_WSS_URL = testConfig["ARB_NOVA_WSS_URL"]
	}
	// if testConfig["AVAX_MAINNET_WSS_URL"] != "" {
	// 	// ETH_NODE_URL = testConfig["ARB_NOVA_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["AVAX_MAINNET_WSS_URL"]
	// }
}

func main() {
	client, err := rpc.Dial(ETH_NODE_WSS_URL)
	if err != nil {
		log.Fatal(err)
	}

	ch := make(chan map[string]interface{})

	sub, err := client.EthSubscribe(context.Background(), ch, "newHeads")
	if err != nil {
		log.Fatal(err)
	}

	var prevHash string
	go func() {
		for {
			select {

			case err := <-sub.Err():
				fmt.Println("sub err!", err)
				os.Exit(1)

			case out := <-ch:
				// fmt.Println("===> out:", out)
				// spew.Dump(out)

				hash, ok := out["hash"].(string)
				if !ok {
					panic(ok)
				}
				parentHash, ok := out["parentHash"].(string)
				if !ok {
					panic(ok)
				}
				if prevHash != "" {
					if prevHash != parentHash {
						fmt.Println("REORG!")
					}
				}
				prevHash = hash
				num, ok := out["number"].(string)
				if !ok {
					panic("hmm")
				}
				blockNumber := hexutil.MustDecodeBig(num)
				fmt.Println("hash", hash, "num", blockNumber.String())
			}
		}
	}()

	time.Sleep(20 * time.Minute)
	sub.Unsubscribe()

	// os.Exit(1)

	// filter := map[string]interface{}{
	// 	"topics": []string{},
	// 	// "fromBlock": "latest",
	// }

	// sub, err = client.EthSubscribe(context.Background(), ch, "logs", filter)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// go func() {
	// 	for {
	// 		select {

	// 		case err := <-sub.Err():
	// 			fmt.Println("sub err!", err)
	// 			os.Exit(1)

	// 		case out := <-ch:
	// 			// fmt.Println("===> out:", out)
	// 			spew.Dump(out)

	// 			removed, ok := out["removed"].(bool)
	// 			if !ok {
	// 				panic("no")
	// 			}
	// 			if removed {
	// 				panic("removed!!!")
	// 			}
	// 		}
	// 	}
	// }()

	// time.Sleep(2 * time.Minute)
	// sub.Unsubscribe()
}
