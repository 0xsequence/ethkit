package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
	"github.com/goware/breaker"
	"github.com/goware/cachestore"
	"github.com/goware/cachestore/memlru"
	"github.com/goware/logger"
)

var DefaultOptions = Options{
	MaxConcurrentFetchReceiptWorkers:      50,
	MaxConcurrentFilterWorkers:            20,
	PastReceiptsCacheSize:                 10_000,
	NumBlocksToFinality:                   -1, // value of -1 here wll select from ethrpc.Networks[chainID].NumBlocksToFinality
	MaxNumBlocksListen:                    -1, // value of -1 here will use NumBlocksToFinality * 2
	DefaultFetchTransactionReceiptTimeout: 300 * time.Second,
}

type Options struct {
	// ..
	MaxConcurrentFetchReceiptWorkers int

	// ..
	MaxConcurrentFilterWorkers int

	// ..
	PastReceiptsCacheSize int

	// ..
	NumBlocksToFinality int

	// MaxNumBlocksWait is the maximum amount of blocks a filter will wait between getting
	// a receipt filter match, before the filter will unsubscribe itself to stop listening.
	// This value may be overriden by setting FilterCond#MaxNumBlocksListen on per-filter basis.
	//
	// NOTE:
	// * value of -1 will use NumBlocksToFinality*2 [default]
	// * value of 0 will set no limit, so filter will always listen
	// * value of N will set the N number of blocks without results before unsubscribing between iterations
	MaxNumBlocksListen int

	// ..
	DefaultFetchTransactionReceiptTimeout time.Duration
}

type ReceiptListener struct {
	options  Options
	log      logger.Logger
	provider *ethrpc.Provider
	monitor  *ethmonitor.Monitor
	br       *breaker.Breaker

	// fetchSem is used to limit amount of concurrenct fetch requests
	fetchSem chan struct{}

	// pastReceipts is a cache of past requested receipts
	pastReceipts cachestore.Store[*types.Receipt]

	// notFoundTxnHashes is a cache to flag txn hashes which are not found on the network
	// so that we can avoid having to ask to refetch. The monitor will pick up these txn hashes
	// for us if they end up turning up.
	notFoundTxnHashes cachestore.Store[uint32]

	// ...
	subscribers       []*subscriber
	registerFiltersCh chan registerFilters
	filterSem         chan struct{}

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
	mu      sync.RWMutex
}

type Receipt struct {
	*types.Transaction
	*types.Receipt
	Message types.Message // TOOD: this is lame..
	Removed bool          // reorged txn
	Final   bool          // flags that this receipt is finalized
	Filter  Filter        // reference to filter which triggered this event
}

var (
	ErrBlah = errors.New("ethreceipts: x")
)

func NewReceiptListener(log logger.Logger, provider *ethrpc.Provider, monitor *ethmonitor.Monitor, options ...Options) (*ReceiptListener, error) {
	opts := DefaultOptions
	if len(options) > 0 {
		opts = options[0]
	}

	if !monitor.Options().WithLogs {
		return nil, fmt.Errorf("ethreceipts: ReceiptListener needs a monitor with WithLogs enabled to function")
	}

	minBlockRetentionLimit := 400
	if monitor.Options().BlockRetentionLimit < minBlockRetentionLimit {
		return nil, fmt.Errorf("ethreceipts: monitor options BlockRetentionLimit must be at least %d", minBlockRetentionLimit)
	}

	pastReceipts, err := memlru.NewWithSize[*types.Receipt](opts.PastReceiptsCacheSize)
	if err != nil {
		return nil, err
	}

	notFoundTxnHashes, err := memlru.NewWithSize[uint32](5000) //, cachestore.WithDefaultKeyExpiry(2*time.Minute))
	if err != nil {
		return nil, err
	}

	if opts.NumBlocksToFinality < 0 {
		// TODO: use breaker..?
		// issue is.. if this fails, then
		chainID, err := provider.ChainID(context.Background())
		if err != nil {
			// hmm... we do need the NumBlocksToFinality ..
			panic(err) // TODO ..
		}
		network, ok := ethrpc.Networks[chainID.Uint64()]
		if ok {
			opts.NumBlocksToFinality = network.NumBlocksToFinality
		}
	}

	return &ReceiptListener{
		options:           opts,
		log:               log,
		provider:          provider,
		monitor:           monitor,
		br:                breaker.New(log, 1*time.Second, 2, 20),
		fetchSem:          make(chan struct{}, opts.MaxConcurrentFetchReceiptWorkers),
		pastReceipts:      pastReceipts,
		notFoundTxnHashes: notFoundTxnHashes,
		subscribers:       make([]*subscriber, 0),
		registerFiltersCh: make(chan registerFilters),
		filterSem:         make(chan struct{}, opts.MaxConcurrentFilterWorkers),
	}, nil
}

func (l *ReceiptListener) Run(ctx context.Context) error {
	if l.IsRunning() {
		return fmt.Errorf("ethreceipts: already running")
	}

	l.ctx, l.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&l.running, 1)
	defer atomic.StoreInt32(&l.running, 0)

	return l.listener()
}

