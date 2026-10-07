package ethreceipts

import (
	"context"
	"errors"
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

func TestHardeningRollbackRemining(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 1)
	old, newBlock := hardeningBlock(100, tx), hardeningBlock(102, tx)
	var current atomic.Pointer[types.Receipt]
	current.Store(hardeningReceipt(old, tx))
	var calls atomic.Int32
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { calls.Add(1); return current.Load(), nil }}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterTxnHash(tx.Hash()).SearchCache(false).QueryOnChainTxnHash(false)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	hardeningProcess(t, l, s, old)
	first := hardeningRead(t, s)
	if first.Final || first.Reorged || first.BlockHash() != old.Hash() {
		t.Fatalf("initial receipt: %+v", first)
	}
	removed := *old
	removed.Event = ethmonitor.Removed
	hardeningProcess(t, l, s, &removed)
	select {
	case rollback := <-s.TransactionReceipt():
		if !rollback.Reorged || rollback.Final || rollback.BlockHash() != old.Hash() || rollback.TransactionHash() != tx.Hash() {
			t.Fatalf("rollback: %+v", rollback)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("missing rollback receipt")
	}
	if calls.Load() != 1 {
		t.Errorf("rollback fetched orphan receipt: %d RPC calls", calls.Load())
	}
	if err := s.finalizeReceipts(big.NewInt(104)); err != nil {
		t.Fatal(err)
	}
	hardeningNoReceipt(t, s)
	current.Store(hardeningReceipt(newBlock, tx))
	hardeningProcess(t, l, s, newBlock)
	mined := hardeningRead(t, s)
	if mined.BlockHash() != newBlock.Hash() || mined.Reorged || mined.Final {
		t.Fatalf("re-mined receipt owns wrong block: %+v", mined)
	}
	if err := s.finalizeReceipts(big.NewInt(105)); err != nil {
		t.Fatal(err)
	}
	final := hardeningRead(t, s)
	if !final.Final || final.Reorged || final.BlockHash() != newBlock.Hash() || final.Filter != q {
		t.Fatalf("canonical final receipt: %+v", final)
	}
}

