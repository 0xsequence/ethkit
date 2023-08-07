package main

import (
	"context"
	"fmt"
	"log"

	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/ethclient"
	"github.com/0xsequence/ethkit/util"
	"github.com/davecgh/go-spew/spew"
)

func main() {
	testConfig, err := util.ReadTestConfig("../../ethkit-test.json")
	if err != nil {
		log.Fatal(err)
	}

	client, err := ethclient.Dial(testConfig["MAINNET_WSS_URL"])
	// client, err := ethclient.Dial(testConfig["POLYGON_MAINNET_WSS_URL"])
	// client, err := ethclient.Dial(testConfig["POLYGON_ZKEVM_WSS_URL"])
	// client, err := ethclient.Dial(testConfig["ARBITRUM_MAINNET_WSS_URL"])
	// client, err := ethclient.Dial(testConfig["OPTIMISM_MAINNET_WSS_URL"])

	if err != nil {
		log.Fatal(err)
	}

	// NOTE: this will break for optimism and arbitrum, as go-ethereum fails
	// to parse something in the header. But see `cmd/chain-ws-head2` which is lower-level.
	// Multiple approaches can be taken, just noting.
	headers := make(chan *types.Header)
	sub, err := client.SubscribeNewHead(context.Background(), headers)
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case err := <-sub.Err():
			log.Fatal(err)
		case header := <-headers:
			block, err := client.BlockByHash(context.Background(), header.Hash())
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("New block:", block.Number().Uint64())
			spew.Dump(block)
		}
	}
}
