package ethfinalizer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// Mempool is a complete local store of all transactions signed by a specific wallet for a specific chain that have been sent, but not necessarily included on chain.
type Mempool[T any] interface {
	// Nonce is the next available nonce for the wallet on the chain.
	Nonce(ctx context.Context) (uint64, error)

	// Commit persists the signed transaction with its metadata in the store.
	// The transaction must be persisted with timestamps of the first and latest submissions.
	// If the transaction already exists in the mempool, the latest timestamp must be updated.
	Commit(ctx context.Context, transaction *types.Transaction, metadata T) error

	// SetError sets the error for a previously committed transaction.
	SetError(ctx context.Context, transaction common.Hash, err error) error

	// Transactions returns the transactions for the specified hashes which are signed by this specific wallet for this specific chain.
	Transactions(ctx context.Context, hashes map[common.Hash]struct{}) (map[common.Hash]*Transaction[T], error)

	// PriciestTransactions returns, by nonce, the most expensive transactions signed by this specific wallet for this specific chain, with a minimum nonce and a latest timestamp.
	PriciestTransactions(ctx context.Context, fromNonce uint64, before time.Time) (map[uint64]*Transaction[T], error)

	// Status returns the statuses for the first and latest transactions for a given nonce.
	Status(ctx context.Context, nonce uint64) (*Status[T], *Status[T], error)
}

type memoryMempool[T any] struct {
	transactions map[common.Hash]*Status[T]
	nonces       map[uint64]*nonceStatus[T]
	highestNonce *uint64
	mu           sync.RWMutex
}

type nonceStatus[T any] struct {
	first, latest *Status[T]
	time          time.Time
}

// NewMemoryMempool creates a minimal in-memory Mempool.
func NewMemoryMempool[T any]() Mempool[T] {
	return &memoryMempool[T]{
		transactions: map[common.Hash]*Status[T]{},
		nonces:       map[uint64]*nonceStatus[T]{},
	}
}

func (m *memoryMempool[T]) Nonce(ctx context.Context) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.highestNonce == nil {
		return 0, nil
	} else {
		return *m.highestNonce + 1, nil
	}
}

func (m *memoryMempool[T]) Commit(ctx context.Context, transaction *types.Transaction, metadata T) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	status_ := Status[T]{
		Transaction: &Transaction[T]{
			Transaction: transaction,
			Metadata:    metadata,
		},
		Time: now,
	}
	m.transactions[transaction.Hash()] = &status_

	status := m.nonces[transaction.Nonce()]
	if status == nil {
		status = &nonceStatus[T]{
			first:  &status_,
			latest: &status_,
			time:   now,
		}
		m.nonces[transaction.Nonce()] = status

		if m.highestNonce == nil || transaction.Nonce() > *m.highestNonce {
			m.highestNonce = new(uint64)
			*m.highestNonce = transaction.Nonce()
		}
	}

	if transaction.Hash() == status.latest.Transaction.Hash() {
		status.time = now
	} else if transaction.GasFeeCapCmp(status.latest.Transaction.Transaction) > 0 && transaction.GasTipCapCmp(status.latest.Transaction.Transaction) > 0 {
		status.latest = &status_
		status.time = now
	}

	return nil
}

func (m *memoryMempool[T]) SetError(ctx context.Context, transaction common.Hash, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := m.transactions[transaction]
	if status == nil {
		return fmt.Errorf("unknown transaction %v", transaction)
	}

	status.Error = err
	return nil
}

func (m *memoryMempool[T]) Transactions(ctx context.Context, hashes map[common.Hash]struct{}) (map[common.Hash]*Transaction[T], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	transactions := make(map[common.Hash]*Transaction[T], len(hashes))
	for hash := range hashes {
		status := m.transactions[hash]
		if status != nil {
			transactions[hash] = status.Transaction
		}
	}

	return transactions, nil
}

func (m *memoryMempool[T]) PriciestTransactions(ctx context.Context, fromNonce uint64, before time.Time) (map[uint64]*Transaction[T], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var capacity uint64
	if m.highestNonce != nil && *m.highestNonce+1 > fromNonce {
		capacity = *m.highestNonce + 1 - fromNonce
	}

	transactions := make(map[uint64]*Transaction[T], capacity)
	for nonce := fromNonce; ; nonce++ {
		status := m.nonces[nonce]
		if status == nil || !status.time.Before(before) {
			break
		}

		transactions[nonce] = status.latest.Transaction
	}

	return transactions, nil
}

func (m *memoryMempool[T]) Status(ctx context.Context, nonce uint64) (*Status[T], *Status[T], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := m.nonces[nonce]
	if status == nil {
		return nil, nil, nil
	}

	return status.first, status.latest, nil
}