func TestHardeningInflightRollbackRemining(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 11)
	old, canonical := hardeningBlock(100, tx), hardeningBlock(102, tx)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	p := &hardeningProvider{receipt: func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
				return hardeningReceipt(old, tx), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return hardeningReceipt(canonical, tx), nil
	}}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterTxnHash(tx.Hash()).SearchCache(false).QueryOnChainTxnHash(false)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	snapshot := s.Filters()
	oldDone := make(chan error, 1)
	go func() {
		_, err := l.processCachedBlocks(context.Background(), ethmonitor.Blocks{old}, []*subscriber{s}, [][]Filterer{snapshot})
		oldDone <- err
	}()
	<-entered
	removed := *old
	removed.Event = ethmonitor.Removed
	hardeningProcess(t, l, s, &removed)
	select {
	case rollback := <-s.TransactionReceipt():
		if !rollback.Reorged || rollback.Final || rollback.BlockHash() != old.Hash() || rollback.Filter != q {
			t.Errorf("wrong in-flight rollback: %+v", rollback)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("missing in-flight rollback")
	}
	if err := s.finalizeReceipts(big.NewInt(104)); err != nil {
		t.Fatal(err)
	}
	hardeningNoReceipt(t, s)
	hardeningProcess(t, l, s, canonical)
	select {
	case mined := <-s.TransactionReceipt():
		if mined.BlockHash() != canonical.Hash() || mined.Reorged || mined.Final || mined.Filter != q {
			t.Errorf("wrong canonical mined receipt: %+v", mined)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("missing canonical mined receipt")
	}
	close(release)
	select {
	case <-oldDone:
	case <-time.After(time.Second):
		t.Fatal("old fetch did not finish")
	}
	// A registration snapshot retained before rollback must also remain invalid.
	if _, err := l.processCachedBlocks(context.Background(), ethmonitor.Blocks{old}, []*subscriber{s}, [][]Filterer{snapshot}); err != nil {
		t.Fatal(err)
	}
	hardeningNoReceipt(t, s)
	if err := s.finalizeReceipts(big.NewInt(105)); err != nil {
		t.Fatal(err)
	}
	select {
	case final := <-s.TransactionReceipt():
		if !final.Final || final.Reorged || final.BlockHash() != canonical.Hash() || final.Filter != q {
			t.Errorf("wrong canonical final: %+v", final)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("missing canonical final")
	}
	cached, err := l.fetchTransactionReceipt(context.Background(), tx.Hash(), false)
	if err != nil || cached.BlockHash != canonical.Hash() {
		t.Errorf("late orphan RPC poisoned receipt cache: receipt=%+v error=%v", cached, err)
	}
	if calls.Load() != 2 {
		t.Errorf("unexpected fetch count: %d", calls.Load())
	}
}

// This matcher deliberately needs the fetched receipt, which removed block data
// cannot supply. Rollback must use its previously delivered owner and receipt.
type hardeningStatusFilter struct{ Filterer }

func (f *hardeningStatusFilter) Match(_ context.Context, r Receipt) (bool, error) {
	return r.Status() == 1, nil
}

func TestHardeningRollbackRetainsCustomOwner(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 14)
	b := hardeningBlock(100, tx)
	complete := hardeningReceipt(b, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return complete, nil }}
	l := hardeningListener(t, p, hardeningOptions())
	q := &hardeningStatusFilter{FilterLogs(func([]*types.Log) bool { return true }).Finalize(true).QueryOnChain(func(context.Context) (*types.Receipt, error) { return complete, nil }).(Filterer)}
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	if err := l.queryFilterOnChain(context.Background(), s, s.Filters()); err != nil {
		t.Fatal(err)
	}
	first := hardeningRead(t, s)
	if first.Filter != q || first.Receipt() != complete {
		t.Fatal("wrong custom filter owner or receipt")
	}
	removed := *b
	removed.Event = ethmonitor.Removed
	hardeningProcess(t, l, s, &removed)
	select {
	case r := <-s.TransactionReceipt():
		if !r.Reorged || r.Final || r.Filter != q || r.Receipt() != complete {
			t.Errorf("rollback lost retained data/owner: %+v", r)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("custom complete-receipt matcher lost rollback")
	}
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	hardeningNoReceipt(t, s)
}

func TestHardeningRollbackInvalidatesPending(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 15)
	old, canonical := hardeningBlock(100, tx), hardeningBlock(102, tx)
	var failing atomic.Bool
	failing.Store(true)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
		if failing.Load() {
			return nil, errors.New("transient receipt error")
		}
		return hardeningReceipt(canonical, tx), nil
	}}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterTxnHash(tx.Hash()).SearchCache(false).QueryOnChainTxnHash(false)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	hardeningProcess(t, l, s, old)
	s.retryMu.Lock()
	pendingBefore := len(s.pendingReceipts)
	s.retryMu.Unlock()
	if pendingBefore != 1 {
		t.Fatalf("expected one pending owner, got %d", pendingBefore)
	}
	removed := *old
	removed.Event = ethmonitor.Removed
	hardeningProcess(t, l, s, &removed)
	s.retryMu.Lock()
	pendingAfter := len(s.pendingReceipts)
	s.retryMu.Unlock()
	if pendingAfter != 0 {
		t.Errorf("rollback retained %d orphan pending deliveries", pendingAfter)
	}
	select {
	case r := <-s.TransactionReceipt():
		if !r.Reorged || r.Filter != q || r.BlockHash() != old.Hash() {
			t.Errorf("wrong pending rollback: %+v", r)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("missing pending rollback")
	}
	failing.Store(false)
	hardeningProcess(t, l, s, canonical)
	mined := hardeningRead(t, s)
	if mined.BlockHash() != canonical.Hash() || mined.Reorged {
		t.Fatal("retry ownership interfered with re-mining")
	}
	if err := s.finalizeReceipts(big.NewInt(105)); err != nil {
		t.Fatal(err)
	}
	final := hardeningRead(t, s)
	if !final.Final || final.BlockHash() != canonical.Hash() || final.Filter != q {
		t.Fatal("wrong re-mined final owner")
	}
}

