package ethmonitor_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethmonitor/internal/mocks"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/go-ethereum/crypto"
	memcache "github.com/goware/cachestore-mem"
	cachestore "github.com/goware/cachestore2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestMonitorPrefetchKeepsUp reproduces a chain producing blocks faster than
// the monitor can fetch them one at a time, and checks prefetching keeps the
// monitor at the head, in both polling and streaming mode.
func TestMonitorPrefetchKeepsUp(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	// 100 blocks/s, with 15ms per node call: a serial fetch of a block and
	// its logs manages ~30 blocks/s at best.
	const blockInterval = 10 * time.Millisecond
	const latency = 15 * time.Millisecond
	const runFor = 2 * time.Second

	for _, streaming := range []bool{false, true} {
		for _, concurrency := range []int{0, 4, 8} {
			t.Run(fmt.Sprintf("streaming=%v/prefetch=%d", streaming, concurrency), func(t *testing.T) {
				chain := newFakeChain(1000, 1, latency)
				monitor := newTestMonitor(t, chain, streaming, concurrency, 0)

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go chain.produce(ctx, blockInterval)
				go monitor.Run(ctx)
				defer monitor.Stop()

				time.Sleep(runFor)
				lag := chain.head() - monitor.LatestBlockNum().Uint64()
				t.Logf("head:%d monitor:%d lag:%d blocks", chain.head(), monitor.LatestBlockNum().Uint64(), lag)

				if concurrency == 0 {
					// the bug: without prefetching, the monitor falls behind
					assert.Greater(t, lag, uint64(60))
				} else {
					assert.Less(t, lag, uint64(15))
				}
			})
		}
	}
}

// TestMonitorPrefetchReorg reorgs the chain while the prefetcher is ahead of
// the monitor, and checks the monitor ends on the new fork with a consistent
// stream of events.
func TestMonitorPrefetchReorg(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	// prefetched: the reorg starts at a block the prefetcher already cached
	// from the abandoned fork, ahead of the monitor head. otherwise it starts
	// below the monitor head, which has already published blocks from it.
	//
	// the window is kept small, as the monitor pauses 2s for every block it
	// reverts, and stale prefetched blocks can deepen a reorg up to the window.
	for _, concurrency := range []int{2, 4} {
		for _, prefetched := range []bool{true, false} {
			t.Run(fmt.Sprintf("prefetch=%d/prefetched=%v", concurrency, prefetched), func(t *testing.T) {
				testMonitorPrefetchReorg(t, concurrency, prefetched)
			})
		}
	}
}

