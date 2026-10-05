package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func receiptsFixCollect(s Subscription) []Receipt {
	var receipts []Receipt
	timer := time.NewTimer(40 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case r := <-s.TransactionReceipt():
			receipts = append(receipts, r)
		case <-timer.C:
			return receipts
		}
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

func TestReceiptsFixCustomValueRemoval(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		for _, exhausted := range []bool{false, true} {
			for _, source := range []string{"original", "Filters", "Receipt.Filter"} {
				t.Run(fmt.Sprintf("pointer=%v/exhausted=%v/%s", pointer, exhausted, source), func(t *testing.T) {
					tx1, _, _ := hardeningTxn(t, 801)
					tx2, _, _ := hardeningTxn(t, 802)
					b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
					l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
					custom := func(label string) Filterer {
						return ownershipValueFilter{FilterLogs(func([]*types.Log) bool { return false }).Finalize(true).ID(7).(Filterer), []string{label}, func(context.Context, Receipt) (bool, error) { return label != "", nil }}
					}
					value := custom("selected").(ownershipValueFilter)
					var q Filterer = value
					if pointer {
						q = &value
					}
					other := custom("surviving")
					s := l.Subscribe(q, other).(*subscriber)
					defer s.Unsubscribe()
					first, second := s.filterers()[0].(*filterOwner), s.filterers()[1].(*filterOwner)
					fromFilters := s.Filters()[0]
					process := func(b *types.Receipt) {
						t.Helper()
						if _, err := s.matchFiltersAndPublish(context.Background(), s.filterers(), []Receipt{{receipt: b}}); err != nil {
							t.Fatal(err)
						}
					}
					process(hardeningReceipt(b1, tx1))
					var mined Receipt
					for i := 0; i < 2; i++ {
						r := hardeningRead(t, s)
						if r.owner == first {
							mined = r
							if pointer {
								if r.Filter != q || fromFilters != q {
									t.Fatal("public pointer identity changed")
								}
							} else if r.Filter.(ownershipValueFilter).Filterer != value.Filterer || fromFilters.(ownershipValueFilter).Filterer != value.Filterer {
								t.Fatal("original public custom value changed")
							}
						}
					}
					if exhausted {
						s.exhaustFilter(first)
						value.Filterer.(*filter).closeExhausted()
					}
					candidate := q
					switch source {
					case "Filters":
						candidate = fromFilters
					case "Receipt.Filter":
						candidate = mined.Filter
					}
					s.RemoveFilter(candidate)
					if s.hasFilter(first) || s.finalizer.hasOwner(first) || !s.hasFilter(second) || !s.finalizer.hasOwner(second) {
						t.Error("public removal did not isolate selected registration/finality")
					}
					process(hardeningReceipt(b2, tx2))
					late := receiptsFixCollect(s)
					if len(late) != 1 || late[0].owner != second || late[0].TransactionHash() != tx2.Hash() || late[0].Final || late[0].Reorged {
						t.Error("selected cancellation changed later surviving delivery")
					}
					if err := s.finalizeReceipts(big.NewInt(105)); err != nil {
						t.Fatal(err)
					}
					finals := receiptsFixCollect(s)
					seen := make(map[common.Hash]bool)
					for _, r := range finals {
						if r.owner != second || !r.Final || r.Reorged || seen[r.TransactionHash()] {
							t.Error("removed custom value still finalized or surviving final duplicated")
						}
						seen[r.TransactionHash()] = true
					}
					if len(finals) != 2 || !seen[tx1.Hash()] || !seen[tx2.Hash()] {
						t.Error("custom removal lost surviving finals")
					}
				})
			}
		}
	}
}

