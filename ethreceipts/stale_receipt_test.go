package ethreceipts

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func TestReceiptsStaleRPCRecoversCurrentCandidate(t *testing.T) {
	for _, pendingFirst := range []bool{false, true} {
		name := "initial_response"
		if pendingFirst {
			name = "pending_retry"
		}
		t.Run(name, func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 6000)
			a, b := hardeningBlock(100, tx), hardeningBlock(102, tx)
			var current atomic.Pointer[types.Receipt]
			current.Store(hardeningReceipt(a, tx))
			var providerError atomic.Bool
			var calls atomic.Int32
			p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
				calls.Add(1)
				if providerError.Load() {
					return nil, errors.New("temporary provider failure")
				}
				return current.Load(), nil
			}}
			l := hardeningListener(t, p, hardeningOptions())
			q := FilterTxnHash(tx.Hash()).SearchCache(false).QueryOnChainTxnHash(false).Finalize(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			ctx := context.Background()
			hardeningProcess(t, l, s, a)
			mined := hardeningRead(t, s)
			if mined.BlockHash() != a.Hash() || mined.Reorged || mined.Final {
				t.Fatalf("wrong initial mined receipt: %+v", mined)
			}
			removed := *a
			removed.Event = ethmonitor.Removed
			hardeningProcess(t, l, s, &removed)
			rollback := hardeningRead(t, s)
			if !rollback.Reorged || rollback.BlockHash() != a.Hash() {
				t.Fatalf("wrong rollback notification: %+v", rollback)
			}
			providerError.Store(pendingFirst)
			hardeningProcess(t, l, s, b)
			providerError.Store(false)
			pendingCount := func() int {
				s.retryMu.Lock()
				defer s.retryMu.Unlock()
				return len(s.pendingReceipts)
			}
			makeDue := func() {
				s.retryMu.Lock()
				defer s.retryMu.Unlock()
				for _, pending := range s.pendingReceipts {
					pending.nextRetryAt = time.Now().Add(-time.Second)
				}
			}
			if pendingFirst {
				if pendingCount() != 1 {
					t.Fatal("provider failure did not queue B")
				}
				makeDue()
				hardeningProcess(t, l, s, hardeningBlock(103))
			}
			if pendingCount() != 1 {
				t.Errorf("stale A response discarded current B candidate: pending=%d", pendingCount())
			}
			if _, found, _ := l.pastReceipts.Get(ctx, tx.Hash().Hex()); found {
				t.Fatal("stale A receipt was cached")
			}
			hardeningNoReceipt(t, s)

			// The provider recovers; only an unrelated later block triggers retry.
			canonical := hardeningReceipt(b, tx)
			current.Store(canonical)
			makeDue()
			callsBefore := calls.Load()
			hardeningProcess(t, l, s, hardeningBlock(104))
			if calls.Load() <= callsBefore {
				t.Fatal("provider recovered but B was never retried")
			}
			mined = hardeningRead(t, s)
			if mined.BlockHash() != b.Hash() || mined.TransactionHash() != tx.Hash() || mined.Reorged || mined.Final || mined.Filter != q {
				t.Fatalf("wrong recovered mined receipt: %+v", mined)
			}
			hardeningProcess(t, l, s, hardeningBlock(105))
			final := hardeningRead(t, s)
			if final.BlockHash() != b.Hash() || final.TransactionHash() != tx.Hash() || final.Reorged || !final.Final || final.Filter != q {
				t.Fatalf("wrong recovered final receipt: %+v", final)
			}
			if pendingCount() != 0 {
				t.Error("delivered B remained pending")
			}
			if cached, found, _ := l.pastReceipts.Get(ctx, tx.Hash().Hex()); !found || cached != canonical {
				t.Error("canonical B receipt was not cached")
			}
			hardeningNoReceipt(t, s)
		})
	}
}

func TestReceiptsStaleRPCKeepsObsoleteWorkTerminal(t *testing.T) {
	t.Run("expected_generation_removed_during_fetch", func(t *testing.T) {
		tx, _, _ := hardeningTxn(t, 6001)
		b := hardeningBlock(100, tx)
		entered, release := make(chan struct{}), make(chan struct{})
		p := &hardeningProvider{receipt: func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
			close(entered)
			select {
			case <-release:
				return hardeningReceipt(b, tx), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}
		l := hardeningListener(t, p, hardeningOptions(), b)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := l.fetchTransactionReceipt(ctx, tx.Hash(), true, blockRef{b.Hash(), 0})
			done <- err
		}()
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("RPC did not start")
		}
		l.invalidateBlock(ctx, b)
		l.acceptBlock(b)
		if !l.isCurrentBlock(b.Hash(), 1) {
			t.Fatal("same-hash re-adoption did not advance the generation")
		}
		fresh := hardeningReceipt(b, tx)
		l.pastReceipts.Set(ctx, tx.Hash().Hex(), fresh)
		close(release)
		if err := <-done; !errors.Is(err, ethereum.NotFound) {
			t.Fatalf("obsolete expected generation became retryable: %v", err)
		}
		if cached, found, _ := l.pastReceipts.Get(ctx, tx.Hash().Hex()); !found || cached != fresh {
			t.Error("obsolete completion replaced the current cache entry")
		}
	})
	t.Run("hash_query_without_current_candidate", func(t *testing.T) {
		tx, _, _ := hardeningTxn(t, 6002)
		a := hardeningBlock(100, tx)
		p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
			return hardeningReceipt(a, tx), nil
		}}
		l := hardeningListener(t, p, hardeningOptions(), a)
		ctx := context.Background()
		l.invalidateBlock(ctx, a)
		if _, err := l.fetchTransactionReceipt(ctx, tx.Hash(), true); !errors.Is(err, ethereum.NotFound) {
			t.Fatalf("unscoped orphan query became retryable: %v", err)
		}
		if _, found, _ := l.pastReceipts.Get(ctx, tx.Hash().Hex()); found {
			t.Error("unscoped orphan query cached stale data")
		}
	})
}
