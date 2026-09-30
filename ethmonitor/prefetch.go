package ethmonitor

import (
	"context"
	"fmt"
	"math/big"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// prefetchTick is how often the prefetcher reconsiders its window when
// nothing else wakes it up.
const prefetchTick = 25 * time.Millisecond

// prefetcher fetches blocks, and their logs when the monitor runs WithLogs,
// ahead of the monitor's run loop while the monitor trails the chain head.
//
// It only fills the cache, using the same keys the run loop reads, so the run
// loop stays the single place the canonical chain is built and validated;
// prefetching only changes where a payload comes from. When the monitor is at
// the head there is nothing past its next block to fetch, and the prefetcher
// is idle.
type prefetcher struct {
	m           *Monitor
	concurrency int
	window      uint64

	jobs chan prefetchJob
	wake chan struct{}

	// gen is bumped on every reset, so a job scheduled before a reorg can
	// tell its payload may belong to the abandoned fork.
	gen atomic.Uint64

	mu        sync.Mutex
	cursor    uint64 // next block number to schedule
	highWater uint64 // highest block number scheduled since the last reset
}

type prefetchJob struct {
	num uint64
	gen uint64
}

func newPrefetcher(m *Monitor, concurrency, window int) *prefetcher {
	return &prefetcher{
		m:           m,
		concurrency: concurrency,
		window:      uint64(window),
		jobs:        make(chan prefetchJob, concurrency),
		wake:        make(chan struct{}, 1),
	}
}

// notify asks the prefetcher to reconsider its window, without blocking.
func (p *prefetcher) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// run schedules prefetch jobs until ctx is done, and returns once all of its
// workers have exited.
func (p *prefetcher) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			p.m.log.Error(fmt.Sprintf("ethmonitor: panic in prefetch loop: %v - stack: %s", r, string(debug.Stack())))
			p.m.alert.Alert(context.Background(), "ethmonitor: panic in prefetch loop: %v", r)
		}
	}()

	// start from a clean slate, as Run may be called again after a failure
	p.mu.Lock()
	p.gen.Add(1)
	p.cursor, p.highWater = 0, 0
	p.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()
	for i := 0; i < p.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.worker(ctx)
		}()
	}

	ticker := time.NewTicker(prefetchTick)
	defer ticker.Stop()

	var lastHeadPoll time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-ticker.C:
		}

		// in streaming mode the newHeads stream keeps latestHead current,
		// in polling mode nothing does, so we ask the node for it. but only
		// while the monitor is catching up: at the head there is nothing to
		// prefetch, and on a slow chain we'd just double the polling load.
		if !p.m.IsStreamingMode() && p.m.isCatchingUp() && time.Since(lastHeadPoll) >= p.m.options.PollingInterval {
			lastHeadPoll = time.Now()
			p.pollHead(ctx)
		}

		p.schedule()
	}
}

func (p *prefetcher) pollHead(ctx context.Context) {
	tctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	head, err := p.m.provider.BlockNumber(tctx)
	if err != nil {
		p.m.log.Debug(fmt.Sprintf("ethmonitor: prefetch failed to poll head block number: %v", err))
		return
	}
	p.m.latestHead.Store(head)
}

// schedule hands the workers the block numbers in the window past the
// monitor's next block, as far as they have room for.
func (p *prefetcher) schedule() {
	next, ok := p.m.nextBlockNum()
	if !ok {
		return
	}

	// The run loop fetches `next` itself, so we only look past it. We also
	// leave the newest head block to the run loop: it is the one most likely
	// not yet served by the node, and the run loop already retries it.
	head := p.m.latestHead.Load()
	if head < next+2 {
		return
	}
	hi := min(head-1, next+p.window)

	p.mu.Lock()
	defer p.mu.Unlock()

	gen := p.gen.Load()
	for n := max(p.cursor, next+1); n <= hi; n++ {
		select {
		case p.jobs <- prefetchJob{num: n, gen: gen}:
			p.cursor = n + 1
			p.highWater = max(p.highWater, n)
		default:
			// workers are busy, carry on from the cursor next time
			return
		}
	}
}

