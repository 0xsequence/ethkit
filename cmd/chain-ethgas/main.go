package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/0xsequence/ethkit/ethgas"
	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/util"
)

var ETH_NODE_URL = "http://localhost:8545"
var ETH_NODE_WSS_URL = ""

func init() {
	testConfig, err := util.ReadTestConfig("../../ethkit-test.json")
	if err != nil {
		panic(err)
	}

	if testConfig["POLYGON_MAINNET_URL"] != "" {
		ETH_NODE_URL = testConfig["POLYGON_MAINNET_URL"]
		ETH_NODE_WSS_URL = testConfig["POLYGON_MAINNET_WSS_URL"]
	}
	// if testConfig["MAINNET_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["MAINNET_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["MAINNET_WSS_URL"]
	// }

	// ETH_NODE_URL = ""
	// ETH_NODE_WSS_URL = ""
}

func main() {
	fmt.Println("chain-ethgas start")

	// Provider
	provider, err := ethrpc.NewProvider(ETH_NODE_URL, ethrpc.WithStreaming(ETH_NODE_WSS_URL))
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

	// ...
	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	go func() {
		err = monitor.Run(ctx)
		if err != nil {
			panic(err)
		}
	}()
	defer monitor.Stop()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	gasGague, err := ethgas.NewGasGauge(logger, monitor, 1, false)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		err := gasGague.Run(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}()
	defer gasGague.Stop()

	sub := gasGague.Subscribe()
	defer sub.Unsubscribe()

	for {
		select {
		case <-sub.Blocks():
			prices := gasGague.SuggestedGasPrice()
			bids := gasGague.SuggestedGasPriceBid()
			fmt.Println(prices.BlockNum, prices.BlockTime, prices.Instant, prices.Fast, prices.Standard, prices.Slow)
			fmt.Println(bids.BlockNum, bids.BlockTime, bids.Instant, bids.Fast, bids.Standard, bids.Slow)
		}
	}
}
