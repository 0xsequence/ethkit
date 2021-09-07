package ethmonitor

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
	"github.com/goware/pp"
)

var DefaultOptions = Options{
	Logger:                   log.New(os.Stdout, "", 0), // TODO: use log adapter like from goware/breaker , ..
	PollingInterval:          1000 * time.Millisecond,
	Timeout:                  120 * time.Second,
	StartBlockNumber:         nil, // latest
	TrailNumBlocksBehindHead: 0,   // latest
	BlockRetentionLimit:      100,
	WithLogs:                 false,
	LogTopics:                []common.Hash{}, // all logs
	DebugLogging:             false,
	StrictSubscribers:        true,
}

type Options struct {
	// ..
	Logger util.Logger // TODO: replace this pattern with one from goware/breaker

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
	// consume the message from its channel. Default is false, which means subscribers
	// will get always the latest information even if another is lagging to consume.
	StrictSubscribers bool
}

type Monitor struct {
	options Options

	log      util.Logger
	provider *ethrpc.Provider

	chain           *Chain
	nextBlockNumber *big.Int
	// sentBlockNumber *big.Int

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

	return &Monitor{
		options:      options,
		log:          options.Logger,
		provider:     provider,
		chain:        newChain(options.BlockRetentionLimit),
		publishCh:    make(chan Blocks),
		publishQueue: newQueue(options.BlockRetentionLimit),
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

	// Broadcast published events to all subscribers
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case blocks := <-m.publishCh:
				// fmt.Println("==> publishing block", blocks.LatestBlock().NumberU64(), "total events:", len(blocks))

				// broadcast to subscribers
				m.broadcast(blocks)
			}
		}
	}()

	return m.monitor()
}