// reset is called when the run loop pops the block fromNum off the canonical
// chain during a reorg. Any block prefetched above it may belong to the
// abandoned fork, so their by-number cache entries are dropped and scheduling
// resumes from fromNum.
//
// Entries a worker writes after the reset are dropped by the worker itself,
// and any stale entry that slips through is caught by the run loop, which
// confirms a block that does not extend its head with the node before
// treating it as a reorg.
func (p *prefetcher) reset(ctx context.Context, fromNum uint64) {
	p.mu.Lock()
	p.gen.Add(1)
	hi := p.highWater
	p.cursor = fromNum
	p.highWater = fromNum
	p.mu.Unlock()

	for n := fromNum + 1; n <= hi; n++ {
		key := CacheKeyBlockByNumber(p.m.chainID, new(big.Int).SetUint64(n))
		if err := p.m.cache.Delete(ctx, key); err != nil {
			p.m.log.Warn(fmt.Sprintf("ethmonitor: error deleting prefetched block cache for block num %d due to: '%v'", n, err))
		}
	}
}

func (p *prefetcher) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-p.jobs:
			p.fetch(ctx, job)
			p.notify()
		}
	}
}

// fetch loads one block, and its logs, into the cache. It makes a single
// attempt: on any failure the run loop fetches the block itself when it gets
// there.
func (p *prefetcher) fetch(ctx context.Context, job prefetchJob) {
	defer func() {
		if r := recover(); r != nil {
			p.m.log.Error(fmt.Sprintf("ethmonitor: panic in prefetch worker: %v - stack: %s", r, string(debug.Stack())))
			p.m.alert.Alert(context.Background(), "ethmonitor: panic in prefetch worker: %v", r)
		}
	}()

	m := p.m
	if job.gen != p.gen.Load() {
		return
	}

	num := new(big.Int).SetUint64(job.num)
	key := CacheKeyBlockByNumber(m.chainID, num)
	var block *types.Block

	// NOTE: the getter must not retry, as it runs while holding the lock for the key
	getter := func(ctx context.Context, _ string) ([]byte, error) {
		if m.options.DebugLogging {
			m.log.Debug(fmt.Sprintf("ethmonitor: prefetch is calling origin for number %d", job.num))
		}
		tctx, cancel := context.WithTimeout(ctx, m.options.Timeout)
		defer cancel()

		payload, err := m.provider.RawBlockByNumber(tctx, num)
		if err != nil {
			return nil, err
		}
		if len(payload) == 0 || string(payload) == "null" {
			return nil, ethereum.NotFound
		}
		// Validate even without logs: the serial loop must never inherit an
		// undecodable response from a successful cache write.
		block, err = m.unmarshalBlock(payload)
		if err != nil {
			return nil, err
		}
		return payload, nil
	}

	payload, err := m.cache.GetOrSetWithLockEx(ctx, key, getter, m.options.CacheExpiry)
	if err != nil {
		m.log.Debug(fmt.Sprintf("ethmonitor: prefetch of block %d failed: %v", job.num, err))
		return
	}

	if job.gen != p.gen.Load() {
		// a reorg reset ran while we were fetching, so this payload may be
		// from the abandoned fork
		if err := m.cache.Delete(ctx, key); err != nil {
			m.log.Warn(fmt.Sprintf("ethmonitor: error deleting prefetched block cache for block num %d due to: '%v'", job.num, err))
		}
		return
	}

	if block == nil {
		block, err = m.unmarshalBlock(payload)
		if err != nil {
			m.log.Debug(fmt.Sprintf("ethmonitor: prefetch failed to decode block %d: %v", job.num, err))
			if deleteErr := m.cache.Delete(ctx, key); deleteErr != nil {
				m.log.Warn(fmt.Sprintf("ethmonitor: error deleting invalid block cache for block num %d due to: '%v'", job.num, deleteErr))
			}
			return
		}
	}

	if !m.options.WithLogs {
		return
	}

	// logs are keyed by block hash, so they stay valid across reorgs
	tctx, cancel := context.WithTimeout(ctx, m.options.Timeout)
	defer cancel()
	if _, _, err := m.filterLogs(tctx, block.Hash(), m.logTopics(), block.Bloom()); err != nil {
		m.log.Debug(fmt.Sprintf("ethmonitor: prefetch of logs for block %d failed: %v", job.num, err))
	}
}