func TestHardeningSameHashReadoptionRejectsOldWork(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 19)
	block := hardeningBlock(100, tx)
	oldReceipt, canonicalReceipt := hardeningReceipt(block, tx), hardeningReceipt(block, tx)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	p := &hardeningProvider{receipt: func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
				return oldReceipt, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return canonicalReceipt, nil
	}}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterTxnHash(tx.Hash()).SearchCache(false).QueryOnChainTxnHash(false)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	fs := s.Filters()
	oldDone := make(chan error, 1)
	go func() {
		_, err := l.processCachedBlocks(context.Background(), ethmonitor.Blocks{block}, []*subscriber{s}, [][]Filterer{fs})
		oldDone <- err
	}()
	<-entered
	removed := *block
	removed.Event = ethmonitor.Removed
	hardeningProcess(t, l, s, &removed)
	rollback := hardeningRead(t, s)
	if !rollback.Reorged || rollback.BlockHash() != block.Hash() || rollback.Filter != q {
		t.Fatalf("wrong rollback: %+v", rollback)
	}
	// A cache snapshot taken before removal cannot authoritatively re-adopt it.
	hardeningProcess(t, l, s, block)
	hardeningNoReceipt(t, s)
	if calls.Load() != 1 {
		t.Errorf("stale snapshot cleared removal marker: %d fetches", calls.Load())
	}
	// The live block-processing entry point receives the canonical Added event.
	if _, err := l.processBlocks(context.Background(), ethmonitor.Blocks{block}, []*subscriber{s}, [][]Filterer{fs}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-s.TransactionReceipt():
		if r.Final || r.Reorged || r.Filter != q || r.Receipt() != canonicalReceipt {
			t.Errorf("wrong readopted receipt: %+v", r)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("canonical same-hash re-adoption did not reopen delivery")
	}
	close(release)
	<-oldDone
	hardeningNoReceipt(t, s)
	if calls.Load() != 2 {
		t.Errorf("expected fresh readoption fetch, got %d", calls.Load())
	}
	cached, err := l.fetchTransactionReceipt(context.Background(), tx.Hash(), false)
	if err != nil || cached != canonicalReceipt {
		t.Errorf("old in-flight work replaced canonical cache: receipt=%p error=%v", cached, err)
	}
	// A callback with missing block data must acquire the fresh block generation
	// when completing its receipt from cache, rather than inheriting generation zero.
	partial := &types.Receipt{TxHash: tx.Hash()}
	completion := l.Subscribe(FilterLogs(func([]*types.Log) bool { return true }).QueryOnChain(func(context.Context) (*types.Receipt, error) { return partial, nil })).(*subscriber)
	defer completion.Unsubscribe()
	if err := l.queryFilterOnChain(context.Background(), completion, completion.Filters()); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-completion.TransactionReceipt():
		if r.Receipt() != canonicalReceipt || r.BlockHash() != block.Hash() {
			t.Error("incomplete callback lost readopted receipt")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("missing-data callback lost canonical generation")
	}
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-s.TransactionReceipt():
		if !r.Final || r.Reorged || r.Filter != q || r.Receipt() != canonicalReceipt {
			t.Errorf("wrong readopted final: %+v", r)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("missing readopted final")
	}
	hardeningNoReceipt(t, s)
}

