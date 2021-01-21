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
	"github.com/0xsequence/ethkit/util"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var DefaultOptions = Options{
	Logger:              log.New(os.Stdout, "", 0),
	PollingInterval:     1 * time.Second,
	PollingTimeout:      120 * time.Second,
	StartBlockNumber:    nil, // latest
	BlockRetentionLimit: 20,
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

func (m *Monitor) poll(ctx context.Context) error {
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
				m.log.Printf("ethmonitor: [unexpected] %v", err)
				continue
			}

			m.debugLogf("ethmonitor: received next block height:%d hash:%s prevHash:%s numTxns:%d",
				nextBlock.NumberU64(), nextBlock.Hash().String(), nextBlock.ParentHash().String(), len(nextBlock.Transactions()))

			blocks := Blocks{}
			blocks, err = m.buildCanonicalChain(ctx, nextBlock, blocks)
			if err != nil {
				if err == ethereum.NotFound {
					m.log.Printf("ethmonitor: [unexpected] block %s not found", nextBlock.Hash().Hex())
					continue // lets retry this
				}
				m.log.Printf("ethmonitor: [unexpected] %v", err)
				continue
			}

			if m.options.WithLogs {
				m.addLogs(ctx, blocks)

				updatedBlocks := m.backfillChainLogs(ctx, blocks)
				if updatedBlocks != nil && len(updatedBlocks) > 0 {
					blocks = append(blocks, updatedBlocks...)
				}
			} else {
				for _, b := range blocks {
					b.Logs = nil // nil it out to be clear to subscribers
				}
			}

			// notify all subscribers..
			for _, sub := range m.subscribers {
				// do not block
				select {
				case sub.ch <- blocks:
				default:
				}
			}
		}
	}
}

func (m *Monitor) buildCanonicalChain(ctx context.Context, nextBlock *types.Block, blocks Blocks) (Blocks, error) {
	headBlock := m.chain.Head()
	if headBlock == nil || nextBlock.ParentHash() == headBlock.Hash() {
		block := &Block{Type: Added, Block: nextBlock}
		blocks = append(blocks, block)
		return blocks, m.chain.push(block)
	}

	// remove it, not the right block
	poppedBlock := m.chain.pop()
	poppedBlock.Type = Removed
	blocks = append(blocks, poppedBlock)

	nextParentBlock, err := m.provider.BlockByHash(ctx, nextBlock.ParentHash())
	if err != nil {
		return blocks, err
	}

	blocks, err = m.buildCanonicalChain(ctx, nextParentBlock, blocks)
	if err != nil {
		return blocks, err
	}

	block := &Block{Type: Added, Block: nextBlock}
	err = m.chain.push(block)
	if err != nil {
		return blocks, err
	}
	blocks = append(blocks, block)

	return blocks, nil
}

func (m *Monitor) addLogs(ctx context.Context, blocks Blocks) {
	for _, block := range blocks {
		if block.Logs != nil || len(block.Logs) > 0 {
			continue
		}

		// do not attempt to get logs for re-org'd blocks as the data
		// will be inconsistent and may never be available.
		if block.Type == Removed {
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
			m.log.Printf("ethmonitor: [getLogs error] %v", err)
		}
	}
}

func (m *Monitor) backfillChainLogs(ctx context.Context, polledBlocks Blocks) Blocks {
	// in cases of re-orgs and inconsistencies with node state, in certain cases
	// we have to backfill log fetching and send an updated block event to subscribers.

	// Backfill logs for failed getLog calls across the retained chain
	blocks := m.chain.Blocks()
	backfilledBlocks := Blocks{}
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].getLogsFailed {
			m.addLogs(ctx, Blocks{blocks[i]})
			if blocks[i].Type == Added && !blocks[i].getLogsFailed {
				backfilledBlocks = append(backfilledBlocks, blocks[i])
			}
		}
	}

	// Clean backfilled blocks that happened within the same poll cycle
	updatedBlocks := Blocks{}
	for _, backfilledBlock := range backfilledBlocks {
		_, ok := polledBlocks.FindBlock(backfilledBlock.Hash())
		if !ok {
			backfilledBlock.Type = Updated
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
