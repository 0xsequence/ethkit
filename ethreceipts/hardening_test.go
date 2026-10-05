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
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
	"github.com/goware/breaker"
)

// Only the RPC methods used by the listener are provided. No external node is needed.
type hardeningProvider struct {
	ethrpc.RawInterface
	chainID func(context.Context) (*big.Int, error)
	receipt func(context.Context, common.Hash) (*types.Receipt, error)
}

func (p *hardeningProvider) ChainID(ctx context.Context) (*big.Int, error) {
	if p.chainID != nil {
		return p.chainID(ctx)
	}
	return big.NewInt(1), nil
}
func (p *hardeningProvider) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	if p.receipt != nil {
		return p.receipt(ctx, h)
	}
	return nil, ethereum.NotFound
}
func hardeningBlock(num int64, txns ...*types.Transaction) *ethmonitor.Block {
	b := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(num), BlockHash: common.BigToHash(big.NewInt(num)), ParentHash: common.BigToHash(big.NewInt(num - 1)), GasLimit: 30_000_000, Time: uint64(num)})
	return &ethmonitor.Block{Block: b.WithBody(types.Body{Transactions: txns}), Event: ethmonitor.Added, OK: true}
}
func hardeningTxn(t *testing.T, nonce uint64) (*types.Transaction, common.Address, common.Address) {
	t.Helper()
	key, err := crypto.HexToECDSA("1111111111111111111111111111111111111111111111111111111111111111")
	if err != nil {
		t.Fatal(err)
	}
	from := crypto.PubkeyToAddress(key.PublicKey)
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tx, err := types.SignTx(types.NewTransaction(nonce, to, big.NewInt(1), 21000, big.NewInt(1), nil), types.NewEIP155Signer(big.NewInt(1)), key)
	if err != nil {
		t.Fatal(err)
	}
	return tx, from, to
}
func hardeningReceipt(b *ethmonitor.Block, tx *types.Transaction) *types.Receipt {
	return &types.Receipt{TxHash: tx.Hash(), BlockHash: b.Hash(), BlockNumber: b.Number(), Status: 1}
}
func hardeningListener(t *testing.T, p *hardeningProvider, opts Options, blocks ...*ethmonitor.Block) *ReceiptsListener {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mo := ethmonitor.DefaultOptions
	mo.Logger = log
	mo.WithLogs = true
	mo.BlockRetentionLimit = 100
	mo.Bootstrap = true
	m, err := ethmonitor.NewMonitor(p, mo)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) == 0 {
		blocks = []*ethmonitor.Block{hardeningBlock(100)}
	}
	if err = m.Chain().BootstrapFromBlocks(blocks); err != nil {
		t.Fatal(err)
	}
	l, err := NewReceiptsListener(log, p, m, opts)
	if err != nil {
		t.Fatal(err)
	}
	l.ctx = context.Background()
	l.br = breaker.New(log, 0, 1, 0)
	return l
}
func hardeningStart(t *testing.T, l *ReceiptsListener) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- l.Run(context.Background()) }()
	deadline := time.After(time.Second)
	for l.monitor.NumSubscribers() != 1 {
		select {
		case err := <-done:
			t.Fatalf("Run exited before subscription: %v", err)
		case <-deadline:
			t.Fatal("Run did not subscribe to monitor")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	t.Cleanup(func() {
		l.Stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Run did not stop")
		}
	})
}
func hardeningRead(t *testing.T, s Subscription) Receipt {
	t.Helper()
	select {
	case r := <-s.TransactionReceipt():
		return r
	case <-time.After(time.Second):
		t.Fatal("missing receipt")
		return Receipt{}
	}
}
func hardeningNoReceipt(t *testing.T, s Subscription) {
	t.Helper()
	select {
	case r := <-s.TransactionReceipt():
		t.Errorf("unexpected receipt: txn=%s block=%s final=%v reorged=%v owner=%p", r.TransactionHash(), r.BlockHash(), r.Final, r.Reorged, r.Filter)
	case <-time.After(30 * time.Millisecond):
	}
}
func hardeningProcess(t *testing.T, l *ReceiptsListener, s *subscriber, blocks ...*ethmonitor.Block) {
	t.Helper()
	if _, err := l.processBlocks(blocks, []*subscriber{s}, [][]Filterer{s.Filters()}); err != nil {
		t.Fatal(err)
	}
}
func hardeningOptions() Options { opts := DefaultOptions; opts.NumBlocksToFinality = 2; return opts }

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

