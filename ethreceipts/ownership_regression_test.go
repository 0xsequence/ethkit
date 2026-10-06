package ethreceipts

import (
	"context"
	"encoding/json"
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

type ownershipValueFilter struct {
	Filterer
	labels []string
	match  func(context.Context, Receipt) (bool, error)
}

func (f ownershipValueFilter) Match(ctx context.Context, r Receipt) (bool, error) {
	return f.match(ctx, r)
}
func TestOwnershipCustomValueFilter(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 100)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, hardeningOptions(), b)
	base := FilterLogs(func([]*types.Log) bool { return false }).SearchCache(true).(Filterer)
	var matches atomic.Int32
	q := ownershipValueFilter{base, []string{"custom"}, func(_ context.Context, r Receipt) (bool, error) {
		matches.Add(1)
		return r.TransactionHash() == tx.Hash(), nil
	}}
	s := l.Subscribe(q)
	defer s.Unsubscribe()
	gotFilter, ok := s.Filters()[0].(ownershipValueFilter)
	if !ok || gotFilter.Filterer != base || gotFilter.labels[0] != "custom" {
		t.Fatal("public Filters lost original custom value")
	}
	hardeningStart(t, l)
	r := hardeningRead(t, s)
	original, ok := r.Filter.(ownershipValueFilter)
	if !ok || original.Filterer != base || original.labels[0] != "custom" || original.match == nil {
		t.Fatal("receipt lost original custom Filterer value")
	}
	if r.TransactionHash() != tx.Hash() || r.BlockHash() != b.Hash() || matches.Load() == 0 {
		t.Fatal("custom Match behavior lost")
	}
	s.ClearFilters()
}

type ownershipCollectionFilter struct {
	Filterer
	cancel context.CancelFunc
}

func (f *ownershipCollectionFilter) Match(context.Context, Receipt) (bool, error) {
	if f.cancel != nil {
		f.cancel()
		return false, nil
	}
	return false, errors.New("temporary match failure")
}
func TestOwnershipCollectionAbort(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "matcher_error", true: "cancellation"}[canceled], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 101)
			tx2, _, _ := hardeningTxn(t, 102)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true)
			bad := &ownershipCollectionFilter{Filterer: FilterLogs(func([]*types.Log) bool { return false }).(Filterer)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if canceled {
				bad.cancel = cancel
			}
			s := l.Subscribe(q, bad).(*subscriber)
			defer s.Unsubscribe()
			_, err := s.matchFiltersAndPublish(ctx, s.Filters(), []Receipt{{receipt: hardeningReceipt(b1, tx1)}})
			if err == nil {
				t.Error("expected matching abort")
			}
			if canceled && !errors.Is(err, context.Canceled) {
				t.Errorf("cancellation error: %v", err)
			}
			if len(s.inFlight) != 0 || len(s.claims) != 0 {
				t.Errorf("aborted collection retained work: inFlight=%d claims=%d", len(s.inFlight), len(s.claims))
			}
			hardeningNoReceipt(t, s)
			s.RemoveFilter(bad)
			if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b2, tx2)}}); err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx2.Hash() || r.Filter != q {
					t.Error("unused claim blocked later transaction")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("aborted reservation blocked later processing")
			}
		})
	}
}

