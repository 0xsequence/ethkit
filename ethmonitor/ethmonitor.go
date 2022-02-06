package ethmonitor

import (
	"context"
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
	"github.com/goware/superr"
)

var DefaultOptions = Options{
	Logger:                   util.NewLogger(util.LogLevel_WARN),
	PollingInterval:          1000 * time.Millisecond,
	Timeout:                  60 * time.Second,
	StartBlockNumber:         nil, // latest
	TrailNumBlocksBehindHead: 0,   // latest
	BlockRetentionLimit:      200,
	WithLogs:                 false,
	LogTopics:                []common.Hash{}, // all logs
	DebugLogging:             false,
	StrictSubscribers:        true,
}

type Options struct {
	// ..
	Logger util.Logger

	// ..
	PollingInterval time.Duration

	// ..
	Timeout time.Duration

	// ..
	StartBlockNumber *big.Int

	// ..
	TrailNumBlocksBehindHead int

	// ..
	BlockRetentionLimit int

	// ..
	WithLogs bool

	// ..
	LogTopics []common.Hash

	// ..
	DebugLogging bool

	// StrictSubscribers when enabled will force monitor to block if a subscriber doesn't
	// consume the message from its channel (the default). When false, it means subscribers
	// will get always the latest information even if another is lagging to consume.
	StrictSubscribers bool
}

var (
	ErrFatal                 = errors.New("ethmonitor: fatal error, stopping.")
	ErrReorg                 = errors.New("ethmonitor: block reorg")
	ErrUnexpectedParentHash  = errors.New("ethmonitor: unexpected parent hash")
	ErrUnexpectedBlockNumber = errors.New("ethmonitor: unexpected block number")
	ErrQueueFull             = errors.New("ethmonitor: publish queue is full")
	ErrMaxAttempts           = errors.New("ethmonitor: max attempts hit")
)

type Monitor struct {
	options Options

	log      util.Logger
	provider *ethrpc.Provider

	chain           *Chain
	nextBlockNumber *big.Int

	publishCh    chan Blocks
	publishQueue *queue
	subscribers  []*subscriber

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
	mu      sync.RWMutex
}

func NewMonitor(provider *ethrpc.Provider, opts ...Options) (*Monitor, error) {
	options := DefaultOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	if options.Logger == nil {
		return nil, fmt.Errorf("ethmonitor: logger is nil")
	}

	options.BlockRetentionLimit += options.TrailNumBlocksBehindHead

	if options.DebugLogging {
		stdLogger, ok := options.Logger.(*util.StdLogAdapter)
		if ok {
			stdLogger.Level = util.LogLevel_DEBUG
		}
	}

	return &Monitor{
		options:      options,
		log:          options.Logger,
		provider:     provider,
		chain:        newChain(options.BlockRetentionLimit),
		publishCh:    make(chan Blocks),
		publishQueue: newQueue(options.BlockRetentionLimit * 2),
		subscribers:  make([]*subscriber, 0),
	}, nil
}

func (m *Monitor) Run(ctx context.Context) error {
	if m.IsRunning() {
		return fmt.Errorf("ethmonitor: already running")
	}

	m.ctx, m.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&m.running, 1)
	defer atomic.StoreInt32(&m.running, 0)

	// Start from latest, or start from a specific block number
	if m.chain.Head() != nil {
		m.nextBlockNumber = m.chain.Head().Number()
	} else if m.options.StartBlockNumber != nil {
		m.nextBlockNumber = m.options.StartBlockNumber
	}

	if m.nextBlockNumber == nil {
		m.log.Info("ethmonitor: starting from block=latest")
	} else {
		if m.chain.Head() == nil {
			m.log.Infof("ethmonitor: starting from block=%d", m.nextBlockNumber)
		} else {
			m.log.Infof("ethmonitor: starting from block=%d", m.nextBlockNumber.Uint64()+1)
		}
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
	return m.monitor()
}

func (m *Monitor) Stop() {
	m.log.Info("ethmonitor: stop")
	m.ctxStop()
}

func (m *Monitor) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

func (m *Monitor) Options() Options {
	return m.options
}

