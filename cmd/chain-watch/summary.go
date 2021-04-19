package main

import (
	"fmt"

	"github.com/0xsequence/ethkit/ethmonitor"
)

type summary struct {
	feed []ethmonitor.Blocks

	countAdded   int
	countRemoved int
	countUpdated int

	blockNumAdded   []uint64
	blockNumRemoved []uint64
	blockNumUpdated []uint64
}

func printSummary(feed []ethmonitor.Blocks) {
	summary := &summary{feed: feed}

	for _, blocks := range feed {
		for _, b := range blocks {
			switch b.Type {
			case ethmonitor.Added:
				summary.countAdded += 1
				summary.blockNumAdded = append(summary.blockNumAdded, b.NumberU64())
				break
			case ethmonitor.Removed:
				summary.countRemoved += 1
				summary.blockNumRemoved = append(summary.blockNumRemoved, b.NumberU64())
				break
			case ethmonitor.Updated:
				summary.countUpdated += 1
				summary.blockNumUpdated = append(summary.blockNumUpdated, b.NumberU64())
				break
			}
		}
	}

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

	// TODO: lets analyze, and lets ensure all added/removed make sense..
}

func analyzeSummary(summary *summary) {
	feed := summary.feed
	_ = feed

	// TODO: here we check that removed blocks,
}
