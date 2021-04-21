package ethmonitor

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
)

var DefaultOptions = Options{
	Logger:              log.New(os.Stdout, "", 0),
	PollingInterval:     1 * time.Second,
	PollingTimeout:      120 * time.Second,
	StartBlockNumber:    nil, // latest
	BlockRetentionLimit: 100,
	WithLogs:            true,
	LogTopics:           []common.Hash{}, // all logs
	DebugLogging:        false,
}

type Options struct {
	Logger              util.Logger
	PollingInterval     time.Duration
	PollingTimeout      time.Duration
	StartBlockNumber    *big.Int
	BlockRetentionLimit int
	WithLogs            bool
	LogTopics           []common.Hash
	DebugLogging        bool
}

type Monitor struct {
	options Options

	ctx     context.Context
	ctxStop context.CancelFunc

	log             util.Logger
	provider        *ethrpc.Provider
	chain           *Chain
	subscribers     []*subscriber
	nextBlockNumber *big.Int

	started bool
	running sync.WaitGroup
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

	return &Monitor{
		options:     options,
		log:         options.Logger,
		provider:    provider,
		chain:       newChain(options.BlockRetentionLimit),
		subscribers: make([]*subscriber, 0),
	}, nil
}

func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("already started")
	}
	m.started = true

	// Start anew, or resume
	if m.chain.Head() == nil && m.options.StartBlockNumber != nil {
		m.nextBlockNumber = m.options.StartBlockNumber
	} else if m.chain.Head() != nil {
		m.nextBlockNumber = m.chain.Head().Number()
	}
	if m.nextBlockNumber == nil {
		m.debugLogf("ethmonitor: starting from block=latest")
	} else {
		if m.chain.Head() == nil {
			m.debugLogf("ethmonitor: starting from block=%d", m.nextBlockNumber)
		} else {
			m.debugLogf("ethmonitor: starting from block=%d", m.nextBlockNumber.Uint64()+1)
		}
	}

	m.ctx, m.ctxStop = context.WithCancel(ctx)

	go func() {
		m.running.Add(1)
		defer m.running.Done()
		m.poll(m.ctx)
	}()

	return nil
}

func (m *Monitor) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}

	m.debugLogf("ethmonitor: stop")
	m.started = false
	m.ctxStop()
	m.mu.Unlock()

	m.running.Wait()
	return nil
}

func (m *Monitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *Monitor) Options() Options {
	return m.options
}

func (m *Monitor) Provider() *ethrpc.Provider {
	return m.provider
}

func (m *Monitor) poll(ctx context.Context) error {
	events := Blocks{}

	// TODO: all fine and well, but what happens if we get re-org then another re-org
	// in between.. will we be fine..? or will event-source be a mess..? we might need
	// to de-dupe "events" data before publishing, or give more time between reorgs?

	for {
		select {

		case <-ctx.Done():
			return nil

		case <-time.After(m.options.PollingInterval):
			if !m.IsRunning() {
				break
			}

			// Max time for any tick to execute in
			ctx, cancelFunc := context.WithTimeout(context.Background(), m.options.PollingTimeout)
			defer cancelFunc()

			headBlock := m.chain.Head()
			if headBlock != nil {
				m.nextBlockNumber = big.NewInt(0).Add(headBlock.Number(), big.NewInt(1))
			}

			nextBlock, err := m.provider.BlockByNumber(ctx, m.nextBlockNumber)
			if err == ethereum.NotFound {
				continue
			}
			if err != nil {
				m.log.Printf("ethmonitor: [retrying] failed to fetch next block # %d, due to: %v", m.nextBlockNumber, err)
				continue
			}

			events, err = m.buildCanonicalChain(ctx, nextBlock, events)
			if err != nil {
				if err == ethereum.NotFound {
					continue // lets retry this
				}
				time.Sleep(m.options.PollingInterval) // pause, then retry
				continue
			}

			if m.options.WithLogs {
				m.addLogs(ctx, events)

				updatedBlocks := m.backfillChainLogs(ctx, events)
				if updatedBlocks != nil && len(updatedBlocks) > 0 {
					events = append(events, updatedBlocks...)
				}
			} else {
				for _, b := range events {
					b.Logs = nil // nil it out to be clear to subscribers
				}
			}

			// notify all subscribers..
			for _, sub := range m.subscribers {
				// non-blocking send
				select {
				case sub.ch <- events:
				default:
				}
			}

			// TODO: if we hit a reorg, we may want to wait 1-2 blocks
			// after the reorg so its safe, merge the event groups, and publish

			// clear events sink upon publishing it to subscribers
			events = events[:0]
		}
	}
}

