package ethmonitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
	"github.com/cespare/xxhash/v2"
	"github.com/goware/breaker"
	"github.com/goware/cachestore"
	"github.com/goware/cachestore/cachestorectl"
	"github.com/goware/calc"
	"github.com/goware/channel"
	"github.com/goware/logger"
	"github.com/goware/superr"
)

var DefaultOptions = Options{
	Logger:                           logger.NewLogger(logger.LogLevel_WARN),
	PollingInterval:                  1500 * time.Millisecond,
	ExpectedBlockInterval:            15 * time.Second,
	StreamingErrorResetInterval:      15 * time.Minute,
	StreamingRetryAfter:              20 * time.Minute,
	StreamingErrNumToSwitchToPolling: 3,
	UnsubscribeOnStop:                false,
	Timeout:                          20 * time.Second,
	StartBlockNumber:                 nil, // latest
	TrailNumBlocksBehindHead:         0,   // latest
	BlockRetentionLimit:              200,
	WithLogs:                         false,
	LogTopics:                        []common.Hash{}, // all logs
	DebugLogging:                     false,
	CacheExpiry:                      300 * time.Second,
	Alerter:                          util.NoopAlerter(),
}

type Options struct {
	// Logger used by ethmonitor to log warnings and debug info
	Logger logger.Logger

	// PollingInterval to query the chain for new blocks
	PollingInterval time.Duration

	// ExpectedBlockInterval is the expected time between blocks
	ExpectedBlockInterval time.Duration

	// StreamingErrorResetInterval is the time to reset the streaming error count
	StreamingErrorResetInterval time.Duration

	// StreamingRetryAfter is the time to wait before retrying the streaming again
	StreamingRetryAfter time.Duration

	// StreamingErrNumToSwitchToPolling is the number of errors before switching to polling
	StreamingErrNumToSwitchToPolling int

	// Auto-unsubscribe on monitor stop or error
	UnsubscribeOnStop bool

	// Timeout duration used by the rpc client when fetching data from the remote node.
	Timeout time.Duration

	// StartBlockNumber to begin the monitor from.
	StartBlockNumber *big.Int

	// Bootstrap flag which indicates the monitor will expect the monitor's
	// events to be bootstrapped, and will continue from that point. This also
	// takes precedence over StartBlockNumber when set to true.
	Bootstrap bool

	// TrailNumBlocksBehindHead is the number of blocks we trail behind
	// the head of the chain before broadcasting new events to the subscribers.
	TrailNumBlocksBehindHead int

	// BlockRetentionLimit is the number of blocks we keep on the canonical chain
	// cache.
	BlockRetentionLimit int

	// Retain block and logs payloads
	RetainPayloads bool

	// WithLogs will include logs with the blocks if specified true.
	WithLogs bool

	// LogTopics will filter only specific log topics to include.
	LogTopics []common.Hash

	// CacheBackend to use for caching block data
	// NOTE: do not use this unless you know what you're doing.
	// In most cases leave this nil.
	CacheBackend cachestore.Backend

	// CacheExpiry is how long to keep each record in cache
	CacheExpiry time.Duration

	// Alerter config via github.com/goware/alerter
	Alerter util.Alerter

	// DebugLogging toggle
	DebugLogging bool
}

var (
	ErrFatal                 = errors.New("ethmonitor: fatal error, stopping")
	ErrReorg                 = errors.New("ethmonitor: block reorg")
	ErrUnexpectedParentHash  = errors.New("ethmonitor: unexpected parent hash")
	ErrUnexpectedBlockNumber = errors.New("ethmonitor: unexpected block number")
	ErrQueueFull             = errors.New("ethmonitor: publish queue is full")
	ErrMaxAttempts           = errors.New("ethmonitor: max attempts hit")
	ErrMonitorStopped        = errors.New("ethmonitor: stopped")
)

type Monitor struct {
	options Options

	log      logger.Logger
	alert    util.Alerter
	provider ethrpc.RawInterface

	chain             *Chain
	chainID           *big.Int
	nextBlockNumber   *big.Int
	nextBlockNumberMu sync.Mutex
	pollInterval      atomic.Int64

	cache cachestore.Store[[]byte]

	publishCh    chan Blocks
	publishQueue *queue
	subscribers  []*subscriber

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
	mu      sync.RWMutex
}