func TestOwnershipQueryReuse(t *testing.T) {
	for _, clearAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "clear"}[clearAll], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 103)
			tx2, _, _ := hardeningTxn(t, 104)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b1, tx1)}}); err != nil {
				t.Fatal(err)
			}
			hardeningRead(t, s)
			if clearAll {
				s.ClearFilters()
			} else {
				s.RemoveFilter(q.(Filterer))
			}
			if len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 {
				t.Error("explicit removal retained canceled ownership")
			}
			s.AddFilter(q)
			if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b2, tx2)}}); err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx2.Hash() || r.Filter != q || r.Final {
					t.Error("reused query kept old selection")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("reused LimitOne query missed second transaction")
			}
			if err := s.finalizeReceipts(big.NewInt(104)); err != nil {
				t.Fatal(err)
			}
			select {
			case r := <-s.TransactionReceipt():
				if !r.Final || r.TransactionHash() != tx2.Hash() || r.Filter != q {
					t.Error("canceled owner finalized in new lifetime")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("new owner did not finalize")
			}
			hardeningNoReceipt(t, s)
		})
	}
}
func TestOwnershipReaddRejectsOldWorker(t *testing.T) {
	for _, clearAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "clear"}[clearAll], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 105)
			tx2, _, _ := hardeningTxn(t, 106)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			entered, release := make(chan struct{}), make(chan struct{})
			p := &hardeningProvider{receipt: func(ctx context.Context, h common.Hash) (*types.Receipt, error) {
				if h == tx1.Hash() {
					close(entered)
					select {
					case <-release:
						return hardeningReceipt(b1, tx1), nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return hardeningReceipt(b2, tx2), nil
			}}
			l := hardeningListener(t, p, hardeningOptions())
			q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true)
			s := l.Subscribe(q).(*subscriber)
			defer s.Unsubscribe()
			done := make(chan error, 1)
			go func() {
				_, err := l.processCachedBlocks(context.Background(), ethmonitor.Blocks{b1}, []*subscriber{s}, [][]Filterer{s.Filters()})
				done <- err
			}()
			<-entered
			if clearAll {
				s.ClearFilters()
			} else {
				s.RemoveFilter(q.(Filterer))
			}
			s.AddFilter(q)
			hardeningProcess(t, l, s, b2)
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx2.Hash() || r.Filter != q {
					t.Error("new lifetime selected old transaction")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("new lifetime suppressed by old claim")
			}
			close(release)
			<-done
			hardeningNoReceipt(t, s)
			if len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 {
				t.Error("completed/canceled lifetime retained state")
			}
		})
	}
}
func TestOwnershipCompletionReleasesState(t *testing.T) {
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	s := l.Subscribe().(*subscriber)
	defer s.Unsubscribe()
	for i := 0; i < 32; i++ {
		tx, _, _ := hardeningTxn(t, uint64(200+i))
		b := hardeningBlock(100, tx)
		q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(i%4 != 0)
		s.AddFilter(q)
		if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: hardeningReceipt(b, tx)}}); err != nil {
			t.Fatal(err)
		}
		hardeningRead(t, s)
		switch i % 4 {
		case 1:
			if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
				t.Fatal(err)
			}
			hardeningRead(t, s)
		case 2:
			s.RemoveFilter(q.(Filterer))
		case 3:
			s.ClearFilters()
		}
		if len(s.Filters()) != 0 || len(s.claims) != 0 || len(s.inFlight) != 0 || len(s.deliveries) != 0 {
			t.Fatalf("completion %d retained state: active=%d claims=%d inFlight=%d deliveries=%d", i, len(s.Filters()), len(s.claims), len(s.inFlight), len(s.deliveries))
		}
		if len(s.finalizer.queue) != 0 || len(s.finalizer.txns) != 0 || len(s.pendingReceipts) != 0 {
			t.Fatalf("completion %d retained queued ownership", i)
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

// The real monitor polls this provider and builds/broadcasts reorg events. Tests
// advance its canonical RPC responses without changing monitor production APIs.
type ownershipLiveProvider struct {
	*hardeningProvider
	mu        sync.Mutex
	canonical map[uint64]*types.Block
	blocks    map[common.Hash]*types.Block
}

func (p *ownershipLiveProvider) advance(blocks ...*types.Block) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, b := range blocks {
		p.canonical[b.NumberU64()] = b
		p.blocks[b.Hash()] = b
	}
}

func ownershipBlockPayload(b *types.Block) (json.RawMessage, error) {
	if b == nil {
		return nil, ethereum.NotFound
	}
	header := b.Header()
	header.Difficulty = big.NewInt(0)
	payload, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	fields["transactions"], err = json.Marshal(b.Transactions())
	if err != nil {
		return nil, err
	}
	fields["uncles"] = json.RawMessage(`[]`)
	return json.Marshal(fields)
}

func (p *ownershipLiveProvider) RawBlockByNumber(_ context.Context, num *big.Int) (json.RawMessage, error) {
	p.mu.Lock()
	b := p.canonical[num.Uint64()]
	p.mu.Unlock()
	return ownershipBlockPayload(b)
}

func (p *ownershipLiveProvider) RawBlockByHash(_ context.Context, hash common.Hash) (json.RawMessage, error) {
	p.mu.Lock()
	b := p.blocks[hash]
	p.mu.Unlock()
	return ownershipBlockPayload(b)
}

func (p *ownershipLiveProvider) RawFilterLogs(context.Context, ethereum.FilterQuery) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
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
