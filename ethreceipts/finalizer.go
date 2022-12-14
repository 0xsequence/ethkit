package ethreceipts

import (
	"math/big"
	"sort"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type finalizer struct {
	queue               []finalTxn
	txns                map[common.Hash]struct{}
	numBlocksToFinality *big.Int
	mu                  sync.Mutex
}

type finalTxn struct {
	receipt  Receipt
	blockNum big.Int
}

func (f *finalizer) len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queue)
}

func (f *finalizer) lastBlockNum() big.Int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queue) == 0 {
		return *big.NewInt(0)
	}
	return f.queue[0].blockNum
}

func (f *finalizer) enqueue(receipt Receipt, blockNum big.Int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	txnHash := receipt.Transaction.Hash()

	if _, ok := f.txns[txnHash]; ok {
		// update the blockNum if we already have this txn, as it could have been included
		// again after a reorg in a new block
		for i, entry := range f.queue {
			if entry.receipt.Transaction.Hash() == txnHash {
				f.queue[i] = finalTxn{receipt, blockNum}
			}
		}
		return
	}

	// append new
	f.queue = append(f.queue, finalTxn{receipt, blockNum})
	f.txns[txnHash] = struct{}{}

	// sort block order from oldest to newest in case of a reorg
	if len(f.queue) >= 2 && f.queue[0].blockNum.Cmp(&f.queue[1].blockNum) < 0 {
		sort.SliceStable(f.queue, func(i, j int) bool {
			return f.queue[i].blockNum.Cmp(&f.queue[j].blockNum) < 0
		})
	}
}

func (f *finalizer) dequeue(currentBlockNum *big.Int) []finalTxn {
	f.mu.Lock()
	defer f.mu.Unlock()

	finalTxns := []finalTxn{}

	for _, txn := range f.queue {
		if currentBlockNum.Cmp(big.NewInt(0).Add(&txn.blockNum, f.numBlocksToFinality)) > 0 {
			finalTxns = append(finalTxns, txn)
		}
	}

	if len(finalTxns) > 0 {
		f.queue = f.queue[len(finalTxns):]
	}

	return finalTxns
}
