package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/0xsequence/ethkit/go-ethereum/rpc"
	"github.com/0xsequence/ethkit/util"
)

func main() {
	testConfig, err := util.ReadTestConfig("../../ethkit-test.json")
	if err != nil {
		log.Fatal(err)
	}

	// nodeWebsocketURL := testConfig["MAINNET_WSS_URL"]
	nodeWebsocketURL := testConfig["POLYGON_MAINNET_WSS_URL"]
	// nodeWebsocketURL := testConfig["POLYGON_ZKEVM_WSS_URL"]
	// nodeWebsocketURL := testConfig["ARBITRUM_MAINNET_WSS_URL"]
	// nodeWebsocketURL := testConfig["OPTIMISM_MAINNET_WSS_URL"]

	client, err := rpc.Dial(nodeWebsocketURL)
	if err != nil {
		log.Fatal(err)
	}

	ch := make(chan any)

	// Listening for new blocks:
	sub, err := client.EthSubscribe(context.Background(), ch, "newHeads")

	// Listening for new logs:
	// sub, err := client.EthSubscribe(context.Background(), ch, "logs", map[string]interface{}{})

	if err != nil {
		log.Fatal(err)
	}

	for {
		select {

		case <-sub.Err():
			log.Fatal(fmt.Errorf("websocket error %w", err))

		case data := <-ch:
			v, _ := json.Marshal(data)
			fmt.Println(string(v))
			fmt.Println("# bytes??", len(v), "CU?", float64(len(v))*.04)
		}
	}

}
