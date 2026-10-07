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
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/goware/breaker"
)

func TestSubscriptionUnsubscribe(t *testing.T) {
	for _, workers := range []int{1, 8} {
		name := "repeated"
		if workers > 1 {
			name = "concurrent"
		}
		t.Run(name, func(t *testing.T) {
			l := hardeningListener(t, &hardeningProvider{}, hardeningOptions())
			s := l.Subscribe()
			other := l.Subscribe()
			defer other.Unsubscribe()
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() {
						if p := recover(); p != nil {
							t.Errorf("Unsubscribe panicked: %v", p)
						}
					}()
					<-start
					s.Unsubscribe()
					s.Unsubscribe()
				}()
			}
			close(start)
			wg.Wait()
			if l.NumSubscribers() != 1 {
				t.Fatal("Unsubscribe did not remove exactly one subscription")
			}
			select {
			case <-s.Done():
			default:
				t.Error("unsubscribed Done channel remains open")
			}
			select {
			case _, ok := <-s.TransactionReceipt():
				if ok {
					t.Error("unsubscribed receipt channel remains open")
				}
			case <-time.After(time.Second):
				t.Error("unsubscribed receipt channel did not close")
			}
			select {
			case <-other.Done():
				t.Error("Unsubscribe closed an unrelated subscription")
			default:
			}
		})
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

func TestHardeningStopCancelsStartupBackoff(t *testing.T) {
	entered := make(chan struct{})
	var calls atomic.Int32
	p := &hardeningProvider{chainID: func(context.Context) (*big.Int, error) {
		if calls.Add(1) == 1 {
			close(entered)
			return nil, errors.New("transient ChainID failure")
		}
		return nil, breaker.ErrFatal
	}}
	l := hardeningListener(t, p, DefaultOptions)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.Run(parent) }()
	<-entered
	// Allow the failed attempt to enter its one-second backoff.
	time.Sleep(100 * time.Millisecond)
	l.Stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("startup cancellation error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		cancel()
		<-done
		t.Error("Stop waited for startup ChainID backoff")
	}
	if calls.Load() != 1 {
		t.Errorf("ChainID retried after cancellation: %d calls", calls.Load())
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
		_, l.ctxStop = context.WithCancel(context.Background())
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

// blockingAlerter holds the subscriber channel's piping goroutine, as a slow
// alerting backend would, until released.
type blockingAlerter struct {
	entered chan struct{}
	once    sync.Once
	release chan struct{}
}

func (a *blockingAlerter) Alert(context.Context, string, ...interface{}) {
	a.once.Do(func() { close(a.entered) })
	<-a.release
}

func TestReceiptsFixBlockedSendKeepsListenerUnlocked(t *testing.T) {
	var txns []*types.Transaction
	for nonce := uint64(820); nonce < 820+subscriberQueueWarning+2; nonce++ {
		tx, _, _ := hardeningTxn(t, nonce)
		txns = append(txns, tx)
	}
	b := hardeningBlock(100, txns...)
	alerter := &blockingAlerter{entered: make(chan struct{}), release: make(chan struct{})}
	opts := hardeningOptions()
	opts.Alerter = alerter
	l := hardeningListener(t, &hardeningProvider{}, opts)
	q := FilterLogs(func([]*types.Log) bool { return true })
	s := l.Subscribe(q).(*subscriber)
	owner := s.owner(q.(Filterer))
	published := make(chan struct{})
	go func() {
		defer close(published)
		// Nobody reads, so the receipt that passes subscriberQueueWarning raises
		// an alert and the next one blocks in Send.
		for _, tx := range txns {
			s.publish(context.Background(), Receipt{receipt: hardeningReceipt(b, tx)}, owner)
		}
	}()
	t.Cleanup(func() {
		close(alerter.release)
		select {
		case <-published:
			s.Unsubscribe()
		case <-time.After(time.Second):
			t.Error("publisher did not stop after releasing the alert")
		}
	})
	select {
	case <-alerter.entered:
	case <-published:
		t.Fatal("publisher finished without raising a queue alert")
	case <-time.After(time.Second):
		t.Fatal("subscriber queue alert did not fire")
	}
	for deadline := time.Now().Add(200 * time.Millisecond); time.Now().Before(deadline); {
		checked := make(chan struct{})
		go func() {
			defer close(checked)
			l.isCurrentBlock(b.Hash(), 0)
		}()
		select {
		case <-checked:
		case <-time.After(time.Second):
			t.Fatal("receiptMu held across a blocked subscriber send")
		}
	}
}

// publicationContext signals the first cancellation check without changing
// its result, so the test can cancel while the subsequent block check waits.
type publicationContext struct {
	context.Context
	once    sync.Once
	checked chan struct{}
}

func (c *publicationContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.checked) })
	return err
}

