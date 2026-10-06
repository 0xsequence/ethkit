package ethreceipts

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func TestHardeningQueryOnChainUsesCompleteReceipt(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 9)
	b := hardeningBlock(90, tx)
	complete := hardeningReceipt(b, tx)
	var calls atomic.Int32
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
		calls.Add(1)
		return nil, ethereum.NotFound
	}}
	l := hardeningListener(t, p, hardeningOptions())
	s := l.Subscribe(FilterLogs(func([]*types.Log) bool { return true }).QueryOnChain(func(context.Context) (*types.Receipt, error) { return complete, nil })).(*subscriber)
	defer s.Unsubscribe()
	if err := l.queryFilterOnChain(context.Background(), s, s.Filters()); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-s.TransactionReceipt():
		if r.Receipt() != complete || r.BlockHash() != b.Hash() {
			t.Fatal("callback receipt replaced")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("complete callback receipt was not delivered")
	}
	if calls.Load() != 0 {
		t.Errorf("complete callback caused %d redundant RPCs", calls.Load())
	}
}

func TestHardeningFetchPreservesSharedQuery(t *testing.T) {
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true })
	s := l.Subscribe(q)
	defer s.Unsubscribe()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, _ = l.FetchTransactionReceiptWithFilter(ctx, q) }()
	}
	wg.Wait()
	opts := s.Filters()[0].Options()
	if opts.LimitOne || opts.SearchCache {
		t.Fatalf("Fetch rewrote active subscription: %+v", opts)
	}
}

type ownershipGateFilter struct {
	Filterer
	entered, release chan struct{}
	calls            atomic.Int32
}

func (f *ownershipGateFilter) Match(ctx context.Context, _ Receipt) (bool, error) {
	if f.calls.Add(1) == 1 {
		close(f.entered)
		select {
		case <-f.release:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	return false, nil
}

func TestOwnershipStaleFetchPreservesCache(t *testing.T) {
	for _, sameHash := range []bool{false, true} {
		t.Run(map[bool]string{false: "remined_block", true: "readopted_generation"}[sameHash], func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 107)
			old, canonical := hardeningBlock(100, tx), hardeningBlock(102, tx)
			if sameHash {
				canonical = old
			}
			complete := hardeningReceipt(canonical, tx)
			var calls atomic.Int32
			var unavailable atomic.Bool
			p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
				calls.Add(1)
				if unavailable.Load() {
					return nil, ethereum.NotFound
				}
				return complete, nil
			}}
			l := hardeningListener(t, p, hardeningOptions())
			q := FilterTxnHash(tx.Hash()).SearchCache(false).QueryOnChainTxnHash(false)
			gate := &ownershipGateFilter{Filterer: FilterLogs(func([]*types.Log) bool { return false }).(Filterer), entered: make(chan struct{}), release: make(chan struct{})}
			s := l.Subscribe(q, gate).(*subscriber)
			defer s.Unsubscribe()
			done := make(chan error, 1)
			go func() {
				_, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{transaction: tx, blockHash: old.Hash(), blockNum: old.Number()}})
				done <- err
			}()
			<-gate.entered
			removed := *old
			removed.Event = ethmonitor.Removed
			hardeningProcess(t, l, s, &removed)
			hardeningRead(t, s)
			if sameHash {
				if _, err := l.processBlocks(context.Background(), ethmonitor.Blocks{canonical}, []*subscriber{s}, [][]Filterer{s.Filters()}); err != nil {
					t.Fatal(err)
				}
			} else {
				hardeningProcess(t, l, s, canonical)
			}
			hardeningRead(t, s)
			cached, found, _ := l.pastReceipts.Get(context.Background(), tx.Hash().Hex())
			if !found || cached != complete {
				t.Fatal("canonical receipt was not cached")
			}
			close(gate.release)
			<-done
			cached, found, _ = l.pastReceipts.Get(context.Background(), tx.Hash().Hex())
			if !found || cached != complete {
				t.Error("late collection destroyed canonical cache")
			}
			unavailable.Store(true)
			if _, err := l.fetchTransactionReceipt(context.Background(), tx.Hash(), true, blockRef{old.Hash(), 0}); !errors.Is(err, ethereum.NotFound) {
				t.Errorf("stale request error: %v", err)
			}
			cached, found, _ = l.pastReceipts.Get(context.Background(), tx.Hash().Hex())
			if !found || cached != complete {
				t.Error("stale expected block/generation deleted current cache")
			}
			r, err := l.fetchTransactionReceipt(context.Background(), tx.Hash(), false)
			if err != nil || r != complete {
				t.Errorf("fresh cache query depended on unavailable origin: %v", err)
			}
			if calls.Load() != 1 {
				t.Errorf("stale work refetched a current canonical result: calls=%d", calls.Load())
			}
		})
	}
}
