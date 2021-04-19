package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/util"
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
}

func main() {

	var err error
	var monitor *ethmonitor.Monitor

	fmt.Println("chain-watch start")

	// Provider
	provider, err := ethrpc.NewProvider(ETH_NODE_URL)
	if err != nil {
		log.Fatal(err)
	}

	// Monitor

	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.Logger = log.Default()

	// if logLevel == zerolog.DebugLevel {
	monitorOptions.DebugLogging = true
	// }
	monitorOptions.StartBlockNumber = nil // track the head

	monitorOptions.BlockRetentionLimit = 64

	monitorOptions.PollingInterval = time.Duration(250 * time.Millisecond)
	// monitorOptions.PollingInterval = time.Duration(1000 * time.Millisecond)

	// if cfg.Environment == "test" {
	// 	monitorOptions.PollingInterval = time.Duration(100 * time.Millisecond)
	// }
	// if cfg.Ethereum.PollInterval > 100 { // 100 ms is the minimum, or would be crazy
	// 	monitorOptions.PollingInterval = time.Duration(cfg.Ethereum.PollInterval) * time.Millisecond
	// }

	monitor, err = ethmonitor.NewMonitor(provider, monitorOptions)
	if err != nil {
		log.Fatal(err)
	}

	err = reorgWatch(monitor)
	if err != nil {
		log.Fatal(err)
	}
}

func reorgWatch(monitor *ethmonitor.Monitor) error {
	err := monitor.Start(context.Background())
	if err != nil {
		return err
	}
	defer monitor.Stop()

	sub := monitor.Subscribe()
	defer sub.Unsubscribe()

	feed := []ethmonitor.Blocks{}

	go func() {
		for {
			select {
			case blocks := <-sub.Blocks():

				for _, b := range blocks {
					fmt.Println("  -> type:", b.Type, "block:", b.NumberU64(), b.Hash().Hex(), "parent:", b.ParentHash().Hex(), "# logs:", len(b.Logs))
				}
				fmt.Println("")

				feed = append(feed, blocks)

			case <-sub.Done():
				return
			}
		}
	}()

	select {
	case <-time.After(2 * time.Minute): // max amount of time to run, or wait for ctrl+c
		break
	}
	monitor.Stop()

	printSummary(feed)

	return nil
}