func TestSubscriptionPublishCancellationDuringBlockCheck(t *testing.T) {
	tx, _, _ := hardeningTxn(t, 9903)
	b := hardeningBlock(100, tx)
	l := hardeningListener(t, &hardeningProvider{}, hardeningOptions(), b)
	q := FilterLogs(func([]*types.Log) bool { return true }).Finalize(true)
	s := l.Subscribe(q).(*subscriber)
	defer s.Unsubscribe()
	owner := s.owner(q.(Filterer))
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &publicationContext{Context: base, checked: make(chan struct{})}
	done := make(chan bool, 1)
	l.receiptMu.Lock()
	go func() {
		done <- s.publish(ctx, Receipt{receipt: hardeningReceipt(b, tx)}, owner)
	}()
	select {
	case <-ctx.checked:
	case <-time.After(time.Second):
		l.receiptMu.Unlock()
		t.Fatal("publication did not check cancellation")
	}
	cancel()
	l.receiptMu.Unlock()
	select {
	case published := <-done:
		if published {
			t.Error("receipt published after cancellation during block validation")
		}
	case <-time.After(time.Second):
		t.Fatal("publication did not finish")
	}
	if len(s.deliveries) != 0 || s.finalizer.hasOwner(owner) {
		t.Error("canceled publication recorded delivery or queued finality")
	}
	hardeningNoReceipt(t, s)
}

// customWaitFilter reports its own start block and never matches.
type customWaitFilter struct{ Filterer }

func (customWaitFilter) StartBlockNum() uint64                        { return 1 }
func (customWaitFilter) Match(context.Context, Receipt) (bool, error) { return false, nil }

func TestReceiptsFixCustomFilterMaxWait(t *testing.T) {
	p := &ownershipLiveProvider{
		hardeningProvider: &hardeningProvider{},
		canonical:         make(map[uint64]*types.Block),
		blocks:            make(map[common.Hash]*types.Block),
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
	l, err := NewReceiptsListener(log, p, m, hardeningOptions())
	if err != nil {
		t.Fatal(err)
	}
	nobody := common.HexToAddress("0x3333333333333333333333333333333333333333")
	custom := customWaitFilter{FilterFrom(nobody).MaxWait(1).(Filterer)}
	builtin := FilterFrom(nobody).MaxWait(1)
	s := l.Subscribe(custom, builtin)
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
	// One block at a time, so the built-in filter sees blocks after its start.
	for num := int64(100); num <= 102; num++ {
		p.advance(hardeningBlock(num).Block)
		for deadline := time.Now().Add(5 * time.Second); m.LatestBlockNum().Int64() != num; time.Sleep(time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("monitor did not reach block %d", num)
			}
		}
	}
	select {
	case <-builtin.(Filterer).Exhausted():
	case <-time.After(5 * time.Second):
		t.Fatal("built-in filter did not exhaust")
	}
	if filters := s.Filters(); len(filters) != 1 || filters[0] != custom {
		t.Fatal("custom filter was not left to manage its own MaxWait")
	}
}
