package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

var waitBeforeAnalyze = time.Second * 30

type summary struct {
	feed []ethmonitor.Blocks

	countAdded   int
	countRemoved int
	// countUpdated int

	blockNumAdded   [][]uint64
	blockNumRemoved [][]uint64
	// blockNumUpdated [][]uint64
}

func generateSummary(feed []ethmonitor.Blocks) *summary {
	summary := &summary{feed: feed}

	for _, blocks := range feed {
		batchBlockNumAdded := []uint64{}
		batchBlockNumRemoved := []uint64{}
		// batchBlockNumUpdated := []uint64{}

		for _, b := range blocks {
			switch b.Event {
			case ethmonitor.Added:
				summary.countAdded += 1
				batchBlockNumAdded = append(batchBlockNumAdded, b.NumberU64())
				break
			case ethmonitor.Removed:
				summary.countRemoved += 1
				batchBlockNumRemoved = append(batchBlockNumRemoved, b.NumberU64())
				break
				// case ethmonitor.Updated:
				// 	summary.countUpdated += 1
				// 	batchBlockNumUpdated = append(batchBlockNumUpdated, b.NumberU64())
				// 	break
			}
		}

		summary.blockNumAdded = append(summary.blockNumAdded, batchBlockNumAdded)
		summary.blockNumRemoved = append(summary.blockNumRemoved, batchBlockNumRemoved)
		// summary.blockNumUpdated = append(summary.blockNumUpdated, batchBlockNumUpdated)
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
	// fmt.Println("total blocks updated: ", summary.countUpdated)

	fmt.Println("")
	fmt.Println("block numbers added:")
	fmt.Printf("%v\n", summary.blockNumAdded)

	fmt.Println("")
	fmt.Println("block numbers removed:")
	fmt.Printf("%v\n", summary.blockNumRemoved)

	fmt.Println("")
	fmt.Println("block numbers updated:")
	// fmt.Printf("%v\n", summary.blockNumUpdated)

	fmt.Println("")
	fmt.Println("NOTES:")
	fmt.Println(" * compare results with https://explorer-mainnet.maticvigil.com/reorgs")
	fmt.Println(" * 'removed' means blocks which have been marked for removal due to reorg")
	// fmt.Println(" * 'updated' means block data which has been filled after the fact (usual due to bad log fetch initially)")

	// analyze and validate summary
	fmt.Println("")
	fmt.Println("----------------------------------------------------------------------------------")
	fmt.Println("")
}

func analyzeSummary(provider *ethrpc.Provider, chain *ethmonitor.Chain, summary *summary) {
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

	fmt.Println("")
	fmt.Println("=> firstBlock", firstBlock.NumberU64())
	fmt.Println("=> lastBlock", lastBlock.NumberU64())

	err = analyzeCanonicalChain(provider, chain, feed, firstBlock, lastBlock)
	if err != nil {
		log.Fatal(err)
	}
}

func analyzeCanonicalChain(provider *ethrpc.Provider, chain *ethmonitor.Chain, feed []ethmonitor.Blocks, firstBlock, lastBlock *types.Block) error {

	// TODO: what if firstBlock is re-orged..? like ethmonitor_watch4 hash..
	// TODO: report if firstBlock is part of feed removed list..

	fmt.Println("")
	fmt.Println("first block hash:", firstBlock.Hash().Hex())

	//
	// Basic check against monitor.Chain() snapshot and on-chain block range
	//

	// Print block number to hash map based on block numbers
	basic := func() error {
		fmt.Println("")
		fmt.Println("=> Print block number :: hash map by querying by *block number*:")
		blockNumMapIdx := []uint64{}
		blockHashMapIdx := []string{}
		for i := firstBlock.NumberU64(); i <= lastBlock.NumberU64(); i++ {
			block, err := provider.BlockByNumber(context.Background(), big.NewInt(0).SetUint64(i))
			if err != nil {
				return err
			}
			blockNumMapIdx = append(blockNumMapIdx, i)
			blockHashMapIdx = append(blockHashMapIdx, block.Hash().Hex())
		}
		for i := 0; i < len(blockNumMapIdx); i++ {
			fmt.Printf(" %d :: %s\n", blockNumMapIdx[i], blockHashMapIdx[i])
		}

		// Print canonical chain returned from ethmonitor
		fmt.Println("")
		fmt.Println("=> Print chain from ethmonitor:")

		cblockNumMapIdx := []uint64{}
		cblockHashMapIdx := []string{}
		for _, b := range chain.Blocks() {
			fmt.Printf(" [%d] %d :: %s\n", b.Event, b.NumberU64(), b.Hash().Hex())
			cblockNumMapIdx = append(cblockNumMapIdx, b.NumberU64())
			cblockHashMapIdx = append(cblockHashMapIdx, b.Hash().Hex())
		}

		fmt.Println("")
		if len(blockNumMapIdx) != len(cblockNumMapIdx) {
			fmt.Printf("Oops, looks like we have %d entries for range index, and %d entries from canonical index", len(blockNumMapIdx), len(cblockNumMapIdx))
			return errors.New("do not match")
		} else {
			fmt.Printf("Good news.. lengths of chains both have %d entries\n", len(blockNumMapIdx))
		}

		// Check if hashes are equivalent
		//
		// NOTE: if this fails, its possible it stopped at the point of time where
		// a re-org occured. Which also is to say, that we should consider this in other
		// infrastructure which uses this chain list. Potentially, we should persist
		// a canonical chain in chaind as well.
		for i := 0; i < len(cblockNumMapIdx); i++ {
			num := blockNumMapIdx[i]
			cnum := cblockNumMapIdx[i]
			hash := blockHashMapIdx[i]
			chash := cblockHashMapIdx[i]

			if num != cnum {
				return fmt.Errorf("equivalence check of block num failed for block #%d", num)
			}
			if hash != chash {
				return fmt.Errorf("equivalence check of block hash %s failed for block #%d", hash, num)
			}
		}

		fmt.Println("Good stuff! canonical block numbers and hashes match the historical query!")
		return nil
	}
	_ = basic

	if err := basic(); err != nil {
		return err
		// fmt.Println("BASIC ERR", err)
	}

	//
	// Check `feed` of event-sources to the canonical chain
	//

	feedCheck := func() error {
		addedBlockNumbers := make(map[uint64]struct{})
		checkBlockEvent := func(block *ethmonitor.Block) error {
			blockNumber := block.NumberU64()
			_, alreadyAdded := addedBlockNumbers[blockNumber]
			switch block.Event {
			case ethmonitor.Added:
				if alreadyAdded {
					return fmt.Errorf("added block %v, but already added", blockNumber)
				}
				addedBlockNumbers[blockNumber] = struct{}{}
			case ethmonitor.Removed:
				if !alreadyAdded {
					return fmt.Errorf("removed block %v, but it was never added", blockNumber)
				}
				delete(addedBlockNumbers, blockNumber)
				// case ethmonitor.Updated:
				// 	if !alreadyAdded {
				// 		return fmt.Errorf("updated block %v, but it was never added", blockNumber)
				// 	}
			}
			return nil
		}

		isFirstBlock := true
		var latestBlockNumber uint64
		var lowestBlockNumber uint64
		var highestBlockNumber uint64
		for i, blocks := range feed {
			fmt.Println("")
			fmt.Println("FEED BLOCKS EVENT:", i)

			for _, b := range blocks {
				blockNumber := b.NumberU64()

				fmt.Printf(" -> type:%d #%d %s\n", b.Event, blockNumber, b.Hash().Hex())

				if isFirstBlock {
					latestBlockNumber = blockNumber
					lowestBlockNumber = blockNumber
					highestBlockNumber = blockNumber
					isFirstBlock = false
				} else {
					if blockNumber < lowestBlockNumber {
						return fmt.Errorf("event for block %v, lower than first block %v", blockNumber, lowestBlockNumber)
					} else if blockNumber > highestBlockNumber {
						highestBlockNumber = blockNumber
					}

					switch b.Event {
					case ethmonitor.Added:
						if blockNumber != latestBlockNumber+1 {
							return fmt.Errorf("added block %v, but most recent block was %v", blockNumber, latestBlockNumber)
						}
						latestBlockNumber = blockNumber
					case ethmonitor.Removed:
						if blockNumber != latestBlockNumber {
							return fmt.Errorf("removed block %v, but most recent block was %v", blockNumber, latestBlockNumber)
						}
						latestBlockNumber = blockNumber - 1
						// case ethmonitor.Updated:
						// 	if blockNumber > latestBlockNumber {
						// 		return fmt.Errorf("updated block %v, but it was never added", blockNumber)
						// 	}
					}
				}

				if err := checkBlockEvent(b); err != nil {
					return err
				}
			}
		}

		// every block number from lowestBlockNumber to highestBlockNumber must have been added
		for i := lowestBlockNumber; i <= highestBlockNumber; i++ {
			if _, wasFound := addedBlockNumbers[i]; !wasFound {
				return fmt.Errorf("expected to see block %v, but not found", i)
			}
		}

		// make sure the number of blocks added is the size of the range of added block numbers
		if uint64(len(addedBlockNumbers)) != highestBlockNumber-lowestBlockNumber+1 {
			return fmt.Errorf("expected %v block numbers, got %v", highestBlockNumber-lowestBlockNumber+1, len(addedBlockNumbers))
		}

		return nil
	}
	_ = feedCheck

	if err := feedCheck(); err != nil {
		return err
	}

	return nil
}

func getFirstBlock(feed []ethmonitor.Blocks) (*types.Block, error) {
	for _, f := range feed {
		for _, b := range f {
			if b.Event == ethmonitor.Added {
				return b.Block, nil
			}
		}
	}
	return nil, errors.New("first block not found, unexpected!")
}
