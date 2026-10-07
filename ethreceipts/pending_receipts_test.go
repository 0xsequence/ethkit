package ethreceipts

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func TestHardeningPendingOwners(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 6)
	b := hardeningBlock(100, tx)
	var failing atomic.Bool
	failing.Store(true)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
		if failing.Load() {
			return nil, errors.New("temporary provider failure")
		}
		return hardeningReceipt(b, tx), nil
	}}
	l := hardeningListener(t, p, hardeningOptions())
	q1 := FilterLogs(func([]*types.Log) bool { return true }).ID(0).Finalize(true)
	q2 := FilterTxnHash(tx.Hash()).ID(0)
	s := l.Subscribe(q1, q2).(*subscriber)
	defer s.Unsubscribe()
	_, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{transaction: tx, chainID: big.NewInt(1)}})
	if err == nil {
		t.Fatal("expected transient fetch failure")
	}
	failing.Store(false)
	s.retryMu.Lock()
	for _, pending := range s.pendingReceipts {
		pending.nextRetryAt = time.Time{}
	}
	s.retryMu.Unlock()
	s.retryPendingReceipts(context.Background())
	owners := map[Filterer]bool{}
	for i := 0; i < 2; i++ {
		select {
		case r := <-s.TransactionReceipt():
			if r.Final || r.BlockHash() != b.Hash() {
				t.Errorf("wrong recovered receipt: %+v", r)
			}
			owners[r.Filter] = true
		case <-time.After(100 * time.Millisecond):
			t.Error("missing recovered owner")
		}
	}
	if !owners[q1.(Filterer)] || !owners[q2.(Filterer)] {
		t.Error("retry lost an owner")
	}
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case r := <-s.TransactionReceipt():
			if !r.Final || r.BlockHash() != b.Hash() {
				t.Errorf("wrong recovered final: %+v", r)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("missing recovered final")
		}
	}
}

func TestOwnershipPendingDropReleasesClaim(t *testing.T) {
	for _, notFound := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry_limit", true: "not_found"}[notFound], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 108)
			tx2, _, _ := hardeningTxn(t, 109)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			p := &hardeningProvider{receipt: func(_ context.Context, h common.Hash) (*types.Receipt, error) {
				if h == tx2.Hash() {
					return hardeningReceipt(b2, tx2), nil
				}
				return nil, errors.New("temporary receipt failure")
			}}
			l := hardeningListener(t, p, hardeningOptions())
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{transaction: tx1, blockHash: b1.Hash(), blockNum: b1.Number()}}); err == nil {
				t.Fatal("expected initial pending fetch failure")
			}
			if len(s.pendingReceipts) != 1 || len(s.claims) != 1 {
				t.Fatal("pending fetch lost its selection")
			}
			for _, pending := range s.pendingReceipts {
				pending.nextRetryAt = time.Time{}
				pending.attempts = maxReceiptRetryAttempts - 1
			}
			if notFound {
				p.receipt = func(_ context.Context, h common.Hash) (*types.Receipt, error) {
					if h == tx2.Hash() {
						return hardeningReceipt(b2, tx2), nil
					}
					return nil, ethereum.NotFound
				}
			}
			s.retryPendingReceipts(context.Background())
			if len(s.pendingReceipts) != 0 || len(s.claims) != 0 {
				t.Error("discarded pending work retained an undelivered claim")
			}
			hardeningProcess(t, l, s, b2)
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx2.Hash() || r.Filter != q {
					t.Error("released pending claim selected wrong transaction")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("discarded pending work blocked a new transaction")
			}
		})
	}
}