func testMonitorPrefetchReorg(t *testing.T, concurrency int, prefetched bool) {
	chain := newFakeChain(1000, 200, 15*time.Millisecond)
	backend, err := memcache.NewBackend(1024)
	require.NoError(t, err)
	cache := cachestore.OpenStore[[]byte](backend)
	monitor := newTestMonitor(t, chain, false, concurrency, 4, backend)

	sub := monitor.Subscribe("TestMonitorPrefetchReorg")
	defer sub.Unsubscribe()
	events := newEventLog()
	go func() {
		for blocks := range sub.Blocks() {
			events.apply(t, blocks)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Run(ctx)
	defer monitor.Stop()

	require.Eventually(t, func() bool {
		return monitor.LatestBlockNum().Uint64() >= 1050
	}, 10*time.Second, time.Millisecond)

	var reorgAt uint64
	if prefetched {
		// the monitor takes ~15ms a block here, so a block cached two or
		// more past its head is still ahead of it when we reorg
		require.Eventually(t, func() bool {
			head := monitor.LatestBlockNum().Uint64()
			for n := head + 4; n >= head+2; n-- {
				key := ethmonitor.CacheKeyBlockByNumber(big.NewInt(1), new(big.Int).SetUint64(n))
				if _, ok, _ := cache.Get(context.Background(), key); ok {
					reorgAt = n
					return true
				}
			}
			return false
		}, 5*time.Second, time.Millisecond)
	} else {
		reorgAt = monitor.LatestBlockNum().Uint64() - 2
	}
	chain.reorgFrom(reorgAt)
	t.Logf("reorged chain from block %d, monitor head %d", reorgAt, monitor.LatestBlockNum().Uint64())

	require.Eventually(t, func() bool {
		head := monitor.LatestReadyBlock()
		return head != nil && head.Hash() == chain.hashAt(chain.head())
	}, 60*time.Second, 10*time.Millisecond)

	// the retained chain must be the new canonical chain
	for _, b := range monitor.Chain().Blocks() {
		assert.Equal(t, chain.hashAt(b.NumberU64()), b.Hash(), "block %d", b.NumberU64())
	}

	// and so must the chain subscribers built from the events
	require.Eventually(t, func() bool {
		return events.head() == chain.hashAt(chain.head())
	}, 5*time.Second, 10*time.Millisecond)
	for num, hash := range events.blocks() {
		assert.Equal(t, chain.hashAt(num), hash, "subscriber block %d", num)
	}
}

// TestMonitorPrefetchHeadAhead has the node announce heads it cannot serve
// yet, as when a websocket is ahead of the http node. The prefetcher's misses
// must not wedge the monitor.
func TestMonitorPrefetchHeadAhead(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	chain := newFakeChain(1000, 1, 5*time.Millisecond)
	chain.announceAhead = 5
	monitor := newTestMonitor(t, chain, true, 4, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go chain.produce(ctx, 10*time.Millisecond)
	go monitor.Run(ctx)
	defer monitor.Stop()

	time.Sleep(1500 * time.Millisecond)
	assert.Greater(t, monitor.LatestBlockNum().Uint64(), uint64(1050))
}

// TestMonitorPrefetchSlowChainNoExtraCalls checks the prefetcher stays out
// of the way of a monitor at the head of a slow chain: workers never ask for
// blocks the chain hasn't made, and in polling mode the head isn't polled.
func TestMonitorPrefetchSlowChainNoExtraCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	type calls struct{ blockNumber, foundBlocks, filterLogs, farAheadBlocks int64 }

	run := func(streaming bool, concurrency int) calls {
		chain := newFakeChain(1000, 1, 5*time.Millisecond)
		monitor := newTestMonitor(t, chain, streaming, concurrency, 0)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go chain.produce(ctx, 200*time.Millisecond)
		go monitor.Run(ctx)
		defer monitor.Stop()

		time.Sleep(2 * time.Second)
		return calls{
			blockNumber:    chain.blockNumberCalls.Load(),
			foundBlocks:    chain.foundBlockByNumberCalls.Load(),
			filterLogs:     chain.filterLogsCalls.Load(),
			farAheadBlocks: chain.farAheadBlockByNumberCalls.Load(),
		}
	}

	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
			off := run(streaming, 0)
			on := run(streaming, 4)
			t.Logf("prefetch=0: %+v", off)
			t.Logf("prefetch=4: %+v", on)

			assert.Zero(t, off.blockNumber)
			assert.Zero(t, off.farAheadBlocks)
			assert.Zero(t, on.farAheadBlocks, "prefetch asked for a block beyond the next block at the head")
			if streaming {
				// the stream supplies the head
				assert.Zero(t, on.blockNumber)
			} else {
				// at most the one poll at startup, before the monitor has found
				// out it's at the head
				assert.LessOrEqual(t, on.blockNumber, int64(1))
			}

			// Compare requests that found blocks. Ordinary polling retries for the
			// next missing block depend on timer scheduling and cannot be compared
			// across independent runs. Allow jitter in the number of produced blocks.
			assert.InDelta(t, off.foundBlocks, on.foundBlocks, 3)
			assert.InDelta(t, off.filterLogs, on.filterLogs, 3)
		})
	}
}

func TestMonitorPrefetchShutdownNoGoroutineLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	chain := newFakeChain(1000, 100, 5*time.Millisecond)
	baseline := runtime.NumGoroutine()

	monitor := newTestMonitor(t, chain, false, 4, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- monitor.Run(ctx)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Monitor.Run() didn't return within timeout")
	}

	// Run joins its goroutines; allow runtime cleanup before comparing counts.
	// NOTE: poll here rather than with assert.Eventually, whose condition runs
	// in a goroutine of its own and so always counts one extra.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > baseline {
		buf := make([]byte, 1<<20)
		t.Fatalf("%d goroutine(s) leaked:\n%s", n-baseline, buf[:runtime.Stack(buf, true)])
	}
}