func NewMonitor(provider ethrpc.RawInterface, options ...Options) (*Monitor, error) {
	opts := DefaultOptions
	if len(options) > 0 {
		opts = options[0]
	}

	if opts.Logger == nil {
		return nil, fmt.Errorf("ethmonitor: logger is nil")
	}
	if opts.Alerter == nil {
		opts.Alerter = util.NoopAlerter()
	}

	opts.BlockRetentionLimit += opts.TrailNumBlocksBehindHead

	if opts.DebugLogging {
		stdLogger, ok := opts.Logger.(*logger.StdLogAdapter)
		if ok {
			stdLogger.Level = logger.LogLevel_DEBUG
		}
	}

	var err error
	var cache cachestore.Store[[]byte]
	if opts.CacheBackend != nil {
		cache, err = cachestorectl.Open[[]byte](opts.CacheBackend, cachestore.WithLockExpiry(5*time.Second))
		if err != nil {
			return nil, fmt.Errorf("ethmonitor: open cache: %w", err)
		}

		if opts.CacheExpiry == 0 {
			opts.CacheExpiry = 60 * time.Second
		}
	}

	return &Monitor{
		options:      opts,
		log:          opts.Logger,
		alert:        opts.Alerter,
		provider:     provider,
		chain:        newChain(opts.BlockRetentionLimit, opts.Bootstrap),
		chainID:      nil,
		cache:        cache,
		publishCh:    make(chan Blocks),
		publishQueue: newQueue(opts.BlockRetentionLimit * 2),
		subscribers:  make([]*subscriber, 0),
	}, nil
}

func (m *Monitor) lazyInit(ctx context.Context) error {
	var err error
	m.chainID, err = getChainID(ctx, m.provider)
	if err != nil {
		return err
	}
	return nil
}

func (m *Monitor) Run(ctx context.Context) error {
	if m.IsRunning() {
		return fmt.Errorf("ethmonitor: already running")
	}

	m.ctx, m.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&m.running, 1)
	defer atomic.StoreInt32(&m.running, 0)

	if err := m.lazyInit(ctx); err != nil {
		return err
	}

	// Check if in bootstrap mode -- in which case we expect nextBlockNumber
	// to already be set.
	if m.options.Bootstrap && m.chain.blocks == nil {
		return errors.New("ethmonitor: monitor is in Bootstrap mode, and must be bootstrapped before run")
	}

	// Start from latest, or start from a specific block number
	if m.chain.Head() != nil {
		// starting from last block of our canonical chain
		m.nextBlockNumber = big.NewInt(0).Add(m.chain.Head().Number(), big.NewInt(1))
	} else if m.options.StartBlockNumber != nil {
		if m.options.StartBlockNumber.Cmp(big.NewInt(0)) >= 0 {
			// starting from specific block number
			m.nextBlockNumber = m.options.StartBlockNumber
		} else {
			// starting some number blocks behind the latest block num
			latestBlock, _ := m.provider.BlockByNumber(m.ctx, nil)
			if latestBlock != nil && latestBlock.Number() != nil {
				m.nextBlockNumber = big.NewInt(0).Add(latestBlock.Number(), m.options.StartBlockNumber)
				if m.nextBlockNumber.Cmp(big.NewInt(0)) < 0 {
					m.nextBlockNumber = nil
				}
			}
		}
	} else {
		// noop, starting from the latest block on the network
	}

	if m.nextBlockNumber == nil {
		m.log.Info("ethmonitor: starting from block=latest")
	} else {
		m.log.Infof("ethmonitor: starting from block=%d", m.nextBlockNumber)
	}

	// Broadcast published events to all subscribers
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case blocks := <-m.publishCh:
				if m.options.DebugLogging {
					m.log.Debug("ethmonitor: publishing block", blocks.LatestBlock().NumberU64(), "# events:", len(blocks))
				}

				// broadcast to subscribers
				m.broadcast(blocks)
			}
		}
	}()

	// Monitor the chain for canonical representation
	err := m.monitor()
	if m.options.UnsubscribeOnStop {
		m.UnsubscribeAll(err)
	}
	return err
}

func (m *Monitor) Stop() {
	m.log.Info("ethmonitor: stop")
	if m.ctxStop != nil {
		m.ctxStop()
	}
	if m.options.UnsubscribeOnStop {
		m.UnsubscribeAll(ErrMonitorStopped)
	}
}

func (m *Monitor) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

