package ethreceipts

import (
	"math/big"
	"sync"

	"github.com/0xsequence/ethkit/go-ethereum/common"
)

// Public filter IDs are labels; distinct filters can share one label.
type receiptKey struct {
	txnHash    common.Hash
	blockHash  common.Hash
	generation uint64
	owner      *filterOwner
}

func receiptOwner(receipt Receipt, owner *filterOwner) receiptKey {
	return receiptKey{receipt.TransactionHash(), receipt.BlockHash(), receipt.generation, owner}
}

type finalizer struct {
	queue               []finalTxn
	txns                map[receiptKey]struct{}
	numBlocksToFinality *big.Int
	mu                  sync.Mutex
}
type finalTxn struct {
	receipt  Receipt
	blockNum *big.Int
}

func (f *finalizer) setFinality(num int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.numBlocksToFinality = big.NewInt(int64(num))
}
func (f *finalizer) enqueue(owner *filterOwner, receipt Receipt, blockNum *big.Int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if receipt.Final || blockNum == nil {
		return
	}
	key := receiptOwner(receipt, owner)
	if _, ok := f.txns[key]; ok {
		for i, entry := range f.queue {
			if receiptOwner(entry.receipt, entry.receipt.owner) == key {
				f.queue[i] = finalTxn{receipt, new(big.Int).Set(blockNum)}
				return
			}
		}
	}
	f.queue = append(f.queue, finalTxn{receipt, new(big.Int).Set(blockNum)})
	f.txns[key] = struct{}{}
}
func (f *finalizer) invalidateBlock(block blockRef) []Receipt {
	f.mu.Lock()
	defer f.mu.Unlock()
	var invalidated []Receipt
	retained := f.queue[:0]
	for _, txn := range f.queue {
		if txn.receipt.BlockHash() == block.hash && txn.receipt.generation == block.generation {
			invalidated = append(invalidated, txn.receipt)
			delete(f.txns, receiptOwner(txn.receipt, txn.receipt.owner))
		} else {
			retained = append(retained, txn)
		}
	}
	clear(f.queue[len(retained):])
	f.queue = retained
	return invalidated
}
func (f *finalizer) invalidateOwner(owner *filterOwner) {
	f.mu.Lock()
	defer f.mu.Unlock()
	retained := f.queue[:0]
	for _, txn := range f.queue {
		if txn.receipt.owner == owner {
			delete(f.txns, receiptOwner(txn.receipt, owner))
		} else {
			retained = append(retained, txn)
		}
	}
	clear(f.queue[len(retained):])
	f.queue = retained
}
func (f *finalizer) hasOwner(owner *filterOwner) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, txn := range f.queue {
		if txn.receipt.owner == owner {
			return true
		}
	}
	return false
}
func (f *finalizer) findOwner(filter Filterer) *filterOwner {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, txn := range f.queue {
		if sameFilter(txn.receipt.Filter, filter) {
			return txn.receipt.owner
		}
	}
	return nil
}
func (f *finalizer) clear() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = nil
	clear(f.txns)
}
func (f *finalizer) dequeue(currentBlockNum *big.Int) []finalTxn {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Zero is unresolved until Run selects the network's finality policy.
	if f.numBlocksToFinality == nil || f.numBlocksToFinality.Sign() <= 0 {
		return nil
	}
	var finalized []finalTxn
	retained := f.queue[:0]
	for _, txn := range f.queue {
		if currentBlockNum.Cmp(new(big.Int).Add(txn.blockNum, f.numBlocksToFinality)) >= 0 {
			finalized = append(finalized, txn)
			delete(f.txns, receiptOwner(txn.receipt, txn.receipt.owner))
		} else {
			retained = append(retained, txn)
		}
	}
	clear(f.queue[len(retained):])
	f.queue = retained
	return finalized
}
