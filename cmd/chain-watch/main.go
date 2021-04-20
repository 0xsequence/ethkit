package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/util"
	"github.com/goware/httpvcr"
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
	monitorOptions.PollingInterval = time.Duration(250 * time.Millisecond)
	monitorOptions.Logger = log.Default()
	monitorOptions.DebugLogging = true
	monitorOptions.BlockRetentionLimit = 64
	monitorOptions.StartBlockNumber = nil // track the head

	feed, err := chainWatch(provider, monitorOptions)
	if err != nil {
		log.Fatal(err)
	}

	summary := generateSummary(feed)
	printSummary(summary)
	analyzeSummary(summary)
}

func chainWatch(provider *ethrpc.Provider, monitorOptions ethmonitor.Options) ([]ethmonitor.Blocks, error) {
	ctx := context.Background()
	vcr := httpvcr.New("ethmonitor_watch")
	vcr.Start(ctx)

	vcr.URLRewriter = func(url string) string {
		// rewrite the url to hide the API keys
		return "http://polygon/"
	}

	if vcr.Mode() == httpvcr.ModeReplay {
		// change options to run replay tests faster
		monitorOptions.PollingInterval = 5 * time.Millisecond
	}

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	if err != nil {
		log.Fatal(err)
	}

	err = monitor.Start(ctx)
	if err != nil {
		return nil, err
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
	case <-vcr.Done():
		break
	case <-time.After(120 * time.Second): // max amount of time to run, or wait for ctrl+c
		break
	}
	monitor.Stop()
	vcr.Stop()

	return feed, nil
}