// A peer can complete an old-fork fetch between DEL and the confirmation read,
// and a cache deletion can fail. Neither may cause a false canonical removal.
func TestMonitorPrefetchRefetchBypassesCache(t *testing.T) {
	for _, deleteFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("deleteFails=%v", deleteFails), func(t *testing.T) {
			chain := newFakeChain(1000, 20, time.Millisecond)
			oldNext, ok := chain.byNumber(big.NewInt(1001))
			require.True(t, ok)
			chain.reorgFrom(1000)

			backend, err := memcache.NewBackend(512)
			require.NoError(t, err)
			key := ethmonitor.CacheKeyBlockByNumber(big.NewInt(1), big.NewInt(1001))
			racingBackend := &repopulatingBackend{
				Backend: backend, key: key, payload: oldNext.payload(), deleteFails: deleteFails,
			}
			require.NoError(t, racingBackend.SetEx(context.Background(), key, []byte(oldNext.payload()), time.Minute))
			monitor := newTestMonitor(t, chain, false, 1, 1, racingBackend)
			sub := monitor.Subscribe("TestMonitorPrefetchRefetchBypassesCache")
			defer sub.Unsubscribe()
			runMonitorForTest(t, monitor)

			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			for {
				select {
				case blocks := <-sub.Blocks():
					for _, b := range blocks {
						require.Equal(t, ethmonitor.Added, b.Event, "removed canonical block %d", b.NumberU64())
						require.Equal(t, chain.hashAt(b.NumberU64()), b.Hash())
						if b.NumberU64() >= 1001 {
							require.Positive(t, racingBackend.deletes.Load(), "did not exercise the mismatch confirmation")
							return
						}
					}
				case <-timer.C:
					t.Fatal("monitor did not advance through the stale cache entry")
				}
			}
		})
	}
}

func TestMonitorPrefetchInvalidBlockRecovery(t *testing.T) {
	for _, withLogs := range []bool{false, true} {
		for _, cached := range []bool{false, true} {
			t.Run(fmt.Sprintf("withLogs=%v/cached=%v", withLogs, cached), func(t *testing.T) {
				chain := newFakeChain(1000, 30, time.Millisecond)
				initial := newTestMonitor(t, chain, false, 4, 0)
				opts := initial.Options()
				opts.WithLogs = withLogs
				provider := &invalidBlockProvider{fakeProvider: initial.Provider().(*fakeProvider), target: 1000}
				if cached {
					key := ethmonitor.CacheKeyBlockByNumber(big.NewInt(1), big.NewInt(1000))
					require.NoError(t, opts.CacheBackend.SetEx(context.Background(), key, []byte(`{"number":"0x3e8"}`), time.Minute))
					// The origin is healthy; only the existing cache entry is invalid.
					provider.injected.Store(true)
				} else {
					// Exercise the automatic memory cache with one invalid origin response.
					opts.CacheBackend = nil
				}
				monitor, err := ethmonitor.NewMonitor(provider, opts)
				require.NoError(t, err)
				runMonitorForTest(t, monitor)
				require.Eventually(t, func() bool {
					return monitor.LatestReadyBlock() != nil && monitor.LatestBlockNum().Uint64() == chain.head()
				}, 2*time.Second, time.Millisecond)
				require.True(t, provider.injected.Load())
				for _, block := range monitor.Chain().Blocks() {
					require.Equal(t, chain.hashAt(block.NumberU64()), block.Hash())
				}
			})
		}
	}
}

func TestMonitorPrefetchIdleAfterBacklog(t *testing.T) {
	chain := newFakeChain(1000, 12, time.Millisecond)
	monitor := newTestMonitor(t, chain, false, 4, 0)
	provider := &idleHeadProvider{fakeProvider: monitor.Provider().(*fakeProvider), missed: make(chan struct{})}
	monitor, err := ethmonitor.NewMonitor(provider, monitor.Options())
	require.NoError(t, err)
	runMonitorForTest(t, monitor)

	// Wait for the serial loop's first miss after draining the backlog, rather
	// than sampling during the last block's processing.
	select {
	case <-provider.missed:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not drain the backlog")
	}
	before := chain.blockNumberCalls.Load()
	time.Sleep(150 * time.Millisecond)
	// Permit a head poll that was already selected when the miss occurred.
	require.LessOrEqual(t, chain.blockNumberCalls.Load()-before, int64(1))
}