func TestReceiptsFixRetryDeadlineAfterFetch(t *testing.T) {
	for _, terminal := range []string{"retry", "remove", "clear", "rollback"} {
		t.Run(terminal, func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 800)
			b := hardeningBlock(100, tx)
			var calls atomic.Int32
			p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
				calls.Add(1)
				return hardeningReceipt(b, tx), nil
			}}
			l := hardeningListener(t, p, hardeningOptions(), b)
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			owner := s.owner(q.(Filterer))
			seed := Receipt{transaction: tx, blockHash: b.Hash(), blockNum: b.Number()}
			if !s.reserve(seed, owner) {
				t.Fatal("initial reservation failed")
			}
			s.addPendingReceipt(seed, owner)
			s.releaseReservation(seed, owner)
			key := receiptOwner(seed, owner)
			s.pendingReceipts[key].nextRetryAt = time.Time{}
			pending := s.pendingReceipts[key]
			parent := context.Background()
			attempt, cancel := context.WithTimeout(parent, 200*time.Millisecond)
			defer cancel()
			s.deliveryMu.Lock()
			done := make(chan struct{})
			go func() { s.retryPendingReceipts(attempt); close(done) }()
			deadline := time.Now().Add(time.Second)
			for {
				_, cached, _ := l.pastReceipts.Get(parent, tx.Hash().Hex())
				if cached && len(l.fetchSem) == 0 {
					break
				}
				if time.Now().After(deadline) {
					s.deliveryMu.Unlock()
					t.Fatal("receipt fetch did not complete before publication")
				}
				time.Sleep(time.Millisecond)
			}
			<-attempt.Done()
			terminalDone := make(chan struct{})
			go func() {
				defer close(terminalDone)
				switch terminal {
				case "remove":
					s.RemoveFilter(q.(Filterer))
				case "clear":
					s.ClearFilters()
				case "rollback":
					ref := l.invalidateBlock(parent, b)
					s.rollbackBlock(ref)
				}
			}()
			s.deliveryMu.Unlock()
			<-done
			<-terminalDone
			hardeningNoReceipt(t, s)
			if parent.Err() != nil {
				t.Fatal("retry child canceled its listener parent")
			}
			if terminal == "retry" {
				if s.pendingReceipts[key] != pending || len(s.claims) != 1 || len(s.deliveries) != 0 || len(s.finalizer.queue) != 0 {
					t.Errorf("expired successful retry lost exact ownership: pending=%d claims=%d", len(s.pendingReceipts), len(s.claims))
				}
				if s.pendingReceipts[key] == nil {
					return
				}
				if pending.attempts != 1 || pending.nextRetryAt.After(time.Now().Add(time.Second)) {
					t.Error("successful expired attempt was not promptly released for retry")
				}
				pending.nextRetryAt = time.Time{}
				s.retryPendingReceipts(parent)
				r := hardeningRead(t, s)
				if r.TransactionHash() != tx.Hash() || r.BlockHash() != b.Hash() || r.Filter != q || r.Final || r.Reorged {
					t.Error("later active retry lost mined identity")
				}
				if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
					t.Fatal(err)
				}
				r = hardeningRead(t, s)
				if !r.Final || r.Reorged || r.TransactionHash() != tx.Hash() || r.BlockHash() != b.Hash() || r.Filter != q {
					t.Error("later active retry lost finality identity")
				}
				if calls.Load() != 1 {
					t.Error("later retry discarded its successful cached receipt")
				}
			} else {
				if len(s.pendingReceipts) != 0 || len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 || len(s.finalizer.queue) != 0 {
					t.Error("expired retry resurrected terminal owner/block state")
				}
				s.retryPendingReceipts(parent)
				if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
					t.Fatal(err)
				}
			}
			hardeningNoReceipt(t, s)
		})
	}
}