func (m *Monitor) Options() Options {
	return m.options
}

func (m *Monitor) Provider() ethrpc.Interface {
	return m.provider
}

func (m *Monitor) listenNewHead() <-chan uint64 {
	ch := make(chan uint64)

	var latestHeadBlock atomic.Uint64
	nextBlock := make(chan uint64)

	go func() {
		var streamingErrorCount int
		var streamingErrorLastTime time.Time

	reconnect:
		// reset the latest head block
		latestHeadBlock.Store(0)

		// if we have too many streaming errors, we'll switch to polling
		streamingErrorCount++
		if time.Since(streamingErrorLastTime) > m.options.StreamingErrorResetInterval {
			streamingErrorCount = 0
		}

		// listen for new heads either via streaming or polling
		if m.provider.IsStreamingEnabled() && streamingErrorCount < m.options.StreamingErrNumToSwitchToPolling {
			// Streaming mode if available, where we listen for new heads
			// and push the new block number to the nextBlock channel.
			m.log.Info("ethmonitor: starting stream head listener")

			newHeads := make(chan *types.Header)
			sub, err := m.provider.SubscribeNewHeads(m.ctx, newHeads)
			if err != nil {
				m.log.Warnf("ethmonitor (chain %s): websocket connect failed: %v", m.chainID.String(), err)
				m.alert.Alert(context.Background(), "ethmonitor (chain %s): websocket connect failed: %v", m.chainID.String(), err)
				time.Sleep(2000 * time.Millisecond)

				streamingErrorLastTime = time.Now()
				goto reconnect
			}

			for {
				blockTimer := time.NewTimer(3 * m.options.ExpectedBlockInterval)

				select {
				case <-m.ctx.Done():
					// if we're done, we'll unsubscribe and close the nextBlock channel
					sub.Unsubscribe()
					close(nextBlock)
					blockTimer.Stop()
					return

				case err := <-sub.Err():
					// if we have an error, we'll reconnect
					m.log.Warnf("ethmonitor (chain %s): websocket subscription error: %v", m.chainID.String(), err)
					m.alert.Alert(context.Background(), "ethmonitor (chain %s): websocket subscription error: %v", m.chainID.String(), err)
					sub.Unsubscribe()

					streamingErrorLastTime = time.Now()
					blockTimer.Stop()
					goto reconnect

				case <-blockTimer.C:
					// if we haven't received a new block in a while, we'll reconnect.
					m.log.Warnf("ethmonitor: haven't received block in expected time, reconnecting..")
					sub.Unsubscribe()

					streamingErrorLastTime = time.Now()
					goto reconnect

				case newHead := <-newHeads:
					blockTimer.Stop()

					latestHeadBlock.Store(newHead.Number.Uint64())
					select {
					case nextBlock <- newHead.Number.Uint64():
					default:
						// non-blocking
					}
				}
			}
		} else {
			// We default to polling if streaming is not enabled
			m.log.Info("ethmonitor: starting poll head listener")

			retryStreamingTimer := time.NewTimer(m.options.StreamingRetryAfter)
			for {
				// if streaming is enabled, we'll retry streaming
				if m.provider.IsStreamingEnabled() {
					select {
					case <-retryStreamingTimer.C:
						// retry streaming
						m.log.Info("ethmonitor: retrying streaming...")
						streamingErrorLastTime = time.Now().Add(-m.options.StreamingErrorResetInterval * 2)
						goto reconnect
					default:
						// non-blocking
					}
				}

				// Polling mode, where we poll for the latest block number
				select {
				case <-m.ctx.Done():
					// if we're done, we'll close the nextBlock channel
					close(nextBlock)
					retryStreamingTimer.Stop()
					return

				case <-time.After(time.Duration(m.pollInterval.Load())):
					nextBlock <- 0
				}
			}
		}
	}()

	// The main loop which notifies the monitor to continue to the next block
	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return
			default:
			}

			var nextBlockNumber uint64
			m.nextBlockNumberMu.Lock()
			if m.nextBlockNumber != nil {
				nextBlockNumber = m.nextBlockNumber.Uint64()
			}
			m.nextBlockNumberMu.Unlock()

			latestBlockNum := latestHeadBlock.Load()
			if nextBlockNumber == 0 || latestBlockNum > nextBlockNumber {
				// monitor is behind, so we just push to keep going without
				// waiting on the nextBlock channel
				ch <- nextBlockNumber
				continue
			} else {
				// wait for the next block
				<-nextBlock
				ch <- latestBlockNum
			}
		}
	}()

	return ch
}