func (l *ReceiptListener) Stop() {
	l.log.Info("ethreceipts: stop")
	l.ctxStop()
}

func (l *ReceiptListener) IsRunning() bool {
	return atomic.LoadInt32(&l.running) == 1
}

func (l *ReceiptListener) Subscribe(filters ...Filter) Subscription {
	l.mu.Lock()
	defer l.mu.Unlock()

	ch := make(chan Receipt)
	subscriber := &subscriber{
		listener: l,
		ch:       ch,
		sendCh:   util.MakeUnboundedChan(ch, l.log, 100),
		done:     make(chan struct{}),
		finalizer: &finalizer{
			numBlocksToFinality: big.NewInt(int64(l.options.NumBlocksToFinality)),
			queue:               []finalTxn{},
			txns:                map[common.Hash]struct{}{},
		},
	}

	subscriber.unsubscribe = func() {
		close(subscriber.done)
		l.mu.Lock()
		defer l.mu.Unlock()
		close(subscriber.sendCh)

		// flush subscriber.ch so that the MakeUnboundedChan goroutine exits
		for ok := true; ok; _, ok = <-subscriber.ch {
		}

		for i, sub := range l.subscribers {
			if sub == subscriber {
				l.subscribers = append(l.subscribers[:i], l.subscribers[i+1:]...)
				return
			}
		}
	}

	l.subscribers = append(l.subscribers, subscriber)

	subscriber.Subscribe(filters...)

	return subscriber
}

// TODO: return Receipt .. and include Final bool ..
// prob also return waitReceipt()
func (l *ReceiptListener) FetchTransactionReceipt(ctx context.Context, txnHash common.Hash, optTimeout ...time.Duration) (*types.Receipt, error) {
	// l.mu.Lock()
	// defer l.mu.Unlock()

	// Use optional timeout if passed, otherwise use deadline on the provided ctx, or finally,
	// set a default timeout.
	var cancel context.CancelFunc
	if len(optTimeout) > 0 {
		ctx, cancel = context.WithTimeout(ctx, optTimeout[0])
		defer cancel()
	} else {
		if _, ok := ctx.Deadline(); !ok {
			ctx, cancel = context.WithTimeout(ctx, l.options.DefaultFetchTransactionReceiptTimeout)
			defer cancel()
		}
	}

	txnHashHex := txnHash.String()

	// First check pastReceipts if we already have it
	receipt, ok, _ := l.pastReceipts.Get(ctx, txnHashHex)
	if ok {
		return receipt, nil
	}

	// Listen for new txns early in case event comes through
	sub := l.monitor.Subscribe()
	defer sub.Unsubscribe()

	// 2. check monitor.Chain() for the txn hash.. does this retention list though include reorged txns..? and we'll check if its been added
	txn := l.monitor.GetTransaction(txnHash)

	// 3. lets fetch it from the provider, in case its old.. what if its too new, and not-found, and we keep asking? we need a cache for notFoundTxnHashes
	// since the listener started, so we dont ask again, as the monitor will pick it up for us..
	var err error
	if _, ok, _ := l.notFoundTxnHashes.Get(ctx, txnHashHex); !ok || txn != nil {
		receipt, err = l.fetchTransactionReceipt(ctx, txnHash)
		if errors.Is(err, breaker.ErrFatal) || errors.Is(err, breaker.ErrHitMaxRetries) {
			// happens most likely due to a node failure
			return nil, fmt.Errorf("ethreceipts: failed to fetch receipt %w", err)
		}
	}

	if receipt != nil {
		return receipt, nil
	}

	// TODO: maybe return waitFinalize method which will subscribe, return + unsubscribe for finality....

	// 4. listen for it on the monitor until the txn shows up, etc.
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-sub.Done():
			return nil, nil

		case blocks := <-sub.Blocks():
			for _, block := range blocks {
				for _, txn := range block.Transactions() {
					if txn.Hash() != txnHash {
						// next
						continue
					}

					switch block.Event {
					case ethmonitor.Added:
						receipt, err := l.fetchTransactionReceipt(ctx, txnHash)
						if err != nil {
							if errors.Is(err, breaker.ErrFatal) || errors.Is(err, breaker.ErrHitMaxRetries) {
								// happens most likely due to a node failure
								return nil, fmt.Errorf("ethreceipts: failed to fetch receipt %w", err)
							}
							return nil, err
						}
						return receipt, nil

					case ethmonitor.Removed:
						l.pastReceipts.Delete(ctx, txnHashHex)
						l.notFoundTxnHashes.Delete(ctx, txnHashHex)
					}
				}
			}
		}
	}
}