func TestMonitorPrefetchFatalExitStopsWorkers(t *testing.T) {
	chain := newFakeChain(1000, 20, time.Millisecond)
	backend, err := memcache.NewBackend(512)
	require.NoError(t, err)
	initial := newTestMonitor(t, chain, false, 4, 0, &invalidLogsBackend{backend})
	opts := initial.Options()
	opts.BlockRetentionLimit = 2 // Four queued events; failed logs prevent dequeue.
	monitor, err := ethmonitor.NewMonitor(initial.Provider().(ethrpc.RawInterface), opts)
	require.NoError(t, err)
	sub := monitor.Subscribe("TestMonitorPrefetchFatalExitStopsWorkers")
	defer sub.Unsubscribe()

	for i := 0; i < 2; i++ {
		done := runMonitorForTest(t, monitor)
		select {
		case err := <-done:
			require.ErrorIs(t, err, ethmonitor.ErrFatal)
		case <-time.After(2 * time.Second):
			t.Fatal("expected a fatal publish error")
		}
		require.False(t, monitor.IsRunning())
		before := chain.blockNumberCalls.Load()
		time.Sleep(150 * time.Millisecond)
		require.Equal(t, before, chain.blockNumberCalls.Load(), "prefetch continued after Run returned")
	}
}

func TestMonitorPrefetchRestart(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
			chain := newFakeChain(1000, 10, time.Millisecond)
			monitor := newTestMonitor(t, chain, streaming, 4, 0)
			ctx, cancel := context.WithCancel(context.Background())
			produced := make(chan struct{})
			go func() {
				defer close(produced)
				chain.produce(ctx, 20*time.Millisecond)
			}()
			defer func() { cancel(); <-produced }()

			for i := 0; i < 2; i++ {
				target := chain.head()
				done := runMonitorForTest(t, monitor)
				require.Eventually(t, func() bool {
					return monitor.LatestBlockNum().Uint64() >= target
				}, 2*time.Second, time.Millisecond)
				monitor.Stop()
				select {
				case err := <-done:
					require.NoError(t, err)
				case <-time.After(2 * time.Second):
					t.Fatal("monitor did not stop")
				}
				// Make a gap that the next run must ingest.
				require.Eventually(t, func() bool {
					return chain.head() >= monitor.LatestBlockNum().Uint64()+3
				}, time.Second, time.Millisecond)
			}
		})
	}
}

func runMonitorForTest(t *testing.T, monitor *ethmonitor.Monitor) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- monitor.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("monitor did not shut down")
		}
	})
	return done
}

type repopulatingBackend struct {
	cachestore.Backend
	key         string
	payload     []byte
	deleteFails bool
	once        sync.Once
	deletes     atomic.Int64
}

func (b *repopulatingBackend) Delete(ctx context.Context, key string) error {
	if key != b.key {
		return b.Backend.Delete(ctx, key)
	}
	b.deletes.Add(1)
	if b.deleteFails {
		return errors.New("simulated cache deletion failure")
	}
	if err := b.Backend.Delete(ctx, key); err != nil {
		return err
	}
	b.once.Do(func() { _ = b.Backend.SetEx(ctx, key, b.payload, time.Minute) })
	return nil
}

type invalidBlockProvider struct {
	*fakeProvider
	target   uint64
	injected atomic.Bool
}

func (p *invalidBlockProvider) RawBlockByNumber(ctx context.Context, num *big.Int) (json.RawMessage, error) {
	payload, err := p.fakeProvider.RawBlockByNumber(ctx, num)
	if err == nil && num != nil && num.Uint64() == p.target && p.injected.CompareAndSwap(false, true) {
		return json.RawMessage(fmt.Sprintf(`{"number":"0x%x"}`, p.target)), nil
	}
	return payload, err
}

type idleHeadProvider struct {
	*fakeProvider
	missed chan struct{}
	once   sync.Once
}

func (p *idleHeadProvider) RawBlockByNumber(ctx context.Context, num *big.Int) (json.RawMessage, error) {
	payload, err := p.fakeProvider.RawBlockByNumber(ctx, num)
	if errors.Is(err, ethereum.NotFound) {
		p.once.Do(func() { close(p.missed) })
	}
	return payload, err
}

type invalidLogsBackend struct{ cachestore.Backend }

func (b *invalidLogsBackend) GetOrSetWithLockEx(ctx context.Context, key string, getter func(context.Context, string) (any, error), ttl time.Duration) (any, error) {
	if strings.Contains(key, ":Logs:") {
		return []byte("invalid logs"), nil
	}
	return b.Backend.GetOrSetWithLockEx(ctx, key, getter, ttl)
}