func (m *Monitor) monitor() error {
	ctx := m.ctx
	events := Blocks{}

	// minLoopInterval is time we monitor between cycles. It's a fast
	// and fixed amount of time, as the internal method `fetchNextBlock`
	// will actually use the poll interval while searching for the next block.
	minLoopInterval := 5 * time.Millisecond

	// listen for new heads either via streaming or polling
	listenNewHead := m.listenNewHead()

	// monitor run loop
	for {
		select {

		case <-m.ctx.Done():
			return nil

		case newHeadNum := <-listenNewHead:
			// ensure we have a new head number
			m.nextBlockNumberMu.Lock()
			if m.nextBlockNumber != nil && newHeadNum > 0 && m.nextBlockNumber.Uint64() > newHeadNum {
				m.nextBlockNumberMu.Unlock()
				continue
			}
			m.nextBlockNumberMu.Unlock()

			// check if we have a head block, if not, then we set the nextBlockNumber
			headBlock := m.chain.Head()
			if headBlock != nil {
				m.nextBlockNumberMu.Lock()
				m.nextBlockNumber = big.NewInt(0).Add(headBlock.Number(), big.NewInt(1))
				m.nextBlockNumberMu.Unlock()
			}

			// fetch the next block, either via the stream or via a poll
			nextBlock, nextBlockPayload, miss, err := m.fetchNextBlock(ctx)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					m.log.Infof("ethmonitor: fetchNextBlock timed out: '%v', for blockNum:%v, retrying..", err, m.nextBlockNumber)
				} else {
					m.log.Warnf("ethmonitor: fetchNextBlock error reported '%v', for blockNum:%v, retrying..", err, m.nextBlockNumber)
				}

				// pause, then retry
				time.Sleep(m.options.PollingInterval)
				continue
			}

			// if we hit a miss between calls, then we reset the pollInterval, otherwise
			// we speed up the polling interval
			if miss {
				m.pollInterval.Store(int64(m.options.PollingInterval))
			} else {
				m.pollInterval.Store(int64(clampDuration(minLoopInterval, time.Duration(m.pollInterval.Load())/4)))
			}

			// build deterministic set of add/remove events which construct the canonical chain
			events, err = m.buildCanonicalChain(ctx, nextBlock, nextBlockPayload, events)
			if err != nil {
				m.log.Warnf("ethmonitor: error reported '%v', failed to build chain for next blockNum:%d blockHash:%s, retrying..",
					err, nextBlock.NumberU64(), nextBlock.Hash().Hex())

				// pause, then retry
				time.Sleep(m.options.PollingInterval)
				continue
			}

			m.chain.mu.Lock()
			if m.options.WithLogs {
				m.addLogs(ctx, events)
				m.backfillChainLogs(ctx, events)
			} else {
				for _, b := range events {
					b.Logs = nil // nil it out to be clear to subscribers
					b.OK = true
				}
			}
			m.chain.mu.Unlock()

			// publish events
			err = m.publish(ctx, events)
			if err != nil {
				// failing to publish is considered a rare, but fatal error.
				// the only time this happens is if we fail to push an event to the publish queue.
				return superr.New(ErrFatal, err)
			}

			// clear events sink
			events = Blocks{}
		}
	}
}

