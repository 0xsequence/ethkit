package ethreceipts

import (
	"math/big"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum/common"
)

type finalizer struct {
	queue []finalTxn
	txns  map[common.Hash]struct{}
	mu    sync.Mutex
}

type finalTxn struct {
	receipt  Receipt
	blockNum big.Int
}

func (f *finalizer) enqueue(receipt Receipt, blockNum big.Int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	txnHash := receipt.Transaction.Hash()

	if _, ok := f.txns[txnHash]; ok {
		// update the blockNum ...
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
}