func TestHardeningSubscribeBeforeRunFinality(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 2)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, DefaultOptions, b, hardeningBlock(101))
	s := l.Subscribe(FilterTxnHash(tx.Hash())).(*subscriber)
	defer s.Unsubscribe()
	hardeningStart(t, l)
	if r := hardeningRead(t, s); r.Final {
		t.Fatal("receipt final before network threshold")
	}
	processed := make(chan struct{})
	s.AddFilter(FilterLogs(func([]*types.Log) bool { return false }).SearchCache(true).QueryOnChain(func(context.Context) (*types.Receipt, error) { close(processed); return nil, nil }))
	select {
	case <-processed:
	case <-time.After(time.Second):
		t.Fatal("registration did not complete")
	}
	hardeningNoReceipt(t, s)
	if len(s.Filters()) != 2 {
		t.Fatal("premature finality removed transaction filter")
	}
}

func TestHardeningAddressFilters(t *testing.T) {
	tx, from, to := hardeningTxn(t, 3)
	b := hardeningBlock(100, tx)
	for _, tc := range []struct {
		name string
		q    FilterQuery
	}{{"from", FilterFrom(from)}, {"to", FilterTo(to)}} {
		t.Run(tc.name, func(t *testing.T) {
			p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
			l := hardeningListener(t, p, hardeningOptions(), b)
			s := l.Subscribe(tc.q.SearchCache(true))
			defer s.Unsubscribe()
			hardeningStart(t, l)
			select {
			case r := <-s.TransactionReceipt():
				if r.TransactionHash() != tx.Hash() || r.BlockHash() != b.Hash() {
					t.Fatalf("wrong matching transaction: %+v", r)
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("signed address filter missed cached transaction")
			}
		})
	}
}

func TestHardeningFinalizerOrderAndOwners(t *testing.T) {
	tx1, _, _ := hardeningTxn(t, 4)
	tx2, _, _ := hardeningTxn(t, 5)
	newer, older := hardeningBlock(101, tx1), hardeningBlock(100, tx2)
	p := &hardeningProvider{receipt: func(_ context.Context, h common.Hash) (*types.Receipt, error) {
		if h == tx1.Hash() {
			return hardeningReceipt(newer, tx1), nil
		}
		return hardeningReceipt(older, tx2), nil
	}}
	l := hardeningListener(t, p, hardeningOptions())
	q1 := FilterLogs(func([]*types.Log) bool { return true }).ID(7).Finalize(true)
	q2 := FilterLogs(func([]*types.Log) bool { return true }).ID(7).Finalize(true)
	s := l.Subscribe(q1, q2).(*subscriber)
	defer s.Unsubscribe()
	for _, r := range []*types.Receipt{hardeningReceipt(newer, tx1), hardeningReceipt(older, tx2)} {
		if _, err := s.matchFiltersAndPublish(context.Background(), s.Filters(), []Receipt{{receipt: r}}); err != nil {
			t.Fatal(err)
		}
		hardeningRead(t, s)
		hardeningRead(t, s)
	}
	for _, tc := range []struct {
		head int64
		tx   *types.Transaction
		b    *ethmonitor.Block
	}{{103, tx2, older}, {104, tx1, newer}} {
		if err := s.finalizeReceipts(big.NewInt(tc.head)); err != nil {
			t.Fatal(err)
		}
		owners := map[Filterer]bool{}
		for i := 0; i < 2; i++ {
			select {
			case r := <-s.TransactionReceipt():
				if !r.Final || r.TransactionHash() != tc.tx.Hash() || r.BlockHash() != tc.b.Hash() {
					t.Errorf("wrong final at %d: %+v", tc.head, r)
				}
				owners[r.Filter] = true
			case <-time.After(100 * time.Millisecond):
				t.Errorf("missing owner final at %d", tc.head)
			}
		}
		if !owners[q1.(Filterer)] || !owners[q2.(Filterer)] {
			t.Errorf("finality lost distinct owners with same public ID at %d", tc.head)
		}
	}
	hardeningNoReceipt(t, s)
}

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

func TestHardeningLimitOneSnapshots(t *testing.T) {
	for _, sameBlock := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_block", false: "retained_blocks"}[sameBlock], func(t *testing.T) {
			tx1, _, _ := hardeningTxn(t, 7)
			tx2, _, _ := hardeningTxn(t, 8)
			b1, b2 := hardeningBlock(100, tx1), hardeningBlock(101, tx2)
			blocks := ethmonitor.Blocks{b1, b2}
			if sameBlock {
				b1 = hardeningBlock(100, tx1, tx2)
				b2 = b1
				blocks = ethmonitor.Blocks{b1}
			}
			p := &hardeningProvider{receipt: func(_ context.Context, h common.Hash) (*types.Receipt, error) {
				if h == tx1.Hash() {
					return hardeningReceipt(b1, tx1), nil
				}
				return hardeningReceipt(b2, tx2), nil
			}}
			l := hardeningListener(t, p, hardeningOptions())
			s := l.Subscribe(FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true)).(*subscriber)
			defer s.Unsubscribe()
			fs := s.Filters()
			if _, err := l.processBlocks(blocks, []*subscriber{s}, [][]Filterer{fs}); err != nil {
				t.Fatal(err)
			}
			r := hardeningRead(t, s)
			if r.TransactionHash() != tx1.Hash() {
				t.Errorf("LimitOne did not select first match: %s", r.TransactionHash())
			}
			hardeningNoReceipt(t, s)
		})
	}
}

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