func TestReceiptsFixSharedBaseAliases(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 803)
	b := hardeningBlock(100, tx)
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	base := FilterLogs(func([]*types.Log) bool { return false }).Finalize(true).(Filterer)
	first := ownershipValueFilter{base, []string{"first"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
	second := ownershipValueFilter{base, []string{"second"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
	other := ownershipValueFilter{FilterLogs(func([]*types.Log) bool { return true }).Finalize(true).(Filterer), []string{"other"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
	s := l.Subscribe(first, second, other).(*subscriber)
	defer s.Unsubscribe()
	owners := s.filterers()
	if _, err := s.matchFiltersAndPublish(context.Background(), owners, []Receipt{{receipt: hardeningReceipt(b, tx)}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		hardeningRead(t, s)
	}
	// Shared base signals define aliases: old RemoveFilter removes the first
	// matching registration. Independent callback identities use distinct bases.
	s.RemoveFilter(second)
	if s.hasFilter(owners[0].(*filterOwner)) || s.finalizer.hasOwner(owners[0].(*filterOwner)) || !s.hasFilter(owners[1].(*filterOwner)) || !s.hasFilter(owners[2].(*filterOwner)) {
		t.Error("shared-base removal did not preserve first-match alias semantics")
	}
	s.RemoveFilter(s.Filters()[0])
	if s.hasFilter(owners[1].(*filterOwner)) || s.finalizer.hasOwner(owners[1].(*filterOwner)) || !s.finalizer.hasOwner(owners[2].(*filterOwner)) {
		t.Error("removing second base alias affected independent owner")
	}
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	finals := receiptsFixCollect(s)
	if len(finals) != 1 || finals[0].owner != owners[2] || !finals[0].Final || finals[0].TransactionHash() != tx.Hash() {
		t.Error("shared aliases canceled an unrelated queued final")
	}
}

type receiptsFixComparableFilter struct {
	Filterer
	label string
}

func (f receiptsFixComparableFilter) Match(context.Context, Receipt) (bool, error) {
	return f.label != "", nil
}

func TestReceiptsFixComparableSharedBaseIsolation(t *testing.T) {
	for _, pointer := range []bool{false, true} {
		t.Run(fmt.Sprintf("pointer=%v", pointer), func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 804)
			b := hardeningBlock(100, tx)
			l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
			base := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true).(Filterer)
			var first, second Filterer = receiptsFixComparableFilter{base, "first"}, receiptsFixComparableFilter{base, "second"}
			if pointer {
				first = &ownershipValueFilter{base, []string{"first"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
				second = &ownershipValueFilter{base, []string{"second"}, func(context.Context, Receipt) (bool, error) { return true, nil }}
			}
			s := l.Subscribe(first, second).(*subscriber)
			defer s.Unsubscribe()
			owners := s.filterers()
			if _, err := s.matchFiltersAndPublish(context.Background(), owners, []Receipt{{receipt: hardeningReceipt(b, tx)}}); err != nil {
				t.Fatal(err)
			}
			hardeningRead(t, s)
			hardeningRead(t, s)
			s.RemoveFilter(second)
			if !s.hasFilter(owners[0].(*filterOwner)) || !s.finalizer.hasOwner(owners[0].(*filterOwner)) || s.hasFilter(owners[1].(*filterOwner)) || s.finalizer.hasOwner(owners[1].(*filterOwner)) {
				t.Fatal("base fallback overrode definitive comparable identity")
			}
			if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
				t.Fatal(err)
			}
			finals := receiptsFixCollect(s)
			if len(finals) != 1 || finals[0].owner != owners[0] || !finals[0].Final {
				t.Fatal("comparable cancellation lost surviving owner")
			}
		})
	}
}

type receiptsFixNilSignalFilter struct{ ownershipValueFilter }

func (receiptsFixNilSignalFilter) Exhausted() <-chan struct{} { return nil }

func TestReceiptsFixCustomValueWithoutIdentity(t *testing.T) {
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	value := receiptsFixNilSignalFilter{ownershipValueFilter{FilterLogs(func([]*types.Log) bool { return true }).(Filterer), []string{"no identity"}, func(context.Context, Receipt) (bool, error) { return true, nil }}}
	other := FilterLogs(func([]*types.Log) bool { return true }).(Filterer)
	s := l.Subscribe(value, other).(*subscriber)
	defer s.Unsubscribe()
	s.RemoveFilter(s.Filters()[0])
	if len(s.Filters()) != 2 {
		t.Fatal("unidentifiable callback value removed another registration")
	}
	s.ClearFilters()
	s.AddFilter(&value, other)
	s.RemoveFilter(s.Filters()[0])
	if len(s.Filters()) != 1 || s.Filters()[0] != other {
		t.Fatal("pointer identity did not isolate custom nil-signal removal")
	}
}

func TestReceiptsFixRollbackPreservesMinedSelection(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 805)
	other, _, _ := hardeningTxn(t, 806)
	old, replacement, canonical := hardeningBlock(100, tx), hardeningBlock(101, other), hardeningBlock(102, tx)
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	owner := s.owner(q.(Filterer))
	process := func(b *types.Receipt) {
		t.Helper()
		if _, err := s.matchFiltersAndPublish(context.Background(), s.filterers(), []Receipt{{receipt: b}}); err != nil {
			t.Fatal(err)
		}
	}
	process(hardeningReceipt(old, tx))
	hardeningRead(t, s)
	s.rollbackBlock(l.invalidateBlock(context.Background(), old))
	rollback := hardeningRead(t, s)
	if s.claims[owner] != tx.Hash() || !rollback.Reorged || rollback.Filter != q || rollback.BlockHash() != old.Hash() {
		t.Fatal("rollback released an already-mined selection")
	}
	process(hardeningReceipt(replacement, other))
	hardeningNoReceipt(t, s)
	process(hardeningReceipt(canonical, tx))
	mined := hardeningRead(t, s)
	if mined.Reorged || mined.Final || mined.TransactionHash() != tx.Hash() || mined.BlockHash() != canonical.Hash() || mined.Filter != q {
		t.Fatal("retained selection did not re-mine correctly")
	}
	if err := s.finalizeReceipts(big.NewInt(105)); err != nil {
		t.Fatal(err)
	}
	final := hardeningRead(t, s)
	if !final.Final || final.Reorged || final.TransactionHash() != tx.Hash() || final.BlockHash() != canonical.Hash() || final.Filter != q {
		t.Fatal("retained mined selection finalized an orphan")
	}
	hardeningNoReceipt(t, s)
}

func TestReceiptsFixReadoptionAfterRetention(t *testing.T) {
	for _, scenario := range []string{"canonical_retention", "obsolete_incarnation", "later_removal", "alternate_retention"} {
		t.Run(scenario, func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 807)
			b := hardeningBlock(100, tx)
			old, canonical := hardeningReceipt(b, tx), hardeningReceipt(b, tx)
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			var releaseOnce sync.Once
			p := &ownershipLiveProvider{
				hardeningProvider: &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
					if calls.Add(1) == 1 {
						close(entered)
						// Deliberately finish an old RPC after its request timeout,
						// to verify invalidation of late provider completions.
						<-release
						return old, nil
					}
					return canonical, nil
				}},
				canonical: make(map[uint64]*types.Block), blocks: make(map[common.Hash]*types.Block),
			}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			mo := ethmonitor.DefaultOptions
			mo.Logger, mo.WithLogs, mo.Bootstrap = log, true, true
			mo.StreamingDisabled, mo.PrefetchConcurrency, mo.PollingInterval = true, 0, 5*time.Millisecond
			mo.BlockRetentionLimit = 50
			m, err := ethmonitor.NewMonitor(p, mo)
			if err != nil {
				t.Fatal(err)
			}
			if err = m.Chain().BootstrapFromBlocks(ethmonitor.Blocks{hardeningBlock(99), b}); err != nil {
				t.Fatal(err)
			}
			initialSnapshot := *m.LatestBlock()
			opts := hardeningOptions()
			opts.NumBlocksToFinality = 200
			l, err := NewReceiptsListener(log, p, m, opts)
			if err != nil {
				t.Fatal(err)
			}
			q := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			stream := m.Subscribe("delayed receipt events")
			defer stream.Unsubscribe()
			oldDone := make(chan error, 1)
			go func() {
				_, err := l.fetchTransactionReceipt(context.Background(), tx.Hash(), true, blockRef{b.Hash(), 0})
				oldDone <- err
			}()
			<-entered
			releaseOld := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(releaseOld)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- m.Run(ctx) }()
			t.Cleanup(func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Errorf("monitor Run: %v", err)
					}
				case <-time.After(3 * time.Second):
					t.Error("monitor did not stop")
				}
			})
			consume := func() ethmonitor.Blocks {
				t.Helper()
				select {
				case batch := <-stream.Blocks():
					return batch
				case <-time.After(10 * time.Second):
					t.Fatal("missing canonical monitor event")
					return nil
				}
			}
			process := func(batch ethmonitor.Blocks, cached bool) {
				t.Helper()
				var err error
				if cached {
					_, err = l.processCachedBlocksContext(context.Background(), batch, []*subscriber{s}, [][]Filterer{s.filterers()})
				} else {
					_, err = l.processBlocksContext(context.Background(), batch, []*subscriber{s}, [][]Filterer{s.filterers()})
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			fork := func(from, through, offset int64, parent common.Hash) {
				for num := from; num <= through; num++ {
					header := hardeningBlock(num).Header()
					header.BlockHash, header.ParentHash = common.BigToHash(big.NewInt(num+offset)), parent
					next := types.NewBlockWithHeader(header)
					p.advance(next)
					parent = next.Hash()
				}
			}
			waitHeight := func(height int64) {
				t.Helper()
				deadline := time.Now().Add(10 * time.Second)
				for m.LatestBlockNum().Int64() != height {
					if time.Now().After(deadline) {
						t.Fatal("monitor did not advance canonical height")
					}
					time.Sleep(time.Millisecond)
				}
			}
			// The first real reorg removes H, invalidating the held generation-0 RPC.
			fork(100, 101, 10000, hardeningBlock(99).Hash())
			process(consume(), false)
			rollback := hardeningRead(t, s)
			if !rollback.Reorged || rollback.Final || rollback.BlockHash() != b.Hash() || rollback.TransactionHash() != tx.Hash() || rollback.Filter != q {
				t.Fatal("wrong original removal identity")
			}
			process(ethmonitor.Blocks{&initialSnapshot}, true)
			hardeningNoReceipt(t, s)
			// Rebuild the same hash H in a fresh, owned monitor incarnation.
			p.advance(b.Block, hardeningBlock(101).Block, hardeningBlock(102).Block)
			readoption := consume()
			var added *ethmonitor.Block
			for _, event := range readoption {
				if event.Event == ethmonitor.Added && event.Hash() == b.Hash() {
					added = event
				}
			}
			if added == nil {
				t.Fatal("monitor omitted readopted incarnation")
			}
			incarnation, valid := added.CanonicalState()
			if incarnation == 0 || !valid {
				t.Fatal("Added input was not the owned canonical monitor event")
			}
			process(ethmonitor.Blocks{added}, true)
			hardeningNoReceipt(t, s)
			negative := scenario == "later_removal" || scenario == "alternate_retention"
			var removed *ethmonitor.Block
			if negative {
				fork(100, 103, 20000, hardeningBlock(99).Hash())
				for _, event := range consume() {
					if event.Event == ethmonitor.Removed && event.Hash() == b.Hash() {
						removed = event
					}
				}
				if removed == nil {
					t.Fatal("monitor omitted later removal")
				}
			}
			if scenario == "canonical_retention" {
				fork(103, 155, 0, hardeningBlock(102).Hash())
				waitHeight(155)
			} else if scenario == "alternate_retention" {
				fork(104, 155, 20000, common.BigToHash(big.NewInt(20103)))
				waitHeight(155)
			}
			if scenario == "canonical_retention" || scenario == "alternate_retention" {
				if m.GetBlock(b.Hash()) != nil || m.OldestBlockNum().Int64() != 106 {
					t.Fatal("H was not evicted more than one block behind retention")
				}
			}
			if scenario == "obsolete_incarnation" {
				// A removed tracked snapshot cannot fall back to a newer retained
				// incarnation of the same hash, even during authoritative processing.
				if m.GetBlock(b.Hash()) == nil {
					t.Fatal("newer same-hash incarnation is missing")
				}
				process(ethmonitor.Blocks{&initialSnapshot}, false)
				hardeningNoReceipt(t, s)
				if !l.blockStates[b.Hash()].removed {
					t.Error("obsolete tracked incarnation used same-hash fallback")
				}
			}
			process(readoption, false)
			if negative {
				hardeningNoReceipt(t, s)
				if !l.blockStates[b.Hash()].removed {
					t.Error("later-removed Added reopened canonical marker")
				}
				process(ethmonitor.Blocks{added, removed}, false)
				for _, r := range receiptsFixCollect(s) {
					if !r.Reorged || r.Final {
						t.Error("same-batch removal published stale mined/final receipt")
					}
				}
				if _, err := l.fetchTransactionReceipt(context.Background(), tx.Hash(), false); !errors.Is(err, ethereum.NotFound) {
					t.Errorf("later removal reopened hash lookup: %v", err)
				}
			} else {
				select {
				case mined := <-s.TransactionReceipt():
					if mined.TransactionHash() != tx.Hash() || mined.BlockHash() != b.Hash() || mined.Filter != q || mined.Reorged || mined.Final || mined.generation != 1 {
						t.Error("readopted incarnation lost exact mined ownership")
					}
				case <-time.After(100 * time.Millisecond):
					t.Error("canonical readoption after retention was rejected")
				}
				result, err := l.fetchTransactionReceipt(context.Background(), tx.Hash(), false)
				if err != nil || result != canonical {
					t.Errorf("canonical readoption hash lookup failed: %v", err)
				}
			}
			releaseOld()
			if err := <-oldDone; !errors.Is(err, ethereum.NotFound) {
				t.Errorf("old generation RPC escaped invalidation: %v", err)
			}
			if !negative {
				cached, found, _ := l.pastReceipts.Get(context.Background(), tx.Hash().Hex())
				if !found || cached != canonical {
					t.Error("old RPC replaced fresh canonical cache")
				}
			}
			hardeningNoReceipt(t, s)
		})
	}
}