func (m *Monitor) Provider() *ethrpc.Provider {
	return m.provider
}

func (m *Monitor) monitor() error {
	ctx := m.ctx
	events := Blocks{}

	for {
		select {

		case <-m.ctx.Done():
			return nil

		case <-time.After(m.options.PollingInterval):
			headBlock := m.chain.Head()
			if headBlock != nil {
				m.nextBlockNumber = big.NewInt(0).Add(headBlock.Number(), big.NewInt(1))
			}

			nextBlock, err := m.fetchBlockByNumber(ctx, m.nextBlockNumber)
			if err == ethereum.NotFound {
				continue
			}
			if err != nil {
				m.log.Warnf("ethmonitor: [retrying] failed to fetch next block # %d, due to: %v", m.nextBlockNumber, err)
				continue
			}

			events, err = m.buildCanonicalChain(ctx, nextBlock, events)
			if err != nil {
				m.log.Warnf("ethmonitor: error reported '%v', failed to build chain for next blockNum:%d blockHash:%s, retrying..",
					err, nextBlock.NumberU64(), nextBlock.Hash().Hex())

				// pause, then retry
				time.Sleep(m.options.PollingInterval)
				continue
			}

			if m.options.WithLogs {
				m.addLogs(ctx, events)
				m.backfillChainLogs(ctx)
			} else {
				for _, b := range events {
					b.Logs = nil // nil it out to be clear to subscribers
					b.OK = true
				}
			}

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

func (m *Monitor) buildCanonicalChain(ctx context.Context, nextBlock *types.Block, events Blocks) (Blocks, error) {
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
		block := &Block{Event: Added, Block: nextBlock}
		events = append(events, block)
		return events, m.chain.push(block)
	}

	// next block doest match prevHash, therefore we must pop our previous block and recursively
	// rebuild the canonical chain
	poppedBlock := m.chain.pop()
	poppedBlock.Event = Removed
	poppedBlock.OK = true // removed blocks are ready

	m.log.Debugf("ethmonitor: block reorg, reverting block #%d hash:%s prevHash:%s", poppedBlock.NumberU64(), poppedBlock.Hash().Hex(), poppedBlock.ParentHash().Hex())
	events = append(events, poppedBlock)

	// let's always take a pause between any reorg for the polling interval time
	// to allow nodes to sync to the correct chain
	pause := m.options.PollingInterval * time.Duration(len(events))
	time.Sleep(pause)

	// Fetch/connect the broken chain backwards by traversing recursively via parent hashes
	nextParentBlock, err := m.fetchBlockByHash(ctx, nextBlock.ParentHash())
	if err != nil {
		// NOTE: this is okay, it will auto-retry
		return events, err
	}

	events, err = m.buildCanonicalChain(ctx, nextParentBlock, events)
	if err != nil {
		// NOTE: this is okay, it will auto-retry
		return events, err
	}

	block := &Block{Event: Added, Block: nextBlock}
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

		logs, err := m.provider.FilterLogs(tctx, ethereum.FilterQuery{
			BlockHash: &blockHash,
			Topics:    topics,
		})

		if err == nil {
			if logs != nil {
				emptyBloom := emptyLogsBloom(block.Block.Bloom().Bytes())
				if (len(logs) == 0 && emptyBloom) || (len(logs) > 0 && !emptyBloom) {
					block.Logs = logs
					block.OK = true
				}
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

func (m *Monitor) backfillChainLogs(ctx context.Context) {
	// Backfill logs for failed getLog calls across the retained chain.

	// In cases of re-orgs and inconsistencies with node state, in certain cases
	// we have to backfill log fetching and send an updated block event to subscribers.

	// We start by looking through our entire blocks retention for addLogs failed
	// and attempt to fetch the logs again for the same block object.
	//
	// NOTE: we only back-fill 'Added' blocks, as any 'Removed' blocks could be reverted
	// and their logs will never be available from a node.
	blocks := m.chain.Blocks()

	for i := len(blocks) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !blocks[i].OK {
			m.addLogs(ctx, Blocks{blocks[i]})
			if blocks[i].Event == Added && blocks[i].OK {
				m.log.Infof("ethmonitor: [getLogs backfill successful for block:%d %s]", blocks[i].NumberU64(), blocks[i].Hash().Hex())
			}
		}
	}
}

func (m *Monitor) fetchBlockByNumber(ctx context.Context, num *big.Int) (*types.Block, error) {
	maxErrAttempts, errAttempts := 10, 0 // in case of node connection failures

	var block *types.Block
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

		block, err = m.provider.BlockByNumber(tctx, num)
		if err != nil {
			if err == ethereum.NotFound {
				return nil, ethereum.NotFound
			} else {
				errAttempts++
				time.Sleep(m.options.PollingInterval * time.Duration(errAttempts) * 2)
				continue
			}
		}
		return block, nil
	}
}

func (m *Monitor) fetchBlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	maxNotFoundAttempts, notFoundAttempts := 4, 0 // waiting for node to sync
	maxErrAttempts, errAttempts := 10, 0          // in case of node connection failures

	var block *types.Block
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

		block, err = m.provider.BlockByHash(ctx, hash)
		if err != nil {
			if err == ethereum.NotFound {
				notFoundAttempts++
				time.Sleep(m.options.PollingInterval * time.Duration(notFoundAttempts) * 2)
				continue
			} else {
				errAttempts++
				time.Sleep(m.options.PollingInterval * time.Duration(errAttempts) * 2)
				continue
			}
		}
		if block != nil {
			return block, nil
		}
	}
}

func (m *Monitor) publish(ctx context.Context, events Blocks) error {
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

	// NOTE: small edge case, where.. we could "publish" a block which we don't have logs for.. which would enqueue it
	// but not send it ..
	// then, turns out, we need to revert it.. and previous value was also updated..
	// technically should be an add then remove.. so maybe on enqueue we copy the details? i think then OK will dead-lock

	// we prob just need to de-dupe the queue..

	// add method...... dedupe() .. or .purge() .. which will purge unpublished blocks overlapping etc.
	// and it will also solve the trailing-behind issue too..

	// Publish events existing in the queue
	pubEvents, ok := m.publishQueue.dequeue(maxBlockNum)
	if ok {
		m.publishCh <- pubEvents
	}

	return nil
}

func (m *Monitor) broadcast(events Blocks) {
	for _, sub := range m.subscribers {
		if m.options.StrictSubscribers {
			// blocking -- will block the monitor if a subscriber doesn't consume in time
		RETRY:
			select {
			case <-m.ctx.Done():
			case <-sub.done:
			case sub.ch <- events:
			case <-time.After(4 * time.Second):
				// lets log whenever we're blocking the monitor, then continue again.
				m.log.Warn("warning! a subscriber is falling behind (delayed for 4 seconds), as a result the monitor is being held back")
				goto RETRY
			}
		} else {
			// non-blocking -- if a subscriber can't consume it fast enough, then it will miss the batch
			// and the monitor will continue.
			select {
			case <-sub.done:
			case sub.ch <- events:
			default:
			}
		}
	}
}

func (m *Monitor) Subscribe() Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscriber := &subscriber{
		ch:   make(chan Blocks),
		done: make(chan struct{}),
	}

	subscriber.unsubscribe = func() {
		close(subscriber.done)
		m.mu.Lock()
		defer m.mu.Unlock()
		close(subscriber.ch)
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
	m.chain.mu.Lock()
	defer m.chain.mu.Unlock()
	blocks := make(Blocks, len(m.chain.blocks))
	copy(blocks, m.chain.blocks)
	return &Chain{
		blocks: blocks,
	}
}

// LatestBlock will return the head block of the retained chain
func (m *Monitor) LatestBlock() *Block {
	return m.chain.Head()
}

// GetBlock will search the retained blocks for the hash
func (m *Monitor) GetBlock(hash common.Hash) *Block {
	return m.chain.GetBlock(hash)
}

// GetBlock will search within the retained blocks for the txn hash
func (m *Monitor) GetTransaction(hash common.Hash) *types.Transaction {
	return m.chain.GetTransaction(hash)
}