func (m *Monitor) buildCanonicalChain(ctx context.Context, nextBlock *types.Block, nextBlockPayload []byte, events Blocks) (Blocks, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	headBlock := m.chain.Head()

	m.log.Debugf("ethmonitor: new block #%d hash:%s prevHash:%s numTxns:%d",
		nextBlock.NumberU64(), nextBlock.Hash().String(), nextBlock.ParentHash().String(), len(nextBlock.Transactions()))

	if headBlock == nil || nextBlock.ParentHash() == headBlock.Hash() {
		// block-chaining it up
		block := &Block{Event: Added, Block: nextBlock, BlockPayload: m.setPayload(nextBlockPayload)}
		events = append(events, block)
		return events, m.chain.push(block)
	}

	// next block doest match prevHash, therefore we must pop our previous block and recursively
	// rebuild the canonical chain
	poppedBlock := *m.chain.pop() // assign by value so it won't be mutated later
	poppedBlock.Event = Removed
	poppedBlock.OK = true // removed blocks are ready

	// purge the block num from the cache
	if m.cache != nil {
		key := cacheKeyBlockNum(m.chainID, poppedBlock.Number())
		err := m.cache.Delete(ctx, key)
		if err != nil {
			m.log.Warnf("ethmonitor: error deleting block cache for block num %d due to: '%v'", err, poppedBlock.Number().Uint64())
		}
	}

	m.log.Debugf("ethmonitor: block reorg, reverting block #%d hash:%s prevHash:%s", poppedBlock.NumberU64(), poppedBlock.Hash().Hex(), poppedBlock.ParentHash().Hex())
	events = append(events, &poppedBlock)

	// let's always take a pause between any reorg for the polling interval time
	// to allow nodes to sync to the correct chain
	pause := calc.Max(2*m.options.PollingInterval, 2*time.Second)
	time.Sleep(pause)

	// Fetch/connect the broken chain backwards by traversing recursively via parent hashes
	nextParentBlock, nextParentBlockPayload, err := m.fetchBlockByHash(ctx, nextBlock.ParentHash())
	if err != nil {
		// NOTE: this is okay, it will auto-retry
		return events, err
	}

	events, err = m.buildCanonicalChain(ctx, nextParentBlock, nextParentBlockPayload, events)
	if err != nil {
		// NOTE: this is okay, it will auto-retry
		return events, err
	}

	block := &Block{Event: Added, Block: nextBlock, BlockPayload: m.setPayload(nextBlockPayload)}
	err = m.chain.push(block)
	if err != nil {
		return events, err
	}
	events = append(events, block)

	return events, nil
}

func (m *Monitor) addLogs(ctx context.Context, blocks Blocks) {
	tctx, cancel := context.WithTimeout(ctx, m.options.Timeout)
	defer cancel()

	for _, block := range blocks {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// skip, we already have logs for this block or its a removed block
		if block.OK {
			continue
		}

		// do not attempt to get logs for re-org'd blocks as the data
		// will be inconsistent and may never be available.
		if block.Event == Removed {
			block.OK = true
			continue
		}

		blockHash := block.Hash()

		topics := [][]common.Hash{}
		if len(m.options.LogTopics) > 0 {
			topics = append(topics, m.options.LogTopics)
		}

		logs, logsPayload, err := m.filterLogs(tctx, blockHash, topics)

		if err == nil {
			// check the logsBloom from the block to check if we should be expecting logs. logsBloom
			// will be included for any indexed logs.
			if len(logs) > 0 || block.Bloom() == (types.Bloom{}) {
				// successful backfill
				if logs == nil {
					block.Logs = []types.Log{}
				} else {
					block.Logs = logs
				}
				block.LogsPayload = m.setPayload(logsPayload)
				block.OK = true
				continue
			}
		}

		// mark for backfilling
		block.Logs = nil
		block.OK = false

		// NOTE: we do not error here as these logs will be backfilled before they are published anyways,
		// but we log the error anyways.
		m.log.Infof("ethmonitor: [getLogs failed -- marking block %s for log backfilling] %v", blockHash.Hex(), err)
	}
}

func (m *Monitor) filterLogs(ctx context.Context, blockHash common.Hash, topics [][]common.Hash) ([]types.Log, []byte, error) {
	getter := func(ctx context.Context, _ string) ([]byte, error) {
		m.log.Debugf("ethmonitor: filterLogs is calling origin for block hash %s", blockHash)

		tctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()

		logsPayload, err := m.provider.RawFilterLogs(tctx, ethereum.FilterQuery{
			BlockHash: &blockHash,
			Topics:    topics,
		})
		return logsPayload, err
	}

	if m.cache == nil {
		resp, err := getter(ctx, "")
		if err != nil {
			return nil, resp, err
		}
		logs, err := unmarshalLogs(resp)
		return logs, resp, err
	}

	topicsDigest := xxhash.New()
	for _, hashes := range topics {
		for _, hash := range hashes {
			topicsDigest.Write(hash.Bytes())
		}
		topicsDigest.Write([]byte{'\n'})
	}

	key := fmt.Sprintf("ethmonitor:%s:Logs:hash=%s;topics=%d", m.chainID.String(), blockHash.String(), topicsDigest.Sum64())
	resp, err := m.cache.GetOrSetWithLockEx(ctx, key, getter, m.options.CacheExpiry)
	if err != nil {
		return nil, resp, err
	}
	logs, err := unmarshalLogs(resp)
	return logs, resp, err
}