func TestHardeningStopCancelsStartup(t *testing.T) {
	entered := make(chan struct{})
	p := &hardeningProvider{chainID: func(ctx context.Context) (*big.Int, error) { close(entered); <-ctx.Done(); return nil, ctx.Err() }}
	l := hardeningListener(t, p, DefaultOptions)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.Run(parent) }()
	<-entered
	l.Stop()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		cancel()
		<-done
		t.Error("Stop did not cancel startup ChainID request")
	}
}
func TestHardeningMonitorStopCancelsRegistration(t *testing.T) {
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
	entered := make(chan struct{})
	s := l.Subscribe(FilterLogs(func([]*types.Log) bool { return false }).QueryOnChain(func(ctx context.Context) (*types.Receipt, error) { close(entered); <-ctx.Done(); return nil, ctx.Err() }))
	defer s.Unsubscribe()
	done := make(chan error, 1)
	go func() { done <- l.Run(context.Background()) }()
	<-entered
	l.monitor.UnsubscribeAll(errors.New("monitor closed"))
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		l.Stop()
		<-done
		t.Error("monitor closure did not cancel registration")
	}
}
func TestHardeningFetchSemaphoreCancellation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	p := &hardeningProvider{receipt: func(ctx context.Context, h common.Hash) (*types.Receipt, error) {
		close(entered)
		select {
		case <-release:
			return &types.Receipt{TxHash: h, BlockNumber: big.NewInt(100)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	opts := hardeningOptions()
	opts.MaxConcurrentFetchReceiptWorkers = 1
	l := hardeningListener(t, p, opts)
	first := make(chan error, 1)
	go func() { _, err := l.fetchTransactionReceipt(context.Background(), common.Hash{1}, true); first <- err }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second := make(chan error, 1)
	go func() { _, err := l.fetchTransactionReceipt(ctx, common.Hash{2}, true); second <- err }()
	blocked := false
	select {
	case err := <-second:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancellation error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		blocked = true
	}
	close(release)
	<-first
	if blocked {
		<-second
		t.Error("canceled fetch waited for unrelated fetch slot")
	}
}
func TestHardeningFinalityWaitCancellation(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 10)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, hardeningOptions())
	hardeningStart(t, l)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	mined, wait, err := l.FetchTransactionReceiptWithFinality(parent, tx.Hash())
	if err != nil {
		t.Fatal(err)
	}
	if mined.Final {
		t.Fatal("initial receipt final")
	}
	ctx, finalCancel := context.WithCancel(context.Background())
	finalCancel()
	done := make(chan error, 1)
	go func() { _, err := wait(ctx); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("wait error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		cancel()
		<-done
		t.Error("finality waiter ignored its own context")
	}
}
func TestHardeningConcurrentRunStop(t *testing.T) {
	for i := 0; i < 5; i++ {
		l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
		l.ctx, l.ctxStop = context.WithCancel(context.Background())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var wg sync.WaitGroup
		start := make(chan struct{})
		for j := 0; j < 16; j++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; _ = l.Run(ctx) }()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 16; j++ {
				l.Stop()
			}
		}()
		close(start)
		wg.Wait()
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
		_, err := l.processBlocks(ethmonitor.Blocks{old}, []*subscriber{s}, [][]Filterer{snapshot})
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
	if _, err := l.processBlocks(ethmonitor.Blocks{old}, []*subscriber{s}, [][]Filterer{snapshot}); err != nil {
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

func TestHardeningConcurrentLimitOne(t *testing.T) {
	tx1, _, _ := hardeningTxn(t, 12)
	tx2, _, _ := hardeningTxn(t, 13)
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
	q := FilterLogs(func([]*types.Log) bool { return true }).LimitOne(true).Finalize(true)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	fs := s.Filters()
	done := make(chan error, 1)
	go func() {
		_, err := l.processBlocks(ethmonitor.Blocks{b1}, []*subscriber{s}, [][]Filterer{fs})
		done <- err
	}()
	<-entered
	if _, err := l.processBlocks(ethmonitor.Blocks{b2}, []*subscriber{s}, [][]Filterer{fs}); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	r := hardeningRead(t, s)
	if r.TransactionHash() != tx1.Hash() || r.Filter != q {
		t.Errorf("concurrent LimitOne selected different transaction: %+v", r)
	}
	hardeningNoReceipt(t, s)
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	final := hardeningRead(t, s)
	if !final.Final || final.TransactionHash() != tx1.Hash() || final.Filter != q {
		t.Errorf("wrong claimed final: %+v", final)
	}
	hardeningNoReceipt(t, s)
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

func TestHardeningExplicitRemovalCancelsOnlyOwner(t *testing.T) {
	for _, clearAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove_one", true: "clear_all"}[clearAll], func(t *testing.T) {
			tx, _, _ := hardeningTxn(t, 16)
			b := hardeningBlock(100, tx)
			p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
			l := hardeningListener(t, p, hardeningOptions())
			q1 := FilterLogs(func([]*types.Log) bool { return true }).ID(1).Finalize(true)
			q2 := FilterLogs(func([]*types.Log) bool { return true }).ID(2).Finalize(true)
			s := l.Subscribe(q1, q2).(*subscriber)
			defer s.Unsubscribe()
			hardeningProcess(t, l, s, b)
			hardeningRead(t, s)
			hardeningRead(t, s)
			if clearAll {
				s.ClearFilters()
			} else {
				s.RemoveFilter(q1.(Filterer))
			}
			if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
				t.Fatal(err)
			}
			if !clearAll {
				r := hardeningRead(t, s)
				if !r.Final || r.Filter != q2 || r.BlockHash() != b.Hash() {
					t.Errorf("removal lost surviving owner: %+v", r)
				}
			}
			hardeningNoReceipt(t, s)
		})
	}
}

