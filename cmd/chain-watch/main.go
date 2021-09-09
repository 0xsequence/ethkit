package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/util"
	"github.com/goware/pp"
)

var ETH_NODE_URL = "http://localhost:8545"

// TODO: move this to ethmonitor/e2e

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
	fmt.Println("chain-watch start")

	// Provider
	provider, err := ethrpc.NewProvider(ETH_NODE_URL)
	if err != nil {
		log.Fatal(err)
	}

	// Monitor options
	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.PollingInterval = time.Duration(1000 * time.Millisecond)
	monitorOptions.Logger = log.Default()
	monitorOptions.DebugLogging = true
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 64
	monitorOptions.StartBlockNumber = nil // track the head
	// monitorOptions.TrailNumBlocksBehindHead = 4

	chain, feed, err := chainWatch(provider, monitorOptions)
	if err != nil {
		log.Fatal(err)
	}

	summary := generateSummary(feed)
	printSummary(summary)

	analyzeSummary(provider, chain, summary)
}

func chainWatch(provider *ethrpc.Provider, monitorOptions ethmonitor.Options) (*ethmonitor.Chain, []ethmonitor.Blocks, error) {
	ctx := context.Background()
	// vcr := httpvcr.New("ethmonitor_watch1")
	// vcr := httpvcr.New("ethmonitor_watch2")
	// vcr := httpvcr.New("ethmonitor_watch3")
	// vcr := httpvcr.New("ethmonitor_watch4")
	// vcr := httpvcr.New("ethmonitor_watch5")
	// vcr.Start(ctx)

	// vcr.URLRewriter = func(url string) string {
	// 	// rewrite the url to hide the API keys
	// 	return "http://polygon/"
	// }

	// if vcr.Mode() == httpvcr.ModeReplay {
	// 	// change options to run replay tests faster
	// 	monitorOptions.PollingInterval = 5 * time.Millisecond
	// }

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

	sub := monitor.Subscribe()
	defer sub.Unsubscribe()

	feed := []ethmonitor.Blocks{}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case blocks := <-sub.Blocks():

				for _, b := range blocks {
					pp.Green("###  -> type: %d", b.Event).Blue("block:%d", b.NumberU64()).Green("%s parent:%s # logs:%d", b.Hash().Hex(), b.ParentHash().Hex(), len(b.Logs)).Println()
				}
				fmt.Println("")

				// feed = append(feed, blocks.Copy())

			case <-sub.Done():
				return
			}
		}
	}()

	// TODO: we can implement a program, chain-watch-test
	// which will assert ethmonitor behaviour
	// checking the event source to ensure its correct, etc.......

	wg.Wait()
	// select {
	// case <-vcr.Done():
	// 	break
	// case <-time.After(120 * time.Second): // max amount of time to run, or wait for ctrl+c
	// 	break
	// }
	monitor.Stop()
	// vcr.Stop()

	return monitor.Chain(), feed, nil
}
