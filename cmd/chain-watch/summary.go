package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

var waitBeforeAnalyze = time.Second * 30

type summary struct {
	feed []ethmonitor.Blocks

	countAdded   int
	countRemoved int
	countUpdated int

	blockNumAdded   [][]uint64
	blockNumRemoved [][]uint64
	blockNumUpdated [][]uint64
}

func generateSummary(feed []ethmonitor.Blocks) *summary {
	summary := &summary{feed: feed}

	for _, blocks := range feed {
		batchBlockNumAdded := []uint64{}
		batchBlockNumRemoved := []uint64{}
		batchBlockNumUpdated := []uint64{}

		for _, b := range blocks {
			switch b.Type {
			case ethmonitor.Added:
				summary.countAdded += 1
				batchBlockNumAdded = append(batchBlockNumAdded, b.NumberU64())
				break
			case ethmonitor.Removed:
				summary.countRemoved += 1
				batchBlockNumRemoved = append(batchBlockNumRemoved, b.NumberU64())
				break
			case ethmonitor.Updated:
				summary.countUpdated += 1
				batchBlockNumUpdated = append(batchBlockNumUpdated, b.NumberU64())
				break
			}
		}

		summary.blockNumAdded = append(summary.blockNumAdded, batchBlockNumAdded)
		summary.blockNumRemoved = append(summary.blockNumRemoved, batchBlockNumRemoved)
		summary.blockNumUpdated = append(summary.blockNumUpdated, batchBlockNumUpdated)
	}

	return summary
}

func printSummary(summary *summary) {
	fmt.Println("")
	fmt.Println("SUMMARY:")
	fmt.Println("========")
	fmt.Println("")

	fmt.Println("total blocks added:   ", summary.countAdded)
	fmt.Println("total blocks removed: ", summary.countRemoved)
	fmt.Println("total blocks updated: ", summary.countUpdated)

	fmt.Println("")
	fmt.Println("block numbers added:")
	fmt.Printf("%v\n", summary.blockNumAdded)

	fmt.Println("")
	fmt.Println("block numbers removed:")
	fmt.Printf("%v\n", summary.blockNumRemoved)

	fmt.Println("")
	fmt.Println("block numbers updated:")
	fmt.Printf("%v\n", summary.blockNumUpdated)

	fmt.Println("")
	fmt.Println("NOTES:")
	fmt.Println(" * compare results with https://explorer-mainnet.maticvigil.com/reorgs")
	fmt.Println(" * 'removed' means blocks which have been marked for removal due to reorg")
	fmt.Println(" * 'updated' means block data which has been filled after the fact (usual due to bad log fetch initially)")

	// analyze and validate summary
	fmt.Println("")
	fmt.Println("----------------------------------------------------------------------------------")
	fmt.Println("")
}

func analyzeSummary(summary *summary) {
	feed := summary.feed

	fmt.Println("")
	fmt.Println("ANALYZE:")
	fmt.Println("========")
	fmt.Println("")

	if summary.countRemoved == 0 {
		fmt.Println("no reorgs occured, so analysis is inconclusive. try again.")
		return
	}

	fmt.Println("waiting before analysis to allow reorgs to pass..")
	// time.Sleep(waitBeforeAnalyze)
	time.Sleep(1 * time.Second)

	fmt.Println("analyzing our canonical chain by checking again node..")

	firstBlock, err := getFirstBlock(feed)
	if err != nil {
		log.Fatal(err)
	}

	lastBlock := feed[len(feed)-1].LatestBlock().Block

	fmt.Println("=> firstBlock", firstBlock.NumberU64())
	fmt.Println("=> lastBlock", lastBlock.NumberU64())

	err = analyzeCanonicalChain(feed, firstBlock, lastBlock)
	if err != nil {
		log.Fatal(err)
	}
}

func analyzeCanonicalChain(feed []ethmonitor.Blocks, firstBlock, lastBlock *types.Block) error {

	return nil
}

func getFirstBlock(feed []ethmonitor.Blocks) (*types.Block, error) {
	for _, f := range feed {
		for _, b := range f {
			if b.Type == ethmonitor.Added {
				return b.Block, nil
			}
		}
	}
	return nil, errors.New("first block not found, unexpected!")
}