func (m *Monitor) backfillChainLogs(ctx context.Context, newBlocks Blocks) {
	// Backfill logs for failed getLog calls across the retained chain.

	// In cases of re-orgs and inconsistencies with node state, in certain cases
	// we have to backfill log fetching and send an updated block event to subscribers.

	// We start by looking through our entire blocks retention for addLogs failed
	// and attempt to fetch the logs again for the same block object.
	//
	// NOTE: we only back-fill 'Added' blocks, as any 'Removed' blocks could be reverted
	// and their logs will never be available from a node.
	blocks := m.chain.blocks

	for i := len(blocks) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// check if this was a recently added block in the same cycle to avoid
		// making extra backfill calls which just happened before call to backfillChainLogs(..)
		if len(newBlocks) > 0 {
			_, ok := newBlocks.FindBlock(blocks[i].Hash())
			if ok {
				continue
			}
		}

		// attempt to backfill if necessary
		if !blocks[i].OK {
			m.addLogs(ctx, Blocks{blocks[i]})
			if blocks[i].Event == Added && blocks[i].OK {
				m.log.Infof("ethmonitor: [getLogs backfill successful for block:%d %s]", blocks[i].NumberU64(), blocks[i].Hash().Hex())
			}
		}
	}
}

func (m *Monitor) fetchNextBlock(ctx context.Context) (*types.Block, []byte, bool, error) {
	miss := false

	getter := func(ctx context.Context, _ string) ([]byte, error) {
		m.log.Debugf("ethmonitor: fetchNextBlock is calling origin for number %s", m.nextBlockNumber)
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			nextBlockPayload, err := m.fetchRawBlockByNumber(ctx, m.nextBlockNumber)
			if errors.Is(err, ethereum.NotFound) {
				miss = true
				if m.provider.IsStreamingEnabled() {
					// in streaming mode, we'll use a shorter time to pause before we refetch
					time.Sleep(200 * time.Millisecond)
				} else {
					time.Sleep(m.options.PollingInterval)
				}
				continue
			}
			if err != nil {
				m.log.Warnf("ethmonitor: [retrying] failed to fetch next block # %d, due to: %v", m.nextBlockNumber, err)
				miss = true
				time.Sleep(m.options.PollingInterval)
				continue
			}

			return nextBlockPayload, nil
		}
	}

	var nextBlockNumber *big.Int
	m.nextBlockNumberMu.Lock()
	if m.nextBlockNumber != nil {
		nextBlockNumber = big.NewInt(0).Set(m.nextBlockNumber)
	}
	m.nextBlockNumberMu.Unlock()

	// skip cache if isn't provided, or in case when nextBlockNumber is nil (latest)
	if m.cache == nil || nextBlockNumber == nil {
		resp, err := getter(ctx, "")
		if err != nil {
			return nil, resp, miss, err
		}
		block, err := unmarshalBlock(resp)
		return block, resp, miss, err
	}

	// fetch with distributed mutex
	key := cacheKeyBlockNum(m.chainID, nextBlockNumber)
	resp, err := m.cache.GetOrSetWithLockEx(ctx, key, getter, m.options.CacheExpiry)
	if err != nil {
		return nil, resp, miss, err
	}
	block, err := unmarshalBlock(resp)
	return block, resp, miss, err
}

func cacheKeyBlockNum(chainID *big.Int, num *big.Int) string {
	return fmt.Sprintf("ethmonitor:%s:BlockNum:%s", chainID.String(), num.String())
}

func (m *Monitor) fetchRawBlockByNumber(ctx context.Context, num *big.Int) ([]byte, error) {
	m.log.Debugf("ethmonitor: fetchRawBlockByNumber is calling origin for number %s", num)
	maxErrAttempts, errAttempts := 3, 0 // quick retry in case of short-term node connection failures

	var blockPayload []byte
	var err error

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if errAttempts >= maxErrAttempts {
			m.log.Warnf("ethmonitor: fetchBlockByNumber hit maxErrAttempts after %d tries for block num %v due to %v", errAttempts, num, err)
			return nil, superr.New(ErrMaxAttempts, err)
		}

		tctx, cancel := context.WithTimeout(ctx, m.options.Timeout)
		defer cancel()

		blockPayload, err = m.provider.RawBlockByNumber(tctx, num)
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				return nil, ethereum.NotFound
			} else {
				m.log.Warnf("ethmonitor: fetchBlockByNumber failed due to: %v", err)
				errAttempts++
				time.Sleep(time.Duration(errAttempts) * time.Second)
				continue
			}
		}
		return blockPayload, nil
	}
}

