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