func (m *Monitor) Stop() {
	m.debugLogf("ethmonitor: stop")
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
	events := Blocks{}

	// TODO: all fine and well, but what happens if we get re-org then another re-org
	// in between.. will we be fine..? or will event-source be a mess..? we might need
	// to de-dupe "events" data before publishing, or give more time between reorgs?

	for {
		select {

		case <-m.ctx.Done():
			return nil

		case <-time.After(m.options.PollingInterval):

			// Max time for any iteration to execute in
			ctx, _ := context.WithTimeout(m.ctx, m.options.Timeout)

			headBlock := m.chain.Head()
			if headBlock != nil {
				m.nextBlockNumber = big.NewInt(0).Add(headBlock.Number(), big.NewInt(1))
			}

			nextBlock, err := m.provider.BlockByNumber(ctx, m.nextBlockNumber)
			if err == ethereum.NotFound {
				continue
			}
			if err != nil {
				// TODO: use warn level
				// TODO: lets report depending on the kind of error.. if its a "not found", we can skip reporting it
				// but if its an http status code != 2xx, then report it.. or connection failure, etc.
				m.log.Printf("ethmonitor: [retrying] failed to fetch next block # %d, due to: %v", m.nextBlockNumber, err)
				continue
			}

			events, err = m.buildCanonicalChain(ctx, nextBlock, events)
			if err != nil {
				if err == ethereum.NotFound {
					continue // the block number must not be mined
				}
				m.debugLogf("ethmonitor: error reported '%v', failed to build chain for next blockNum:%d blockHash:%s, retrying..",
					err, nextBlock.NumberU64(), nextBlock.Hash().Hex())

				// pause, then retry
				time.Sleep(m.options.PollingInterval)
				continue
			}

			if m.options.WithLogs {
				err := m.addLogs(ctx, events)
				if err != nil {
					// if !errors.Is(err, ErrMaxAttempts) {
					// 	// log any errors which are not max-attempt related
					// 	// TODO: use warn log level
					// 	m.debugLogf("ethmonitor: error reported '%v', failed to add logs for next blockNum:%d blockHash:%s, retrying..",
					// 		err, nextBlock.NumberU64(), nextBlock.Hash().Hex())
					// }
					// continue to retry
					continue
				}

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
				// an error means that the queue is full.. so, we should exit the monitor
				// and return.. we need error code like ErrFatal or something..
				panic(err) // TODO
			}

			// clear events sink
			events = Blocks{}
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

	m.debugLogf("ethmonitor: block reorg, reverting block #%d hash:%s", poppedBlock.NumberU64(), poppedBlock.Hash().Hex())
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

func (m *Monitor) addLogs(ctx context.Context, blocks Blocks) error {
	for _, block := range blocks {
		if block.OK {
			// skip, we already have logs for this block or its a removed block
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// do not attempt to get logs for re-org'd blocks as the data
		// will be inconsistent and may never be available.

		// TODO: however, if we want, we could check our ".Chain()" retention and copy over logs of removed
		// so we can give complete payload on publish..?
		// ^ yes.. lets do this, its more accurate.
		if block.Event == Removed {
			// TODO.. need to get logs we previously said to add and include here.. from chain retention..
			// and logs warn if we couldnt find it.. cuz that woudl be bad / weird for our event source.
			// block.OK = true // ... review, etc.....
			block.OK = true
			panic("TODO..")
			continue
		}

		blockHash := block.Hash()

		topics := [][]common.Hash{}
		if len(m.options.LogTopics) > 0 {
			topics = append(topics, m.options.LogTopics)
		}

		// NOTE: re-attempt and backfill loops means for reorged blocks
		// we're doing a lot of extra attempts for not a lot of good reason.

		// We'll retry a short number of times in case the node is syncing, but, we don't want to
		// try for too long because in case of a reorg, the logs will never be available.
		attempts := 0
		maxAttempts := 0

		for {
			logs, err := m.provider.FilterLogs(ctx, ethereum.FilterQuery{
				BlockHash: &blockHash,
				Topics:    topics,
			})
			if err == nil {
				// success
				if logs != nil {
					block.Logs = logs
				}
				block.OK = true
				break // we're done with this block

			} else {

				// mark for backfilling
				block.Logs = nil
				block.OK = false

				// context timed-out, lets stop
				if errors.Is(err, context.DeadlineExceeded) {
					// TODO: use warn level
					m.debugLogf("ethmonitor: timed-out fetching logs for blockHash %s [marked for log backfilling]", blockHash.Hex())
					return err
				}

				// re-attempt quickly just in case the node was still syncing
				attempts++
				if attempts >= maxAttempts {
					// max tries has been reached
					m.log.Printf("ethmonitor: getLogs failed for blockHash %s - '%v' [marked for log backfilling]", blockHash.Hex(), err)
					// m.debugLogf("ethmonitor: error reported '%v' while fetching logs for blockHash %s (max attempts reached)", err, blockHash.Hex())
					// return fmt.Errorf("failed to fetch logs for blockHash %s: %w", blockHash.Hex(), ErrMaxAttempts)
					break
				}

				// pause and then retry
				m.debugLogf("ethmonitor: error (%v) fetching logs for blockHash %s (attempt %d), retrying..", err, blockHash.Hex(), attempts)
				d := time.Duration(int64(m.options.PollingInterval) * int64(attempts))
				fmt.Println("sleeping for", d)
				time.Sleep(d)
			}
		}
	}
	return nil
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
				m.log.Printf("ethmonitor: [getLogs backfill successful for block:%d %s]", blocks[i].NumberU64(), blocks[i].Hash().Hex())
			}
		}
	}
}

func (m *Monitor) publish(ctx context.Context, events Blocks) error {
	// Check for trail-behind-head mode and set maxBlockNum if applicable
	maxBlockNum := uint64(0)
	if m.options.TrailNumBlocksBehindHead > 0 {
		maxBlockNum = m.LatestBlock().NumberU64() - uint64(m.options.TrailNumBlocksBehindHead)
	}

	// Publish events existing in the queue
	for _, events := range m.publishQueue.dequeue(maxBlockNum) {
		for _, ev := range events {
			pp.Yellow("dequeued %d", ev.Block.NumberU64()).Println()
		}
		m.publishCh <- events
	}

	// Check if new events are in ready state, if even a single event is not ready
	// then we enqueue the entire group
	ready := true // init ready flag, assuming all is okay

	// When trailing behind mode is set, lets always enqueue
	if maxBlockNum > 0 {
		// TODO: we can also clear out any reorged publish events here too..
		ready = false
	}

	// Ensure all events are in ready-OK state
	if ready {
		for _, ev := range events {
			if !ev.OK {
				ready = false
				break
			}
		}
	}

	// Publish now, or enqueue
	if ready {
		m.publishCh <- events
		return nil
	} else {
		err := m.publishQueue.enqueue(events)
		if err != nil {
			return err
		}
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
				m.log.Print("warning! a subscriber is falling behind (delayed for 4 seconds), as a result the monitor is being held back")
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

// TODO: update this to, logDebugf
// and lets add new, logWarnf and maybe logErrorf
// we can use logadapter pattern.. maybe include a log-level in the monitor config here..
func (m *Monitor) debugLogf(format string, v ...interface{}) {
	if !m.options.DebugLogging {
		return
	}
	m.log.Printf(format, v...)
}