func (m *Monitor) fetchBlockByHash(ctx context.Context, hash common.Hash) (*types.Block, []byte, error) {
	getter := func(ctx context.Context, _ string) ([]byte, error) {
		m.log.Debugf("ethmonitor: fetchBlockByHash is calling origin for hash %s", hash)

		maxNotFoundAttempts, notFoundAttempts := 2, 0 // waiting for node to sync
		maxErrAttempts, errAttempts := 2, 0           // quick retry in case of short-term node connection failures

		var blockPayload []byte
		var err error

		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			if notFoundAttempts >= maxNotFoundAttempts {
				return nil, ethereum.NotFound
			}
			if errAttempts >= maxErrAttempts {
				m.log.Warnf("ethmonitor: fetchBlockByHash hit maxErrAttempts after %d tries for block hash %s due to %v", errAttempts, hash.Hex(), err)
				return nil, superr.New(ErrMaxAttempts, err)
			}

			blockPayload, err = m.provider.RawBlockByHash(ctx, hash)
			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					notFoundAttempts++
					time.Sleep(time.Duration(notFoundAttempts) * time.Second)
					continue
				} else {
					errAttempts++
					time.Sleep(time.Duration(errAttempts) * time.Second)
					continue
				}
			}
			if len(blockPayload) > 0 {
				return blockPayload, nil
			}
		}
	}

	// skip if cache isn't provided
	if m.cache == nil {
		resp, err := getter(ctx, "")
		if err != nil {
			return nil, nil, err
		}
		block, err := unmarshalBlock(resp)
		return block, nil, err
	}

	// fetch with distributed mutex
	key := fmt.Sprintf("ethmonitor:%s:BlockHash:%s", m.chainID.String(), hash.String())
	resp, err := m.cache.GetOrSetWithLockEx(ctx, key, getter, m.options.CacheExpiry)
	if err != nil {
		return nil, nil, err
	}
	block, err := unmarshalBlock(resp)
	return block, resp, err
}

func (m *Monitor) publish(ctx context.Context, events Blocks) error {
	// skip publish enqueuing if there are no subscribers
	m.mu.Lock()
	if len(m.subscribers) == 0 {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	// Check for trail-behind-head mode and set maxBlockNum if applicable
	maxBlockNum := uint64(0)
	if m.options.TrailNumBlocksBehindHead > 0 {
		maxBlockNum = m.LatestBlock().NumberU64() - uint64(m.options.TrailNumBlocksBehindHead)
	}

	// Enqueue
	err := m.publishQueue.enqueue(events)
	if err != nil {
		return err
	}

	// Publish events existing in the queue
	pubEvents, ok := m.publishQueue.dequeue(maxBlockNum)
	if ok {
		m.publishCh <- pubEvents
	}

	return nil
}

func (m *Monitor) broadcast(events Blocks) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sub := range m.subscribers {
		sub.ch.Send(events)
	}
}

func (m *Monitor) Subscribe(optLabel ...string) Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()

	var label string
	if len(optLabel) > 0 {
		label = optLabel[0]
	}

	subscriber := &subscriber{
		ch: channel.NewUnboundedChan[Blocks](10, 5000, channel.Options{
			Logger:  m.log,
			Alerter: m.alert,
			Label:   label,
		}),
		done: make(chan struct{}),
	}

	subscriber.unsubscribe = func() {
		close(subscriber.done)
		subscriber.ch.Close()
		subscriber.ch.Flush()

		m.mu.Lock()
		defer m.mu.Unlock()

		for i, sub := range m.subscribers {
			if sub == subscriber {
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				return
			}
		}
	}

	m.subscribers = append(m.subscribers, subscriber)

	return subscriber
}

func (m *Monitor) Chain() *Chain {
	return m.chain
}

// LatestBlock will return the head block of the canonical chain
func (m *Monitor) LatestBlock() *Block {
	return m.chain.Head()
}