func TestOwnershipLiveExhaustedRollbackReleasesOwner(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 110)
	b := hardeningBlock(100, tx)
	p := &ownershipLiveProvider{
		hardeningProvider: &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) {
			return hardeningReceipt(b, tx), nil
		}},
		canonical: make(map[uint64]*types.Block),
		blocks:    make(map[common.Hash]*types.Block),
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mo := ethmonitor.DefaultOptions
	mo.Logger, mo.WithLogs, mo.Bootstrap = log, true, true
	mo.StreamingDisabled, mo.PrefetchConcurrency, mo.PollingInterval = true, 0, 5*time.Millisecond
	mo.BlockRetentionLimit = 100
	m, err := ethmonitor.NewMonitor(p, mo)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Chain().BootstrapFromBlocks(ethmonitor.Blocks{hardeningBlock(99)}); err != nil {
		t.Fatal(err)
	}
	opts := hardeningOptions()
	opts.NumBlocksToFinality = 10
	l, err := NewReceiptsListener(log, p, m, opts)
	if err != nil {
		t.Fatal(err)
	}
	q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true).MaxWait(1)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	hardeningStart(t, l)
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
			t.Error("monitor Run did not stop")
		}
	})
	wait := func(what string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(time.Millisecond)
		}
	}
	alternate := func(num, fork int64, parent common.Hash) *types.Block {
		header := hardeningBlock(num).Header()
		header.BlockHash, header.ParentHash = common.BigToHash(big.NewInt(num+fork)), parent
		return types.NewBlockWithHeader(header)
	}
	p.advance(b.Block)
	mined := hardeningRead(t, s)
	if mined.TransactionHash() != tx.Hash() || mined.BlockHash() != b.Hash() || mined.Filter != q || mined.Final || mined.Reorged {
		t.Fatal("wrong initial mined owner")
	}
	wait("first live match", func() bool { return q.(Filterer).LastMatchBlockNum() == 100 })
	old101 := hardeningBlock(101)
	p.advance(old101.Block)
	wait("block 101", func() bool { return m.LatestBlockNum().Int64() == 101 })
	alt101 := alternate(101, 10000, b.Hash())
	alt102 := alternate(102, 10000, alt101.Hash())
	p.advance(alt101, alt102)
	wait("live reorg counter reset", func() bool {
		return q.(Filterer).StartBlockNum() == 102 && q.(Filterer).LastMatchBlockNum() == 0
	})
	alt103 := alternate(103, 10000, alt102.Hash())
	p.advance(alt103)
	select {
	case <-q.(Filterer).Exhausted():
	case <-time.After(time.Second):
		t.Fatal("live MaxWait did not exhaust owner")
	}
	s.deliveryMu.Lock()
	queued := len(s.finalizer.queue) == 1 && len(s.claims) == 1
	s.deliveryMu.Unlock()
	if !queued || len(s.Filters()) != 0 {
		t.Fatal("exhaustion lost queued finality ownership")
	}
	current := hardeningBlock(99).Block
	for num := int64(100); num <= 104; num++ {
		next := alternate(num, 20000, current.Hash())
		p.advance(next)
		current = next
	}
	wait("rollback invalidation", func() bool {
		s.deliveryMu.Lock()
		defer s.deliveryMu.Unlock()
		return m.LatestBlock().Hash() == current.Hash() && len(s.finalizer.queue) == 0
	})
	select {
	case rollback := <-s.TransactionReceipt():
		if rollback.TransactionHash() != tx.Hash() || rollback.BlockHash() != b.Hash() || rollback.BlockNumber().Int64() != 100 || rollback.Filter != q || !rollback.Reorged || rollback.Final || rollback.Status() != 1 {
			t.Error("wrong exhausted-owner rollback identity/state")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("exhausted queued owner missed its delivered rollback")
	}
	checkRetired := func() {
		t.Helper()
		s.deliveryMu.Lock()
		defer s.deliveryMu.Unlock()
		if len(s.claims) != 0 || len(s.deliveries) != 0 || len(s.inFlight) != 0 || len(s.pendingReceipts) != 0 || len(s.finalizer.queue) != 0 || len(s.finalizer.txns) != 0 {
			t.Errorf("inactive rollback owner retained state: claims=%d deliveries=%d", len(s.claims), len(s.deliveries))
		}
	}
	checkRetired()
	for num := int64(105); num <= 112; num++ {
		next := alternate(num, 20000, current.Hash())
		p.advance(next)
		current = next
	}
	wait("finality advancement", func() bool { return m.LatestBlock().Hash() == current.Hash() })
	s.RemoveFilter(q.(Filterer))
	checkRetired()
	hardeningNoReceipt(t, s)
}

