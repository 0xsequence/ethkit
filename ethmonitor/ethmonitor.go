package ethmonitor

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/horizon-games/ethkit/ethrpc"
	"github.com/pkg/errors"
)

/*

TODO

1. pub/sub notify

	a. new block number / hash
	b. block finalized
	c. txn finalized
	d. txn added
	e. txn removed

2. chain/scanner methods, get a Transaction .. we can fetch from cache, or from network..?

*/

var DefaultOptions = Options{
	BlockPollTime:           1 * time.Second,
	NumBlocksForTxnFinality: 12,
}

type Monitor struct {
	options Options

	ctx     context.Context
	ctxStop context.CancelFunc

	provider    *ethrpc.JSONRPC
	chain       *Chain
	subscribers []chan<- uint64

	started bool
	mu      sync.RWMutex
}

type Options struct {
	BlockPollTime           time.Duration
	NumBlocksForTxnFinality int
}

func NewMonitor(provider *ethrpc.JSONRPC, opts ...Options) (*Monitor, error) {
	options := DefaultOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	return &Monitor{
		options:     options,
		provider:    provider,
		chain:       newChain(),
		subscribers: make([]chan<- uint64, 0),
	}, nil
}

func (w *Monitor) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return errors.Errorf("already started")
	}
	w.started = true
	w.mu.Unlock()

	w.ctx, w.ctxStop = context.WithCancel(ctx)

	go w.poll()

	return nil
}

func (w *Monitor) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.ctxStop()
	w.started = false
	return nil
}

func (w *Monitor) poll() {
	ticker := time.NewTicker(w.options.BlockPollTime)
	defer ticker.Stop()

	var nextBlockNumber *big.Int

	for {
		select {
		case <-ticker.C:
			fmt.Println("tick")

			if w.ctx.Err() == context.Canceled {
				return
			}

			// Max time for any tick to execute in
			ctx, cancelFunc := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancelFunc()

			headBlock := w.chain.Head()
			if headBlock == nil {
				nextBlockNumber = nil // starting block.. (latest)
			} else {
				nextBlockNumber = big.NewInt(0).Add(headBlock.Number(), big.NewInt(1))
			}

			nextBlock, err := w.provider.BlockByNumber(ctx, nextBlockNumber)
			if err == ethereum.NotFound {
				continue
			}
			if err != nil {
				log.Fatal(err) // TODO: how to handle..? depends on error..
			}

			fmt.Println("ethrpc returns block number:", nextBlock.NumberU64())

			err = w.buildCanonicalChain(ctx, nextBlock)
			if err != nil {
				if err == ethereum.NotFound {
					// TODO: log
					continue // lets retry this
				}
				// TODO
				panic(errors.Errorf("canon err: %v", err))
			}

			fmt.Println("len..", len(w.chain.blocks))
			w.chain.PrintAllBlocks()
			fmt.Println("")

			// notify all subscribers..
			for _, sub := range w.subscribers {
				// TODO: hmm, we should never push to a closed channel, or it will panic.
				// what happens if the subscriber goroutine just goes away?

				// do not block
				select {
				case sub <- nextBlock.NumberU64():
				default:
				}
			}

		case <-w.ctx.Done():
			// monitor has stopped
			return
		}
	}
}

func (w *Monitor) buildCanonicalChain(ctx context.Context, nextBlock *types.Block) error {
	headBlock := w.chain.Head()
	if headBlock == nil || nextBlock.ParentHash() == headBlock.Hash() {
		return w.chain.push(nextBlock)
	}

	// remove it, not the right block
	w.chain.pop()

	nextParentBlock, err := w.provider.BlockByHash(ctx, nextBlock.ParentHash())
	if err != nil {
		return err
	}

	err = w.buildCanonicalChain(ctx, nextParentBlock)
	if err != nil {
		return err
	}

	return w.chain.push(nextBlock)
}

func (w *Monitor) Chain() *Chain {
	blocks := make([]*types.Block, len(w.chain.blocks))
	copy(blocks, w.chain.blocks)
	return &Chain{
		blocks: blocks,
	}
}

// channel subscribe to a topic, aka, to a specific kind of event
// * new block number, provide hash, parent hash, ...?
// * txn confirmed..?

// TODO: chan<- *types.Block
func (w *Monitor) Subscribe(sub chan<- uint64) func() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.subscribers = append(w.subscribers, sub)

	unsubscribe := func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for i, s := range w.subscribers {
			if s == sub {
				w.subscribers = append(w.subscribers[:i], w.subscribers[i+1:]...)
				return
			}
		}
	}

	return unsubscribe
}
