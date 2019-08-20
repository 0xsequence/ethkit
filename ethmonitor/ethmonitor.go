package ethmonitor

import (
	"context"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/horizon-games/ethkit/ethrpc"
	"github.com/pkg/errors"
)

var DefaultOptions = Options{
	PollingInterval:        1 * time.Second,
	PollingTimeout:         30 * time.Second,
	StartBlockNumber:       nil, // latest
	BlockRetentionLimit:    20,
	BlockConfirmationDepth: 12,
	WithLogs:               true,
	LogTopics:              []common.Hash{}, // all logs
}

type Options struct {
	PollingInterval        time.Duration
	PollingTimeout         time.Duration
	StartBlockNumber       *big.Int
	BlockRetentionLimit    int
	BlockConfirmationDepth int
	WithLogs               bool
	LogTopics              []common.Hash
}

type EventType uint32

const (
	Added = iota
	Removed
	Confirmed
)

type Event struct {
	Type  EventType
	Block *Block
}

type Monitor struct {
	options Options

	ctx     context.Context
	ctxStop context.CancelFunc

	provider    *ethrpc.JSONRPC
	chain       *Chain
	subscribers []chan<- []Event

	started bool
	mu      sync.RWMutex
}

func NewMonitor(provider *ethrpc.JSONRPC, opts ...Options) (*Monitor, error) {
	options := DefaultOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	return &Monitor{
		options:     options,
		provider:    provider,
		chain:       newChain(options.BlockRetentionLimit),
		subscribers: make([]chan<- []Event, 0),
	}, nil
}

func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return errors.Errorf("already started")
	}
	m.started = true
	m.mu.Unlock()

	m.ctx, m.ctxStop = context.WithCancel(ctx)

	go m.poll()

	return nil
}

func (m *Monitor) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ctxStop()
	m.started = false
	return nil
}

func (m *Monitor) poll() {
	ticker := time.NewTicker(m.options.PollingInterval)
	defer ticker.Stop()

	var nextBlockNumber *big.Int

	for {
		select {
		case <-ticker.C:
			// fmt.Println("tick")

			if m.ctx.Err() == context.Canceled {
				return
			}

			// Max time for any tick to execute in
			ctx, cancelFunc := context.WithTimeout(context.Background(), m.options.PollingTimeout)
			defer cancelFunc()

			headBlock := m.chain.Head()
			if headBlock == nil {
				nextBlockNumber = nil // starting block.. (latest)
			} else {
				nextBlockNumber = big.NewInt(0).Add(headBlock.Number(), big.NewInt(1))
			}

			nextBlock, err := m.fetchBlockByNumber(ctx, nextBlockNumber)
			if err == ethereum.NotFound {
				continue
			}
			if err != nil {
				log.Fatal(err) // TODO: how to handle..? depends on error..
			}

			// fmt.Println("ethrpc returns block number:", nextBlock.NumberU64())

			events := []Event{}
			events, err = m.buildCanonicalChain(ctx, nextBlock, events)
			if err != nil {
				if err == ethereum.NotFound {
					// TODO: log
					continue // lets retry this
				}
				// TODO
				panic(errors.Errorf("canon err: %v", err))
			}

			// TODO: add logs to event blocks..
			if m.options.WithLogs {
				m.addLogs(ctx, events)
			}

			// fmt.Println("len..", len(m.chain.blocks))
			// m.chain.PrintAllBlocks()
			// fmt.Println("")

			// notify all subscribers..
			for _, sub := range m.subscribers {
				// TODO: hmm, we should never push to a closed channel, or it will panic.
				// what happens if the subscriber goroutine just goes away?

				// do not block
				select {
				case sub <- events:
				default:
				}
			}

		case <-m.ctx.Done():
			// monitor has stopped
			return
		}
	}
}

func (m *Monitor) buildCanonicalChain(ctx context.Context, nextBlock *Block, events []Event) ([]Event, error) {
	headBlock := m.chain.Head()
	if headBlock == nil || nextBlock.ParentHash() == headBlock.Hash() {
		events = append(events, Event{Type: Added, Block: nextBlock})
		return events, m.chain.push(nextBlock)
	}

	// remove it, not the right block
	poppedBlock := m.chain.pop()
	events = append(events, Event{Type: Removed, Block: poppedBlock})

	nextParentBlock, err := m.fetchBlockByHash(ctx, nextBlock.ParentHash())
	if err != nil {
		return events, err
	}

	events, err = m.buildCanonicalChain(ctx, nextParentBlock, events)
	if err != nil {
		return events, err
	}

	err = m.chain.push(nextBlock)
	if err != nil {
		return events, err
	}
	events = append(events, Event{Type: Added, Block: nextBlock})

	return events, nil
}

func (m *Monitor) addLogs(ctx context.Context, events []Event) {
	for _, ev := range events {
		if ev.Block.Logs != nil || len(ev.Block.Logs) > 0 {
			continue
		}
		blockHash := ev.Block.Hash()
		logs, err := m.provider.FilterLogs(ctx, ethereum.FilterQuery{
			BlockHash: &blockHash,
			Topics:    [][]common.Hash{m.options.LogTopics},
		})
		if logs != nil {
			ev.Block.Logs = logs
		}
		if err != nil {
			_ = err // TODO: print to logger
		}
	}
}

func (m *Monitor) fetchBlockByNumber(ctx context.Context, number *big.Int) (*Block, error) {
	block, err := m.provider.BlockByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	return &Block{Block: block}, nil
}

func (m *Monitor) fetchBlockByHash(ctx context.Context, hash common.Hash) (*Block, error) {
	block, err := m.provider.BlockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	return &Block{Block: block}, nil
}

func (m *Monitor) Chain() *Chain {
	blocks := make([]*Block, len(m.chain.blocks))
	copy(blocks, m.chain.blocks)
	return &Chain{
		blocks: blocks,
	}
}

func (m *Monitor) GetLatestBlock() *Block {
	return m.chain.Head()
}

// TODO: add FindBlockHash()
//

// TODO: add FindTransactionHash()
//

// TODO: added or removed.......
func (m *Monitor) Subscribe(sub chan<- []Event) func() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.subscribers = append(m.subscribers, sub)

	unsubscribe := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i, s := range m.subscribers {
			if s == sub {
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				return
			}
		}
	}

	return unsubscribe
}