func newTestMonitor(t *testing.T, chain *fakeChain, streaming bool, prefetchConcurrency, prefetchWindow int, cacheBackend ...cachestore.Backend) *ethmonitor.Monitor {
	t.Helper()

	provider := &fakeProvider{
		MockRawInterface: mocks.NewMockRawInterface(gomock.NewController(t)),
		chain:            chain,
		streaming:        streaming,
	}

	opts := ethmonitor.DefaultOptions
	opts.PollingInterval = 20 * time.Millisecond
	opts.Timeout = 2 * time.Second
	opts.WithLogs = true
	opts.StartBlockNumber = new(big.Int).SetUint64(chain.base)
	opts.PrefetchConcurrency = prefetchConcurrency
	opts.PrefetchWindow = prefetchWindow
	if len(cacheBackend) > 0 {
		opts.CacheBackend = cacheBackend[0]
	}

	monitor, err := ethmonitor.NewMonitor(provider, opts)
	require.NoError(t, err)
	return monitor
}

// fakeChain is an in-memory chain served with a fixed latency per call.
type fakeChain struct {
	base    uint64
	latency time.Duration

	// announceAhead makes the chain announce heads this many blocks past the
	// blocks it serves.
	announceAhead uint64

	// node calls served, by method
	blockNumberCalls           atomic.Int64
	blockByNumberCalls         atomic.Int64
	foundBlockByNumberCalls    atomic.Int64
	farAheadBlockByNumberCalls atomic.Int64
	filterLogsCalls            atomic.Int64

	mu        sync.Mutex
	canonical []common.Hash // canonical[i] is block base+i
	blocks    map[common.Hash]fakeBlock
	fork      int
	heads     []*fakeSubscription
}

type fakeBlock struct {
	num    uint64
	hash   common.Hash
	parent common.Hash
}

func newFakeChain(base uint64, n int, latency time.Duration) *fakeChain {
	c := &fakeChain{base: base, latency: latency, blocks: map[common.Hash]fakeBlock{}}
	for i := 0; i < n; i++ {
		c.appendLocked()
	}
	return c
}

func (c *fakeChain) appendLocked() fakeBlock {
	num := c.base + uint64(len(c.canonical))
	b := fakeBlock{
		num:  num,
		hash: crypto.Keccak256Hash([]byte(fmt.Sprintf("fork:%d/block:%d", c.fork, num))),
	}
	if len(c.canonical) > 0 {
		b.parent = c.canonical[len(c.canonical)-1]
	}
	c.canonical = append(c.canonical, b.hash)
	c.blocks[b.hash] = b
	return b
}

func (c *fakeChain) produce(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			b := c.appendLocked()
			heads := append([]*fakeSubscription{}, c.heads...)
			c.mu.Unlock()

			header := &types.Header{Number: new(big.Int).SetUint64(b.num + c.announceAhead)}
			for _, sub := range heads {
				sub.send(header)
			}
		}
	}
}

// reorgFrom replaces the canonical chain from block num onwards with a new
// fork of the same length.
func (c *fakeChain) reorgFrom(num uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := len(c.canonical)
	c.canonical = c.canonical[:num-c.base]
	c.fork++
	for len(c.canonical) < n {
		c.appendLocked()
	}
}

func (c *fakeChain) head() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base + uint64(len(c.canonical)) - 1
}

func (c *fakeChain) hashAt(num uint64) common.Hash {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.canonical[num-c.base]
}

func (c *fakeChain) byNumber(num *big.Int) (fakeBlock, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	i := uint64(len(c.canonical) - 1)
	if num != nil {
		if num.Uint64() < c.base || num.Uint64()-c.base > i {
			return fakeBlock{}, false
		}
		i = num.Uint64() - c.base
	}
	return c.blocks[c.canonical[i]], true
}

func (c *fakeChain) byHash(hash common.Hash) (fakeBlock, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.blocks[hash]
	return b, ok
}