func TestReceiptsFixPartialPendingCandidate(t *testing.T) {
	for _, terminal := range []string{"retry", "rollback", "readoption", "remove", "clear"} {
		t.Run(terminal, func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 808)
			replacement, _, _ := hardeningTxn(t, 809)
			old, next := hardeningBlock(100, tx), hardeningBlock(101, replacement)
			var ready atomic.Bool
			var calls atomic.Int32
			l := hardeningListener(t, &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
				calls.Add(1)
				if !ready.Load() {
					return nil, errors.New("provider temporarily unavailable")
				}
				return hardeningReceipt(old, tx), nil
			}}, hardeningOptions(), old)
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true).
				QueryOnChain(func(context.Context) (*types.Receipt, error) { return &types.Receipt{TxHash: tx.Hash()}, nil })
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			owner := s.owner(q.(Filterer))
			if err := l.queryFilterOnChain(context.Background(), s, s.filterers()); err != nil {
				t.Fatal(err)
			}
			if len(s.pendingReceipts) != 1 || s.claims[owner] != tx.Hash() {
				t.Fatal("partial public callback did not enter the pending completion path")
			}
			var key receiptKey
			var pending *pendingReceipt
			for k, p := range s.pendingReceipts {
				key, pending = k, p
			}
			if key.blockHash != (common.Hash{}) {
				t.Fatal("partial callback already knew its block")
			}
			pending.nextRetryAt = time.Time{}
			ready.Store(true)
			attempt, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			s.deliveryMu.Lock()
			done := make(chan struct{})
			go func() { s.retryPendingReceipts(attempt); close(done) }()
			deadline := time.Now().Add(time.Second)
			for {
				_, cached, _ := l.pastReceipts.Get(context.Background(), tx.Hash().Hex())
				if cached && len(l.fetchSem) == 0 {
					break
				}
				if time.Now().After(deadline) {
					s.deliveryMu.Unlock()
					t.Fatal("partial pending fetch did not complete")
				}
				time.Sleep(time.Millisecond)
			}
			<-attempt.Done()
			s.deliveryMu.Unlock()
			<-done
			hardeningNoReceipt(t, s)
			if s.pendingReceipts[key] != pending || pending.attempts != 1 || calls.Load() != 2 {
				t.Fatal("successful canceled completion did not retain its exact pending owner")
			}
			if terminal == "retry" {
				pending.nextRetryAt = time.Time{}
				s.retryPendingReceipts(context.Background())
				mined := hardeningRead(t, s)
				if mined.Filter != q || mined.TransactionHash() != tx.Hash() || mined.BlockHash() != old.Hash() || mined.generation != 0 || mined.Final || mined.Reorged {
					t.Fatal("partial pending live retry lost its learned candidate")
				}
				if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
					t.Fatal(err)
				}
				final := hardeningRead(t, s)
				if final.Filter != q || final.TransactionHash() != tx.Hash() || final.BlockHash() != old.Hash() || !final.Final || final.Reorged || calls.Load() != 2 {
					t.Fatal("partial retry lost cached completion or canonical finality")
				}
				hardeningNoReceipt(t, s)
				return
			}
			switch terminal {
			case "rollback", "readoption":
				ref := l.invalidateBlock(context.Background(), old)
				if terminal == "readoption" {
					// Even if authoritative same-hash acceptance wins before subscriber
					// cleanup, the learned generation-0 candidate must stay invalid.
					l.acceptBlock(l.monitor.GetBlock(old.Hash()))
					pending.nextRetryAt = time.Time{}
					s.retryPendingReceipts(context.Background())
					hardeningNoReceipt(t, s)
				}
				s.rollbackBlock(ref)
			case "remove":
				s.RemoveFilter(q.(Filterer))
			case "clear":
				s.ClearFilters()
			}
			if len(s.pendingReceipts) != 0 || len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 || len(s.finalizer.queue) != 0 {
				t.Error("terminal invalidation retained a learned pending candidate or selection")
			}
			s.retryPendingReceipts(context.Background())
			hardeningNoReceipt(t, s)
			if terminal == "remove" || terminal == "clear" {
				s.AddFilter(q)
				if s.owner(q.(Filterer)) == owner {
					t.Fatal("new registration reused the canceled owner")
				}
			}
			if _, err := s.matchFiltersAndPublish(context.Background(), s.filterers(), []Receipt{{receipt: hardeningReceipt(next, replacement)}}); err != nil {
				t.Fatal(err)
			}
			select {
			case mined := <-s.TransactionReceipt():
				if mined.Filter != q || mined.TransactionHash() != replacement.Hash() || mined.BlockHash() != next.Hash() || mined.Final || mined.Reorged {
					t.Error("fresh replacement lost its exact mined identity")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("orphan pending claim blocked a fresh canonical replacement")
				return
			}
			if err := s.finalizeReceipts(big.NewInt(104)); err != nil {
				t.Fatal(err)
			}
			final := hardeningRead(t, s)
			if final.Filter != q || final.TransactionHash() != replacement.Hash() || final.BlockHash() != next.Hash() || !final.Final || final.Reorged {
				t.Error("terminal invalidation published an orphan final")
			}
			if len(s.pendingReceipts) != 0 || len(s.claims) != 0 || len(s.Filters()) != 0 || calls.Load() != 2 {
				t.Error("terminal candidate or completed owner was retained/re-fetched")
			}
			hardeningNoReceipt(t, s)
		})
	}
}
