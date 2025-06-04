package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/util"
	rediscache "github.com/goware/cachestore-redis"
	cachestore "github.com/goware/cachestore2"
	"github.com/goware/pp"
)

var ETH_NODE_URL = "http://localhost:8887/polygon"
var ETH_NODE_WSS_URL = ""

const SNAPSHOT_ENABLED = false

// TODO: move this to ethmonitor/e2e

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
	// }
	// if testConfig["ARB_NOVA_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["ARB_NOVA_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["ARB_NOVA_WSS_URL"]
	// }

	// if testConfig["ETHERLINK_MAINNET_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["ETHERLINK_MAINNET_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["ETHERLINK_MAINNET_WSS_URL"]
	// }

	// if testConfig["LAOS_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["LAOS_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["LAOS_WSS_URL"]
	// }

	// if testConfig["ARB_SEPOLIA_URL"] != "" {
	// 	ETH_NODE_URL = testConfig["ARB_SEPOLIA_URL"]
	// 	ETH_NODE_WSS_URL = testConfig["ARB_SEPOLIA_WSS_URL"]
	// }

}

func main() {
	fmt.Println("chain-watch start")

	// Provider
	provider, err := ethrpc.NewProvider(ETH_NODE_URL, ethrpc.WithStreaming(ETH_NODE_WSS_URL)) //, ethrpc.WithStrictValidation())
	if err != nil {
		log.Fatal(err)
	}

	chainID, _ := provider.ChainID(context.Background())
	fmt.Println("=> chain id:", chainID.String())

	// Monitor options
	cachestore.MaxKeyLength = 180
	monitorOptions := ethmonitor.DefaultOptions
	monitorOptions.PollingInterval = time.Duration(2000 * time.Millisecond)
	monitorOptions.WithLogs = true
	monitorOptions.BlockRetentionLimit = 64
	monitorOptions.StreamingRetryAfter = 1 * time.Minute
	monitorOptions.StartBlockNumber = nil // track the head

	latestBlock, err := provider.BlockByNumber(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	_ = latestBlock

	monitorOptions.StartBlockNumber = big.NewInt(0).Sub(latestBlock.Number(), big.NewInt(10))
	// monitorOptions.StartBlockNumber = big.NewInt(3754824)
	// monitorOptions.Bootstrap = true

	monitorOptions.Logger = slog.Default()
	monitorOptions.DebugLogging = true

	// monitorOptions.TrailNumBlocksBehindHead = 4
	// monitorOptions.UnsubscribeOnStop = true

	if os.Getenv("REDIS_ENABLED") == "1" {
		monitorOptions.CacheBackend, err = rediscache.NewBackend(&rediscache.Config{
			Enabled: true,
			Host:    "localhost",
			Port:    6379,
		})
		if err != nil {
			log.Fatal(err)
		}
	}

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

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	snapshotFile := filepath.Join(cwd, "snapshot.json")

	monitor, err := ethmonitor.NewMonitor(provider, monitorOptions)
	if err != nil {
		log.Fatal(err)
	}

	data, _ := os.ReadFile(snapshotFile)
	if len(data) > 0 && SNAPSHOT_ENABLED {
		err = monitor.Chain().BootstrapFromBlocksJSON(data)
		if err != nil {
			panic(err)
		}
	} else {
		err = monitor.Chain().BootstrapFromBlocks(ethmonitor.Blocks{})
	}

	go func() {
		err = monitor.Run(ctx)
		if err != nil {
			fmt.Println("monitor run stopped with", err)
			panic(err)
		}
	}()
	defer monitor.Stop()

	sub := monitor.Subscribe()
	defer sub.Unsubscribe()

	feed := []ethmonitor.Blocks{}
	events := ethmonitor.Blocks{}
	count := 0

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case blocks := <-sub.Blocks():

				for _, b := range blocks {
					pp.Green("###  -> type: %d", b.Event).Blue("block:%d", b.NumberU64()).Green("%s parent:%s # txns:%d # logs:%d", b.Hash().Hex(), b.ParentHash().Hex(), len(b.Transactions()), len(b.Logs)).Println()
				}
				fmt.Println("")

				// feed = append(feed, blocks.Copy())
				events = append(events, blocks.Copy()...)
				count++

				if len(data) == 0 && SNAPSHOT_ENABLED {
					// NOTE: here we write the entire events log to disk each time,
					// but in practice we should write a WAL with just the newly fetched data.
					// As well, instead of writing it to disk as an array, better to write a list
					// of objects, one after another.
					// Or... we can write [event1, event2,event3],[event,event5],[event6],...
					// to the disk, and this would be fine too.
					d, _ := json.Marshal(events)
					writeToFile(snapshotFile, d)
				}

			case <-sub.Done():
				fmt.Println("sub stopped, err?", sub.Err())
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

func writeToFile(path string, data []byte) {
	if !SNAPSHOT_ENABLED {
		return
	}
	os.WriteFile(path, data, 0644)
}