// LatestBlockNum returns the latest block number in the canonical chain
func (m *Monitor) LatestBlockNum() *big.Int {
	latestBlock := m.LatestBlock()
	if latestBlock == nil {
		return big.NewInt(0)
	} else {
		return big.NewInt(0).Set(latestBlock.Number())
	}
}

// LatestReadyBlock returns the latest block in the canonical chain
// which has block.OK state to true, as in all details are available for the block.
func (m *Monitor) LatestReadyBlock() *Block {
	return m.chain.ReadyHead()
}

// LatestReadyBlockNum returns the latest block number in the canonical chain
// which has block.OK state to true, as in all details are available for the block.
func (m *Monitor) LatestReadyBlockNum() *big.Int {
	latestBlock := m.LatestReadyBlock()
	if latestBlock == nil {
		return big.NewInt(0)
	} else {
		return big.NewInt(0).Set(latestBlock.Number())
	}
}

// LatestFinalBlock returns the latest block which has reached finality.
// The argument `numBlocksToFinality` should be a constant value of the number
// of blocks a particular chain needs to reach finality. Ie. on Polygon this
// value would be 120 and on Ethereum it would be 20. As the pubsub system
// publishes new blocks, this value will change, as the chain will progress
// forward. It's recommend / safe to call this method each time in a <-sub.Blocks()
// code block.
func (m *Monitor) LatestFinalBlock(numBlocksToFinality int) *Block {
	m.chain.mu.Lock()
	defer m.chain.mu.Unlock()

	n := len(m.chain.blocks)
	if n < numBlocksToFinality+1 {
		// not enough blocks have been monitored yet
		return nil
	} else {
		// return the block at finality position from the canonical chain
		return m.chain.blocks[n-numBlocksToFinality-1]
	}
}

func (m *Monitor) OldestBlockNum() *big.Int {
	oldestBlock := m.chain.Tail()
	if oldestBlock == nil {
		return big.NewInt(0)
	} else {
		return big.NewInt(0).Set(oldestBlock.Number())
	}
}

// GetBlock will search the retained blocks for the hash
func (m *Monitor) GetBlock(blockHash common.Hash) *Block {
	return m.chain.GetBlock(blockHash)
}

// GetBlock will search within the retained canonical chain for the txn hash. Passing `optMined true`
// will only return transaction which have not been removed from the chain via a reorg.
func (m *Monitor) GetTransaction(txnHash common.Hash) (*types.Transaction, Event) {
	return m.chain.GetTransaction(txnHash)
}

// GetAverageBlockTime returns the average block time in seconds (including fractions)
func (m *Monitor) GetAverageBlockTime() float64 {
	return m.chain.GetAverageBlockTime()
}

func (m *Monitor) NumSubscribers() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.subscribers)
}

func (m *Monitor) UnsubscribeAll(err error) {
	m.mu.Lock()
	var subs []*subscriber
	subs = append(subs, m.subscribers...)
	m.mu.Unlock()

	for _, sub := range subs {
		sub.err = err
		sub.Unsubscribe()
	}
}

// PurgeHistory clears all but the head of the chain. Useful for tests, but should almost
// never be used in a normal application.
func (m *Monitor) PurgeHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.chain.blocks) > 1 {
		m.chain.mu.Lock()
		defer m.chain.mu.Unlock()
		m.chain.blocks = m.chain.blocks[1:1]
	}
}

func (m *Monitor) setPayload(value []byte) []byte {
	if m.options.RetainPayloads {
		return value
	} else {
		return nil
	}
}

func getChainID(ctx context.Context, provider ethrpc.Interface) (*big.Int, error) {
	var chainID *big.Int
	err := breaker.Do(ctx, func() error {
		ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()

		id, err := provider.ChainID(ctx)
		if err != nil {
			return err
		}
		chainID = id
		return nil
	}, nil, 1*time.Second, 2, 3)

	if err != nil {
		return nil, err
	}

	return chainID, nil
}

func clampDuration(x, y time.Duration) time.Duration {
	if x > y {
		return x
	} else {
		return y
	}
}

func unmarshalBlock(blockPayload []byte) (*types.Block, error) {
	var block *types.Block
	err := ethrpc.IntoBlock(blockPayload, &block)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func unmarshalLogs(logsPayload []byte) ([]types.Log, error) {
	var logs []types.Log
	err := json.Unmarshal(logsPayload, &logs)
	if err != nil {
		return nil, err
	}
	return logs, nil
}