func (m *Monitor) buildCanonicalChain(ctx context.Context, nextBlock *types.Block, events Blocks) (Blocks, error) {
	headBlock := m.chain.Head()

	m.debugLogf("ethmonitor: new block #%d hash:%s prevHash:%s numTxns:%d",
		nextBlock.NumberU64(), nextBlock.Hash().String(), nextBlock.ParentHash().String(), len(nextBlock.Transactions()))

	// add it, next block matches the parent hash of our head/latest block -- or its a fresh list
	if headBlock == nil || nextBlock.ParentHash() == headBlock.Hash() {

		// Ensure we don't re-add a bad block during a reorg recovery
		// if events.EventExists(nextBlock, Removed) {
		// 	// let's always take a pause between any reorg for the polling interval time
		// 	m.log.Printf("ethmonitor: reorg recovery resulted in same bad block (%d %s), pausing for %s",
		// 		nextBlock.NumberU64(), nextBlock.Hash().Hex(), m.options.PollingInterval)

		// 	time.Sleep(m.options.PollingInterval)
		// 	return events, ethereum.NotFound
		// }

		block := &Block{Event: Added, Block: nextBlock}
		events = append(events, block)
		return events, m.chain.push(block)
	}

	// next block doest match prevHash, therefore we must pop our previous block and recursively
	// rebuild the canonical chain
	poppedBlock := m.chain.pop()
	poppedBlock.Event = Removed

	m.log.Printf("ethmonitor: block reorg, reverting block #%d hash:%s", poppedBlock.NumberU64(), poppedBlock.Hash().Hex())
	events = append(events, poppedBlock)

	// let's always take a pause between any reorg for the polling interval time
	// to allow nodes to sync to the correct chain
	time.Sleep(m.options.PollingInterval)

	nextParentBlock, err := m.provider.BlockByHash(ctx, nextBlock.ParentHash())
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
	for _, block := range blocks {
		if block.Logs != nil || len(block.Logs) > 0 {
			continue
		}

		// do not attempt to get logs for re-org'd blocks as the data
		// will be inconsistent and may never be available.

		// TODO: however, if we want, we could check our ".Chain()" retention and copy over logs of removed
		// so we can give complete payload on publish..?
		if block.Event == Removed {
			continue
		}

		blockHash := block.Hash()

		topics := [][]common.Hash{}
		if len(m.options.LogTopics) > 0 {
			topics = append(topics, m.options.LogTopics)
		}

		logs, err := m.provider.FilterLogs(ctx, ethereum.FilterQuery{
			BlockHash: &blockHash,
			Topics:    topics,
		})

		if logs != nil && len(logs) > 0 {
			block.Logs = logs
			block.getLogsFailed = false
		}

		if err != nil {
			// mark for backfilling
			block.Logs = nil
			block.getLogsFailed = true
			m.log.Printf("ethmonitor: [getLogs failed -- marking block %s for log backfilling] %v", blockHash.Hex(), err)
		}
	}
}

func (m *Monitor) backfillChainLogs(ctx context.Context, polledBlocks Blocks) Blocks {
	// Backfill logs for failed getLog calls across the retained chain.

	// In cases of re-orgs and inconsistencies with node state, in certain cases
	// we have to backfill log fetching and send an updated block event to subscribers.

	// We start by looking through our entire blocks retention for "getLogsFailed"
	// blocks which were broadcast, and we attempt to fill the logs
	blocks := m.chain.Blocks()

	backfilledBlocks := Blocks{}
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].getLogsFailed {
			m.addLogs(ctx, Blocks{blocks[i]})

			// if successfully backfilled here, lets add it to the backfilled blocks list
			if blocks[i].Event == Added && !blocks[i].getLogsFailed {
				m.log.Printf("ethmonitor: [getLogs backfill successful for block %s]", blocks[i].Hash().Hex())
				backfilledBlocks = append(backfilledBlocks, blocks[i])
			}
		}
	}

	// Clean backfilled blocks that happened within the same poll cycle
	updatedBlocks := Blocks{}
	for _, backfilledBlock := range backfilledBlocks {
		_, ok := polledBlocks.FindBlock(backfilledBlock.Hash(), Added)
		if !ok {
			backfilledBlock.Event = Updated
			updatedBlocks = append(updatedBlocks, backfilledBlock)
		}
	}

	return updatedBlocks
}

func (m *Monitor) debugLogf(format string, v ...interface{}) {
	if !m.options.DebugLogging {
		return
	}
	m.log.Printf(format, v...)
}

func (m *Monitor) Subscribe() Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscriber := &subscriber{
		ch:   make(chan Blocks, 1024),
		done: make(chan struct{}),
	}

	subscriber.unsubscribe = func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, sub := range m.subscribers {
			if sub == subscriber {
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				close(subscriber.done)
				close(subscriber.ch)
				return
			}
		}
	}

	m.subscribers = append(m.subscribers, subscriber)

	return subscriber
}

func (m *Monitor) Chain() *Chain {
	blocks := make([]*Block, len(m.chain.blocks))
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