func TestHardeningExhaustedOwnerKeepsQueuedFinal(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 17)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true).MaxWait(1)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	hardeningProcess(t, l, s, b)
	hardeningRead(t, s)
	// Exhaustion removes matching and closes its signal, but documented pending
	// finality remains owned. Exercise that state without relying on a live chain.
	s.mu.Lock()
	s.filters = nil
	s.mu.Unlock()
	q.(*filter).closeExhausted()
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	final := hardeningRead(t, s)
	if !final.Final || final.Filter != q || final.BlockHash() != b.Hash() {
		t.Fatal("exhaustion canceled a queued final")
	}
	hardeningNoReceipt(t, s)
}

func TestHardeningMonitorClosureCancelsHeadWait(t *testing.T) {
	p := &hardeningProvider{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mo := ethmonitor.DefaultOptions
	mo.Logger = log
	mo.WithLogs = true
	mo.BlockRetentionLimit = 100
	mo.Bootstrap = true
	monitor, err := ethmonitor.NewMonitor(p, mo)
	if err != nil {
		t.Fatal(err)
	}
	l, err := NewReceiptsListener(log, p, monitor, hardeningOptions())
	if err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.Run(parent) }()
	deadline := time.After(time.Second)
	for monitor.NumSubscribers() != 1 {
		select {
		case <-deadline:
			t.Fatal("listener did not subscribe")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	monitor.UnsubscribeAll(errors.New("monitor closed before first block"))
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		cancel()
		// Baseline LatestBlockNum has a fixed 30-second wait; let it finish
		// without mutating the monitor's bootstrap state during concurrent reads.
		select {
		case <-done:
		case <-time.After(32 * time.Second):
			t.Fatal("Run did not clean up")
		}
		t.Error("monitor closure did not cancel waiting for first head")
	}
}

func TestHardeningClearCancelsExhaustedFinality(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 18)
	b := hardeningBlock(100, tx)
	p := &hardeningProvider{receipt: func(context.Context, common.Hash) (*types.Receipt, error) { return hardeningReceipt(b, tx), nil }}
	l := hardeningListener(t, p, hardeningOptions())
	q := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	hardeningProcess(t, l, s, b)
	hardeningRead(t, s)
	s.mu.Lock()
	s.filters = nil
	s.mu.Unlock()
	q.(*filter).closeExhausted()
	s.ClearFilters()
	if err := s.finalizeReceipts(big.NewInt(103)); err != nil {
		t.Fatal(err)
	}
	hardeningNoReceipt(t, s)
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
		_, err := l.processBlocks(ethmonitor.Blocks{block}, []*subscriber{s}, [][]Filterer{fs})
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
	if _, err := l.processBlocksContext(context.Background(), ethmonitor.Blocks{block}, []*subscriber{s}, [][]Filterer{fs}); err != nil {
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