func TestOwnershipExhaustedRollbackKeepsOtherCandidates(t *testing.T) {
	tx1, _, _ := hardeningTxn(t, 111)
	tx2, _, _ := hardeningTxn(t, 112)
	b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true)
	other := FilterTxnHash(tx2.Hash()).SearchCache(false).QueryOnChainTxnHash(false)
	s := l.Subscribe(q, other).(*subscriber)
	defer s.Unsubscribe()
	for _, b := range []*ethmonitor.Block{b1, b2} {
		if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b, b.Transactions()[0])}}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		hardeningRead(t, s)
	}
	s.exhaustFilter(q.(Filterer))
	q.(*filter).closeExhausted()
	removed := *b1
	removed.Event = ethmonitor.Removed
	hardeningProcess(t, l, s, &removed)
	select {
	case r := <-s.TransactionReceipt():
		if r.TransactionHash() != tx1.Hash() || r.BlockHash() != b1.Hash() || r.Filter != q || !r.Reorged || r.Final {
			t.Error("wrong exhausted retained rollback")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("inactive owner missed rollback while another candidate remained")
	}
	if len(s.finalizer.queue) != 2 || len(s.Filters()) != 1 {
		t.Fatal("rollback removed unrelated ownership")
	}
	if err := s.finalizeReceipts(big.NewInt(104)); err != nil {
		t.Fatal(err)
	}
	seen := make(map[Filterer]bool)
	for i := 0; i < 2; i++ {
		r := hardeningRead(t, s)
		if r.TransactionHash() != tx2.Hash() || r.BlockHash() != b2.Hash() || !r.Final || r.Reorged || seen[r.Filter] {
			t.Error("rollback lost or duplicated another owned final")
		}
		seen[r.Filter] = true
	}
	if !seen[q.(Filterer)] || !seen[other.(Filterer)] || len(s.claims) != 0 || len(s.deliveries) != 0 || len(s.Filters()) != 0 {
		t.Error("finished owners were not retired")
	}
	hardeningNoReceipt(t, s)
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

func TestReceiptsFixOrphanedSelectionReleasedAtFinality(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 808)
	other, _, _ := hardeningTxn(t, 809)
	var chain []*ethmonitor.Block
	for num := int64(100); num <= 110; num++ {
		chain = append(chain, hardeningBlock(num))
	}
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions(), chain...)
	header := hardeningBlock(109).Header()
	header.BlockHash = common.HexToHash("0xdead")
	orphan := &ethmonitor.Block{Block: types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: []*types.Transaction{tx}}), Event: ethmonitor.Added, OK: true}
	canonical := hardeningBlock(110, other)
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
	process(hardeningReceipt(orphan, tx))
	if mined := hardeningRead(t, s); mined.Final || mined.BlockHash() != orphan.Hash() {
		t.Fatal("orphan was delivered as final")
	}
	s.rollbackBlock(l.invalidateBlock(context.Background(), orphan))
	hardeningRead(t, s)
	if err := s.finalizeReceipts(big.NewInt(110)); err != nil {
		t.Fatal(err)
	}
	if s.claims[owner] != tx.Hash() {
		t.Fatal("selection released while its txn could still be re-mined")
	}
	// The bootstrapped monitor cannot advance, so shrink the finality depth to
	// put the orphan's height past it.
	l.mu.Lock()
	l.options.NumBlocksToFinality = 1
	l.mu.Unlock()
	if err := s.finalizeReceipts(big.NewInt(110)); err != nil {
		t.Fatal(err)
	}
	hardeningNoReceipt(t, s)
	if _, claimed := s.claims[owner]; claimed || len(s.deliveries) != 0 || len(s.Filters()) != 1 {
		t.Fatal("finalized orphan kept its selection")
	}
	process(hardeningReceipt(canonical, other))
	mined := hardeningRead(t, s)
	if mined.Reorged || mined.Final || mined.TransactionHash() != other.Hash() || mined.BlockHash() != canonical.Hash() || mined.Filter != q {
		t.Fatal("released owner did not select the canonical txn")
	}
	if s.claims[owner] != other.Hash() {
		t.Fatal("canonical txn did not take over the selection")
	}
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
					_, err = l.processCachedBlocks(context.Background(), batch, []*subscriber{s}, [][]Filterer{s.filterers()})
				} else {
					_, err = l.processBlocks(context.Background(), batch, []*subscriber{s}, [][]Filterer{s.filterers()})
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
				for _, r := range receiptsFixCollect(t, s) {
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