func (c *fakeChain) wait(ctx context.Context) error {
	select {
	case <-time.After(c.latency):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b fakeBlock) payload() json.RawMessage {
	zero := common.Hash{}.Hex()
	return json.RawMessage(fmt.Sprintf(`{`+
		`"number":"0x%x","hash":"%s","parentHash":"%s",`+
		`"sha3Uncles":"%s","miner":"0x0000000000000000000000000000000000000000",`+
		`"stateRoot":"%s","transactionsRoot":"%s","receiptsRoot":"%s",`+
		`"logsBloom":"0x%s","difficulty":"0x0","gasLimit":"0x1c9c380","gasUsed":"0x0",`+
		`"timestamp":"0x%x","extraData":"0x","transactions":[],"uncles":[]}`,
		b.num, b.hash.Hex(), b.parent.Hex(),
		zero, zero, zero, zero,
		strings.Repeat("0", 512),
		b.num,
	))
}

// fakeProvider serves a fakeChain. Methods the monitor is not expected to
// call fall through to the gomock mock, which fails the test.
type fakeProvider struct {
	*mocks.MockRawInterface
	chain     *fakeChain
	streaming bool
}

var _ ethrpc.RawInterface = &fakeProvider{}

func (p *fakeProvider) ChainID(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (p *fakeProvider) IsStreamingEnabled() bool {
	return p.streaming
}

func (p *fakeProvider) BlockNumber(ctx context.Context) (uint64, error) {
	p.chain.blockNumberCalls.Add(1)
	if err := p.chain.wait(ctx); err != nil {
		return 0, err
	}
	return p.chain.head() + p.chain.announceAhead, nil
}

func (p *fakeProvider) RawBlockByNumber(ctx context.Context, num *big.Int) (json.RawMessage, error) {
	p.chain.blockByNumberCalls.Add(1)
	if num != nil && num.Uint64() > p.chain.head()+1 {
		p.chain.farAheadBlockByNumberCalls.Add(1)
	}
	if err := p.chain.wait(ctx); err != nil {
		return nil, err
	}
	b, ok := p.chain.byNumber(num)
	if !ok {
		return nil, ethereum.NotFound
	}
	p.chain.foundBlockByNumberCalls.Add(1)
	return b.payload(), nil
}

func (p *fakeProvider) RawBlockByHash(ctx context.Context, hash common.Hash) (json.RawMessage, error) {
	if err := p.chain.wait(ctx); err != nil {
		return nil, err
	}
	b, ok := p.chain.byHash(hash)
	if !ok {
		return nil, ethereum.NotFound
	}
	return b.payload(), nil
}

func (p *fakeProvider) RawFilterLogs(ctx context.Context, q ethereum.FilterQuery) (json.RawMessage, error) {
	p.chain.filterLogsCalls.Add(1)
	if err := p.chain.wait(ctx); err != nil {
		return nil, err
	}
	return json.RawMessage(`[]`), nil
}

func (p *fakeProvider) SubscribeNewHeads(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	sub := &fakeSubscription{ch: ch, err: make(chan error), done: make(chan struct{})}
	p.chain.mu.Lock()
	p.chain.heads = append(p.chain.heads, sub)
	p.chain.mu.Unlock()
	return sub, nil
}

type fakeSubscription struct {
	ch   chan<- *types.Header
	err  chan error
	done chan struct{}
	once sync.Once
}

func (s *fakeSubscription) send(header *types.Header) {
	select {
	case s.ch <- header:
	case <-s.done:
	}
}

func (s *fakeSubscription) Unsubscribe() {
	s.once.Do(func() { close(s.done) })
}

func (s *fakeSubscription) Err() <-chan error {
	return s.err
}

// eventLog rebuilds the chain a subscriber sees from the monitor's events,
// checking each event is consistent with what came before.
type eventLog struct {
	mu    sync.Mutex
	chain []fakeBlock
}

func newEventLog() *eventLog {
	return &eventLog{}
}

func (l *eventLog) apply(t *testing.T, blocks ethmonitor.Blocks) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, b := range blocks {
		switch b.Event {
		case ethmonitor.Added:
			if n := len(l.chain); n > 0 {
				assert.Equal(t, l.chain[n-1].hash, b.ParentHash(), "added block %d does not extend the chain", b.NumberU64())
			}
			l.chain = append(l.chain, fakeBlock{num: b.NumberU64(), hash: b.Hash(), parent: b.ParentHash()})
		case ethmonitor.Removed:
			n := len(l.chain)
			if assert.Greater(t, n, 0, "removed block %d from an empty chain", b.NumberU64()) {
				assert.Equal(t, l.chain[n-1].hash, b.Hash(), "removed block %d is not the head", b.NumberU64())
				l.chain = l.chain[:n-1]
			}
		}
	}
}

func (l *eventLog) head() common.Hash {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.chain) == 0 {
		return common.Hash{}
	}
	return l.chain[len(l.chain)-1].hash
}

func (l *eventLog) blocks() map[uint64]common.Hash {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[uint64]common.Hash, len(l.chain))
	for _, b := range l.chain {
		out[b.num] = b.hash
	}
	return out
}