func (l *ReceiptListener) fetchTransactionReceipt(ctx context.Context, txnHash common.Hash) (*types.Receipt, error) {
	l.filterSem <- struct{}{}

	resultCh := make(chan *types.Receipt)
	errCh := make(chan error)

	go func() {
		defer func() {
			<-l.filterSem
		}()

		txnHashHex := txnHash.String()

		receipt, ok, _ := l.pastReceipts.Get(ctx, txnHashHex)
		if ok {
			resultCh <- receipt
			return
		}

		// TODO: check if we have it in the monitor.. then we can decide if we should wait longer, etc.
		// and maybe or maybe not it was removed..
		// or not......?

		err := l.br.Do(ctx, func() error {
			receipt, err := l.provider.TransactionReceipt(ctx, txnHash)
			if err == ethereum.NotFound {
				// TODO: review..
				l.notFoundTxnHashes.Set(ctx, txnHashHex, 1)
				return nil
			}
			if err != nil {
				return err
			}

			l.pastReceipts.Set(ctx, txnHashHex, receipt)
			l.notFoundTxnHashes.Delete(ctx, txnHashHex)

			resultCh <- receipt
			return nil
		})

		if err != nil {
			errCh <- err
		}
		resultCh <- nil
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case receipt := <-resultCh:
		return receipt, nil
	case err := <-errCh:
		return nil, err
	}
}

func (l *ReceiptListener) listener() error {
	monitor := l.monitor.Subscribe()
	defer monitor.Unsubscribe()

	// TODO: what about l.pastReceipts, and removing them as events are removed....?
	// or updating to state Removed: true ..?

	for {
		select {

		case <-l.ctx.Done():
			l.log.Debug("ethreceipts: parent signaled to cancel - receipt listener is quitting")
			return nil

		case <-monitor.Done():
			l.log.Info("ethreceipts: receipt listener is stopped because monitor signaled its stopping")
			return nil

		// subscriber registered a new filter, lets process past blocks against the new filters
		case reg, ok := <-l.registerFiltersCh:
			if !ok {
				continue
			}
			if len(reg.filters) == 0 {
				continue
			}

			// check if filters asking to search history
			filters := make([]Filter, 0, len(reg.filters))
			for _, f := range reg.filters {
				if f.Options().SearchHistory {
					filters = append(filters, f)
				}
			}
			if len(filters) == 0 {
				continue
			}

			// blocks is in oldest to newest order, which is fine
			blocks := l.monitor.Chain().Blocks()

			err := l.processBlocks(blocks, []*subscriber{reg.subscriber}, [][]Filter{filters})
			if err != nil {
				l.log.Warnf("ethreceipts: failed to process blocks on new filter registration: %v", err)
			}

		// monitor newly mined blocks
		case blocks := <-monitor.Blocks():
			if len(l.subscribers) == 0 {
				continue
			}

			l.mu.Lock()
			subscribers := make([]*subscriber, len(l.subscribers))
			copy(subscribers, l.subscribers)
			filters := make([][]Filter, len(l.subscribers))
			for i := 0; i < len(subscribers); i++ {
				filters[i] = subscribers[i].Filters()
			}
			l.mu.Unlock()

			err := l.processBlocks(blocks, subscribers, filters)
			if err != nil {
				l.log.Warnf("ethreceipts: failed to process blocks: %v", err)
			}
		}
	}
}

func (l *ReceiptListener) processBlocks(blocks ethmonitor.Blocks, subscribers []*subscriber, filters [][]Filter) error {
	if len(subscribers) == 0 || len(filters) == 0 {
		return nil
	}

	// check each block against each subscriber X filter
	for _, block := range blocks {
		// report if the txn was removed
		removed := block.Event == ethmonitor.Removed

		receipts := make([]Receipt, len(block.Transactions()))

		for i, txn := range block.Transactions() {
			txnMsg, err := txn.AsMessage(types.NewLondonSigner(txn.ChainId()), nil)
			if err != nil {
				// NOTE: this should never happen, but lets log in case it does. In the
				// future, we should just not use go-ethereum for these types.
				l.log.Warnf("unexpected failure of txn.AsMessage(..): %s", err)
				continue
			}
			receipts[i] = Receipt{Transaction: txn, Message: txnMsg, Removed: removed}
		}

		var wg sync.WaitGroup
		for i, sub := range subscribers {
			wg.Add(1)
			l.filterSem <- struct{}{}
			go func(i int, sub *subscriber) {
				defer func() {
					<-l.filterSem
					wg.Done()
				}()

				err := sub.matchFilters(l.ctx, filters[i], receipts)
				if err != nil {
					l.log.Warnf("error while processing filters: %s", err)
				}

				finalizer := sub.finalizer
				if finalizer.len() == 0 {
					return
				}

				finalTxns := finalizer.dequeue(block.Number())
				if len(finalTxns) == 0 {
					// no matching txns which have been finalized
					return
				}

				// dispatch to subscriber finalized receipts
				for _, x := range finalTxns {
					x.receipt.Final = true
					sub.sendCh <- x.receipt

					// Automatically remove filters for finalized txn hashes, as they won't come up again.
					f, ok := x.receipt.Filter.(*FilterCond)
					if ok && (f.TxnHash != nil || f.FilterOpts.LimitOne) {
						sub.RemoveFilter(f)
					}
				}
			}(i, sub)
		}
		wg.Wait()
	}

	return nil
}
