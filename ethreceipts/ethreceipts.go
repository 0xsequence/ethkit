package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/0xsequence/ethkit/util"
	"github.com/goware/breaker"
	memcache "github.com/goware/cachestore-mem"
	cachestore "github.com/goware/cachestore2"
	"github.com/goware/calc"
	"github.com/goware/channel"
	"github.com/goware/superr"
	"golang.org/x/sync/errgroup"
)

var DefaultOptions = Options{
	MaxConcurrentFetchReceiptWorkers: 100,
	MaxConcurrentFilterWorkers:       50,
	PastReceiptsCacheSize:            5_000,
	NumBlocksToFinality:              0, // value of <=0 here will select from ethrpc.Networks[chainID].NumBlocksToFinality
	FilterMaxWaitNumBlocks:           0, // value of 0 here means no limit, and will listen until manually unsubscribed
	Alerter:                          util.NoopAlerter(),
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

	// FilterMaxWaitNumBlocks is the maximum amount of blocks a filter will wait between getting
	// a receipt filter match, before the filter will unsubscribe itself and stop listening.
	// This value may be overriden by setting FilterCond#MaxListenNumBlocks on per-filter basis.
	//
	// NOTE:
	// * value of -1 will use NumBlocksToFinality*2
	// * value of 0 will set no limit, so filter will always listen [default]
	// * value of N will set the N number of blocks without results before unsubscribing between iterations
	FilterMaxWaitNumBlocks int

	// Cache backend ...
	// CacheBackend cachestore.Backend

	// Alerter config via github.com/goware/alerter
	Alerter util.Alerter
}

type ReceiptsListener struct {
	options  Options
	log      *slog.Logger
	alert    util.Alerter
	provider ethrpc.Interface
	monitor  *ethmonitor.Monitor
	chainID  *big.Int
	br       *breaker.Breaker

	// fetchSem is used to limit amount of concurrenct fetch requests
	fetchSem chan struct{}

	// pastReceipts is a cache of past requested receipts
	pastReceipts cachestore.Store[*types.Receipt]

	// notFoundTxnHashes is a cache to flag txn hashes which are not found on the network
	// so that we can avoid having to ask to refetch. The monitor will pick up these txn hashes
	// for us if they end up turning up.
	notFoundTxnHashes cachestore.Store[uint64]

	// ...
	subscribers       []*subscriber
	registerFiltersCh chan registerFilters
	filterSem         chan struct{}

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
	mu      sync.RWMutex
}

var (
	ErrFilterMatch        = errors.New("ethreceipts: filter match fail")
	ErrFilterCond         = errors.New("ethreceipts: missing filter condition")
	ErrFilterExhausted    = errors.New("ethreceipts: filter exhausted after maxWait blocks")
	ErrSubscriptionClosed = errors.New("ethreceipts: subscription closed")
)

func NewReceiptsListener(log *slog.Logger, provider ethrpc.Interface, monitor *ethmonitor.Monitor, options ...Options) (*ReceiptsListener, error) {
	opts := DefaultOptions
	if len(options) > 0 {
		opts = options[0]
	}

	if opts.Alerter == nil {
		opts.Alerter = util.NoopAlerter()
	}

	if !monitor.Options().WithLogs {
		return nil, fmt.Errorf("ethreceipts: ReceiptsListener needs a monitor with WithLogs enabled to function")
	}

	minBlockRetentionLimit := 50
	if monitor.Options().BlockRetentionLimit < minBlockRetentionLimit {
		return nil, fmt.Errorf("ethreceipts: monitor options BlockRetentionLimit must be at least %d", minBlockRetentionLimit)
	}

	if opts.PastReceiptsCacheSize <= 0 {
		opts.PastReceiptsCacheSize = DefaultOptions.PastReceiptsCacheSize
	}

	pastReceipts, err := memcache.NewCacheWithSize[*types.Receipt](uint32(opts.PastReceiptsCacheSize))
	if err != nil {
		return nil, err
	}

	notFoundTxnHashes, err := memcache.NewCacheWithSize[uint64](uint32(5000)) //, cachestore.WithDefaultKeyExpiry(2*time.Minute))
	if err != nil {
		return nil, err
	}

	return &ReceiptsListener{
		options:           opts,
		log:               log,
		alert:             opts.Alerter,
		provider:          provider,
		monitor:           monitor,
		br:                breaker.New(log, 1*time.Second, 2, 4), // max 4 retries
		fetchSem:          make(chan struct{}, opts.MaxConcurrentFetchReceiptWorkers),
		pastReceipts:      pastReceipts,
		notFoundTxnHashes: notFoundTxnHashes,
		subscribers:       make([]*subscriber, 0),
		registerFiltersCh: make(chan registerFilters, 1000),
		filterSem:         make(chan struct{}, opts.MaxConcurrentFilterWorkers),
	}, nil
}

func (l *ReceiptsListener) lazyInit(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var err error
	l.chainID, err = getChainID(ctx, l.provider)
	if err != nil {
		l.chainID = big.NewInt(1) // assume mainnet in case of unlikely error
	}

	if l.options.NumBlocksToFinality <= 0 {
		network, ok := ethrpc.Networks[l.chainID.Uint64()]
		if ok {
			l.options.NumBlocksToFinality = network.NumBlocksToFinality
		} else {
			l.options.NumBlocksToFinality = ethrpc.DefaultNumBlocksToFinality
		}
	}

	if l.options.NumBlocksToFinality <= 0 {
		l.options.NumBlocksToFinality = ethrpc.DefaultNumBlocksToFinality
	}

	return nil
}

func (l *ReceiptsListener) Run(ctx context.Context) error {
	if l.IsRunning() {
		return fmt.Errorf("ethreceipts: already running")
	}

	l.ctx, l.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&l.running, 1)
	defer atomic.StoreInt32(&l.running, 0)

	if err := l.lazyInit(ctx); err != nil {
		return err
	}

	l.log.Info("ethreceipts: running")

	return l.listener()
}

func (l *ReceiptsListener) Stop() {
	l.log.Info("ethreceipts: stop")
	l.ctxStop()
}

func (l *ReceiptsListener) IsRunning() bool {
	return atomic.LoadInt32(&l.running) == 1
}

func (l *ReceiptsListener) Subscribe(filterQueries ...FilterQuery) Subscription {
	l.mu.Lock()
	defer l.mu.Unlock()

	subscriber := &subscriber{
		listener: l,
		ch: channel.NewUnboundedChan[Receipt](2, 5000, channel.Options{
			Logger:  l.log,
			Alerter: l.alert,
			Label:   "ethreceipts:subscriber",
		}),
		done: make(chan struct{}),
		finalizer: &finalizer{
			numBlocksToFinality: big.NewInt(int64(l.options.NumBlocksToFinality)),
			queue:               []finalTxn{},
			txns:                map[common.Hash]struct{}{},
		},
	}

	subscriber.unsubscribe = func() {
		close(subscriber.done)
		subscriber.ch.Close()
		subscriber.ch.Flush()

		l.mu.Lock()
		defer l.mu.Unlock()

		for i, sub := range l.subscribers {
			if sub == subscriber {
				l.subscribers = append(l.subscribers[:i], l.subscribers[i+1:]...)
				return
			}
		}
	}

	l.subscribers = append(l.subscribers, subscriber)

	// Subscribe to the filters
	subscriber.AddFilter(filterQueries...)

	return subscriber
}

func (l *ReceiptsListener) NumSubscribers() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.subscribers)
}

func (l *ReceiptsListener) PurgeHistory() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pastReceipts.ClearAll(context.Background())
	l.notFoundTxnHashes.ClearAll(context.Background())
}

type WaitReceiptFinalityFunc func(ctx context.Context) (*Receipt, error)

func (l *ReceiptsListener) FetchTransactionReceipt(ctx context.Context, txnHash common.Hash, optMaxBlockWait ...int) (*Receipt, WaitReceiptFinalityFunc, error) {
	maxWait := -1 // default use -1 maxWait, which is finality*2 value
	if len(optMaxBlockWait) > 0 {
		maxWait = optMaxBlockWait[0]
	}
	filter := FilterTxnHash(txnHash).MaxWait(maxWait)
	return l.FetchTransactionReceiptWithFilter(ctx, filter)
}

func (l *ReceiptsListener) FetchTransactionReceiptWithFilter(ctx context.Context, filter FilterQuery) (*Receipt, WaitReceiptFinalityFunc, error) {
	// Fetch method searches for just a single filter match. If you'd like to keep the filter
	// open to listen to many similar receipts, use .Subscribe(filter) directly instead.
	query := filter.LimitOne(true).SearchCache(true).SearchOnChain(true).Finalize(true)

	filterer, ok := query.(Filterer)
	if !ok {
		return nil, nil, fmt.Errorf("ethreceipts: unable to cast Filterer from FilterQuery")
	}

	condMaxWait := 0
	if filterer.Options().MaxWait != nil {
		condMaxWait = *filterer.Options().MaxWait
	}
	condTxnHash := ""
	if filterer.Cond().TxnHash != nil {
		condTxnHash = (*filterer.Cond().TxnHash).String()
	}

	sub := l.Subscribe(query)

	// Use a WaitGroup to ensure the goroutine cleans up before the function returns
	var wg sync.WaitGroup

	exhausted := make(chan struct{})
	mined := make(chan Receipt, 2)
	finalized := make(chan Receipt, 1)
	found := uint32(0)

	finalityFunc := func(ctx context.Context) (*Receipt, error) {
		// Wait for the goroutine to finish its cleanup before proceeding in finalityFunc,
		// ensuring Unsubscribe has been called if the goroutine exited.
		wg.Wait()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case receipt, ok := <-finalized:
			if !ok {
				// If finalized is closed, it means the goroutine exited without finalizing.
				// Check if it was due to exhaustion.
				select {
				case <-exhausted:
					return nil, superr.Wrap(ErrFilterExhausted, fmt.Errorf("txnHash=%s maxWait=%d", condTxnHash, condMaxWait))
				default:
					// Goroutine likely exited due to parent context cancellation or other reasons.
					return nil, ErrSubscriptionClosed
				}
			}
			return &receipt, nil
		}
	}

	// TODO/NOTE: perhaps in an extended node failure. could there be a scenario
	// where filterer.Exhausted is never hit? and this subscription never unsubscribes..?
	// don't think so, but we can double check.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer sub.Unsubscribe()
		defer close(mined)
		defer close(finalized)

		defer func() {
			if r := recover(); r != nil {
				l.log.Error(fmt.Sprintf("ethreceipts: panic in fetchTransactionReceipt: %v - stack: %s", r, string(debug.Stack())))
				l.alert.Alert(context.Background(), "ethreceipts: panic in fetchTransactionReceipt: %v", r)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return

			case <-sub.Done():
				// Subscription closed externally (less likely here, but good practice)
				return

			case <-filterer.Exhausted():
				// Exhausted, check if we ever found a match.
				if atomic.LoadUint32(&found) == 0 {
					// Never found a match, signal exhaustion and exit.
					close(exhausted)
					return
				}
				// Found a match previously, but now exhausted.
				// Allow loop to continue briefly to let finalizer potentially finish,
				// but the finalized channel will eventually be closed if no final receipt comes.
				// The finalityFunc will handle the exhausted state if needed.
				// We signal exhaustion mainly for the initial return value check.
				close(exhausted)

			case receipt, ok := <-sub.TransactionReceipt():
				if !ok {
					// Channel closed, likely due to Unsubscribe or listener stopping
					return
				}

				atomic.StoreUint32(&found, 1)

				if receipt.Final {
					// Send to mined (in case caller only waits for mined)
					// Use non-blocking send in case mined channel is full or unread
					select {
					case mined <- receipt:
					default:
					}

					// Send to finalized and exit
					finalized <- receipt
					return
				} else {
					if receipt.Reorged {
						// Skip reorged receipts in this fetch method
						continue
					}
					// Send to mined and continue waiting for finalization
					// Use non-blocking send
					select {
					case mined <- receipt:
					default:
					}
				}
			}
		}
	}()

	// Wait for the first mined receipt or an exit signal
	select {
	case <-ctx.Done():
		wg.Wait() // Ensure cleanup
		return nil, nil, ctx.Err()
	case <-sub.Done():
		wg.Wait() // Ensure cleanup
		return nil, nil, ErrSubscriptionClosed
	case <-exhausted:
		// Exhausted before finding *any* receipt.
		// finalityFunc will handle waiting and returning the exhaustion error.
		return nil, finalityFunc, superr.Wrap(ErrFilterExhausted, fmt.Errorf("txnHash=%s maxWait=%d", condTxnHash, condMaxWait))
	case receipt, ok := <-mined:
		if !ok {
			// Mined channel closed without sending, implies goroutine exited early.
			wg.Wait() // Ensure cleanup
			// Check if exhaustion occurred
			select {
			case <-exhausted:
				return nil, finalityFunc, superr.Wrap(ErrFilterExhausted, fmt.Errorf("txnHash=%s maxWait=%d", condTxnHash, condMaxWait))
			default:
				return nil, nil, ErrSubscriptionClosed
			}
		}
		// Got the first mined receipt. Return it and the finality func.
		// The finalityFunc will use wg.Wait() internally.
		return &receipt, finalityFunc, nil
	}
}

// fetchTransactionReceipt from the rpc provider, up to some amount of concurrency. When forceFetch is passed,
// it indicates that we have high conviction that the receipt should be available, as the monitor has found
// this transaction hash.
func (l *ReceiptsListener) fetchTransactionReceipt(ctx context.Context, txnHash common.Hash, forceFetch bool) (*types.Receipt, error) {
	l.fetchSem <- struct{}{}

	resultCh := make(chan *types.Receipt)
	errCh := make(chan error)

	defer close(resultCh)
	defer close(errCh)

	go func() {
		defer func() {
			<-l.fetchSem
		}()

		txnHashHex := txnHash.String()

		receipt, ok, _ := l.pastReceipts.Get(ctx, txnHashHex)
		if ok {
			resultCh <- receipt
			return
		}

		latestBlockNum := l.monitor.LatestBlockNum().Uint64()
		oldestBlockNum := l.monitor.OldestBlockNum().Uint64()

		// Clear out notFound flag if the monitor has identified the transaction hash
		if !forceFetch {
			notFoundBlockNum, notFound, _ := l.notFoundTxnHashes.Get(ctx, txnHashHex)
			if notFound && notFoundBlockNum >= oldestBlockNum {
				l.mu.Lock()
				txn, _ := l.monitor.GetTransaction(txnHash)
				l.mu.Unlock()
				if txn != nil {
					l.log.Debug(fmt.Sprintf("fetchTransactionReceipt(%s) previously not found receipt has now been found in our monitor retention cache", txnHashHex))
					l.notFoundTxnHashes.Delete(ctx, txnHashHex)
					notFound = false
				}
			}
			if notFound {
				errCh <- ethereum.NotFound
				return
			}
		}

		// Fetch the transaction receipt from the node, and use the breaker in case of node failures.
		err := l.br.Do(ctx, func() error {
			tctx, clearTimeout := context.WithTimeout(ctx, 4*time.Second)
			defer clearTimeout()

			receipt, err := l.provider.TransactionReceipt(tctx, txnHash)

			if !forceFetch && errors.Is(err, ethereum.NotFound) {
				// record the blockNum, maybe this receipt is just too new and nodes are telling
				// us they can't find it yet, in which case we will rely on the monitor to
				// clear this flag for us.
				l.log.Debug(fmt.Sprintf("fetchTransactionReceipt(%s) receipt not found -- flagging in notFoundTxnHashes cache", txnHashHex))
				l.notFoundTxnHashes.Set(ctx, txnHashHex, latestBlockNum)
				errCh <- err
				return nil
			} else if forceFetch && receipt == nil {
				// force fetch, lets retry a number of times as the node may end up finding the receipt.
				// txn has been found in the monitor with event added, but still haven't retrived the receipt.
				// this could be that we're too fast and node isn't returning the receipt yet.
				return fmt.Errorf("forceFetch enabled, but failed to fetch receipt %s", txnHash)
			}
			if err != nil {
				return superr.Wrap(fmt.Errorf("failed to fetch receipt %s", txnHash), err)
			}

			l.pastReceipts.Set(ctx, txnHashHex, receipt)
			l.notFoundTxnHashes.Delete(ctx, txnHashHex)

			resultCh <- receipt
			return nil
		})

		if err != nil {
			errCh <- err
		}
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

func (l *ReceiptsListener) listener() error {
	monitor := l.monitor.Subscribe("ethreceipts")
	defer monitor.Unsubscribe()

	latestBlockNum := l.latestBlockNum().Uint64()
	l.log.Debug(fmt.Sprintf("latestBlockNum %d", latestBlockNum))

	g, ctx := errgroup.WithContext(l.ctx)

	// Listen on filter registration to search cached and on-chain receipts
	g.Go(func() error {
		for {
			select {

			case <-ctx.Done():
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

				// check if filters asking to search cache / on-chain
				filters := make([]Filterer, 0, len(reg.filters))
				for _, f := range reg.filters {
					if f.Options().SearchCache || f.Options().SearchOnChain {
						filters = append(filters, f)
					}
				}
				if len(filters) == 0 {
					continue
				}

				// fetch blocks data from the monitor cache. aka the up to some number
				// of blocks which are retained by the monitor. the blocks are ordered
				// from oldest to newest order.
				l.mu.Lock()
				blocks := l.monitor.Chain().Blocks()
				l.mu.Unlock()

				// Search our local blocks cache from monitor retention list
				matchedList, err := l.processBlocks(blocks, []*subscriber{reg.subscriber}, [][]Filterer{filters})
				if err != nil {
					l.log.Warn(fmt.Sprintf("ethreceipts: failed to process blocks during new filter registration: %v", err))
				}

				// Finally, search on chain with filters which have had no results. Note, this strategy only
				// works for txnHash conditions as other filters could have multiple matches.
				err = l.searchFilterOnChain(ctx, reg.subscriber, collectOk(filters, matchedList[0], false))
				if err != nil {
					l.log.Warn(fmt.Sprintf("ethreceipts: failed to search filter on-chain during new filter registration: %v", err))
				}
			}
		}
	})

	// Monitor new blocks for filter matches
	g.Go(func() error {
		for {
			select {

			case <-ctx.Done():
				l.log.Debug("ethreceipts: parent signaled to cancel - receipt listener is quitting")
				return nil

			case <-monitor.Done():
				l.log.Info("ethreceipts: receipt listener is stopped because monitor signaled its stopping")
				return nil

			// monitor newly mined blocks
			case blocks := <-monitor.Blocks():
				if len(blocks) == 0 {
					continue
				}

				latestBlockNum = l.latestBlockNum().Uint64()

				// pass blocks across filters of subscribers
				l.mu.Lock()
				if len(l.subscribers) == 0 {
					l.mu.Unlock()
					continue
				}
				subscribers := make([]*subscriber, len(l.subscribers))
				copy(subscribers, l.subscribers)
				filters := make([][]Filterer, len(l.subscribers))
				for i := 0; i < len(subscribers); i++ {
					filters[i] = subscribers[i].Filters()
				}
				l.mu.Unlock()

				reorg := false
				for _, block := range blocks {
					if block.Event == ethmonitor.Added {
						// eagerly clear notFoundTxnHashes, just in case
						for _, txn := range block.Transactions() {
							l.notFoundTxnHashes.Delete(ctx, txn.Hash().Hex())
						}
					} else if block.Event == ethmonitor.Removed {
						// delete past receipts of removed blocks
						reorg = true
						for _, txn := range block.Transactions() {
							txnHashHex := txn.Hash().Hex()
							l.pastReceipts.Delete(ctx, txnHashHex)
							l.notFoundTxnHashes.Delete(ctx, txnHashHex)
						}
					}
				}

				// mark all filterers of lastMatchBlockNum to 0 in case of reorg
				if reorg {
					for _, list := range filters {
						for _, filterer := range list {
							if f, _ := filterer.(*filter); f != nil {
								f.startBlockNum = latestBlockNum
								f.lastMatchBlockNum = 0
							}
						}
					}
				}

				// Match blocks against subscribers[i] X filters[i][..]
				matchedList, err := l.processBlocks(blocks, subscribers, filters)
				if err != nil {
					l.log.Warn(fmt.Sprintf("ethreceipts: failed to process blocks: %v", err))
				}

				// MaxWait exhaust check
				for x, list := range matchedList {
					for y, matched := range list {
						filterer := filters[x][y]
						if matched || filterer.StartBlockNum() == 0 {
							if f, _ := filterer.(*filter); f != nil {
								if f.startBlockNum == 0 {
									f.startBlockNum = latestBlockNum
								}
								if matched {
									f.lastMatchBlockNum = latestBlockNum
								}
							}
						} else {
							// NOTE: even if a filter is exhausted, the finalizer will still run
							// for those transactions which were previously mined and marked by the finalizer.
							// Therefore, the code below will not impact the functionality of the finalizer.
							maxWait := l.getMaxWaitBlocks(filterer.Options().MaxWait)
							blockNum := calc.Max(filterer.StartBlockNum(), filterer.LastMatchBlockNum())

							if maxWait != 0 && (latestBlockNum-blockNum) >= maxWait {
								f, _ := filterer.(*filter)
								if f == nil {
									panic("ethreceipts: unexpected")
								}

								if (f.Options().LimitOne && f.LastMatchBlockNum() == 0) || !f.Options().LimitOne {
									l.log.Debug(fmt.Sprintf("filter exhausted! last block matched:%d maxWait:%d filterID:%d", filterer.LastMatchBlockNum(), maxWait, filterer.FilterID()))

									subscriber := subscribers[x]
									subscriber.RemoveFilter(filterer)

									select {
									case <-f.Exhausted():
									default:
										close(f.exhausted)
									}
								}
							}
						}
					}
				}
			}
		}
	})

	// TODO/NOTE: perhaps in an extended node failure. could there be a scenario
	// where filterer.Exhausted is never hit? and this subscription never unsubscribes..?
	// TODO: we ultimately need to check the monitor and if we get no new blocks for a period
	// of time, then we can assume node problems.. even more helpful woudl be if the monitor
	// gave us an error count of node failures, and we'd listen on that, and if we hit a threshold
	// and our block number doesn't change after a period of time, then we return an error
	// that we're exhausted due to a node failure.

	return g.Wait()
}

// processBlocks attempts to match blocks against subscriber[i] X filterers[i].. list of filters. There is
// a corresponding list of filters[i] for each subscriber[i].
func (l *ReceiptsListener) processBlocks(blocks ethmonitor.Blocks, subscribers []*subscriber, filterers [][]Filterer) ([][]bool, error) {
	// oks is the 'ok' match of the filterers [][]Filterer results
	oks := make([][]bool, len(filterers))
	for i, f := range filterers {
		oks[i] = make([]bool, len(f))
	}

	if len(subscribers) == 0 || len(filterers) == 0 {
		return oks, nil
	}

	// check each block against each subscriber X filter
	for _, block := range blocks {
		// report if the txn was removed
		reorged := block.Event == ethmonitor.Removed

		receipts := make([]Receipt, len(block.Transactions()))
		logs := groupLogsByTransaction(block.Logs)

		// build receipts for each txn which include the transaction and the logs
		for i, txn := range block.Transactions() {
			txnLog, ok := logs[txn.Hash().Hex()]
			if !ok {
				txnLog = []*types.Log{}
			}

			receipts[i] = Receipt{
				Reorged:     reorged,
				Final:       l.isBlockFinal(block.Number()),
				logs:        txnLog,
				transaction: txn,
			}

			// TODOXXX: avoid using AsMessage as its fairly expensive operation, especially
			// to do it for every txn for every filter.
			// TODO: in order to do this, we'll have to update ethrpc with a different
			// implementation to just use raw types, aka, ethrpc/types.go with Block/Transaction/Receipt/Log ..
			txnMsg, err := ethtxn.AsMessage(txn, l.chainID)
			if err != nil {
				// NOTE: this should never happen, but lets log in case it does. In the
				// future, we should just not use go-ethereum for these types.
				l.log.Warn(fmt.Sprintf("unexpected failure of txn (%s index %d) on block %d (total txns=%d) AsMessage(..): %s",
					txn.Hash(), i, block.NumberU64(), len(block.Transactions()), err,
				))
			} else {
				receipts[i].message = txnMsg
			}
		}

		// match the receipts against the filters
		var wg sync.WaitGroup
		for i, sub := range subscribers {
			wg.Add(1)
			l.filterSem <- struct{}{}
			go func(i int, sub *subscriber) {
				defer func() {
					<-l.filterSem
					wg.Done()
				}()

				// filter matcher
				matched, err := sub.matchFilters(l.ctx, filterers[i], receipts)
				if err != nil {
					l.log.Warn(fmt.Sprintf("error while processing filters: %s", err))
				}
				oks[i] = matched

				// check subscriber to finalize any receipts
				err = sub.finalizeReceipts(block.Number())
				if err != nil {
					l.log.Error(fmt.Sprintf("finalizeReceipts failed: %v", err))
				}
			}(i, sub)
		}
		wg.Wait()
	}

	return oks, nil
}

func (l *ReceiptsListener) searchFilterOnChain(ctx context.Context, subscriber *subscriber, filterers []Filterer) error {
	for _, filterer := range filterers {
		if !filterer.Options().SearchOnChain {
			// skip filters which do not ask to search on chain
			continue
		}

		txnHashCond := filterer.Cond().TxnHash
		if txnHashCond == nil {
			// skip filters which are not searching for txnHashes directly
			continue
		}

		r, err := l.fetchTransactionReceipt(ctx, *txnHashCond, false)
		if !errors.Is(err, ethereum.NotFound) && err != nil {
			l.log.Error(fmt.Sprintf("searchFilterOnChain fetchTransactionReceipt failed: %v", err))
		}
		if r == nil {
			// unable to find the receipt on-chain, lets continue
			continue
		}

		if f, ok := filterer.(*filter); ok {
			f.lastMatchBlockNum = r.BlockNumber.Uint64()
		}

		receipt := Receipt{
			receipt: r,
			// NOTE: we do not include the transaction at this point, as we don't have it.
			// transaction: txn,
			Final: l.isBlockFinal(r.BlockNumber),
		}

		// will always find the receipt, as it will be in our case previously found above.
		// this is called so we can broadcast the match to the filterer's subscriber.
		_, err = subscriber.matchFilters(ctx, []Filterer{filterer}, []Receipt{receipt})
		if err != nil {
			l.log.Error(fmt.Sprintf("searchFilterOnChain matchFilters failed: %v", err))
		}
	}

	return nil
}

func (l *ReceiptsListener) getMaxWaitBlocks(maxWait *int) uint64 {
	if maxWait == nil {
		return uint64(l.options.FilterMaxWaitNumBlocks)
	} else if *maxWait < 0 {
		l.mu.RLock()
		defer l.mu.RUnlock()

		return uint64(l.options.NumBlocksToFinality * 2)
	} else {
		return uint64(*maxWait)
	}
}

func (l *ReceiptsListener) isBlockFinal(blockNum *big.Int) bool {
	latestBlockNum := l.latestBlockNum()
	if latestBlockNum == nil || blockNum == nil {
		return false
	}
	diff := big.NewInt(0).Sub(latestBlockNum, blockNum)

	l.mu.RLock()
	defer l.mu.RUnlock()

	return diff.Cmp(big.NewInt(int64(l.options.NumBlocksToFinality))) >= 0
}

func (l *ReceiptsListener) latestBlockNum() *big.Int {
	latestBlockNum := l.monitor.LatestBlockNum()
	if latestBlockNum == nil || latestBlockNum.Cmp(big.NewInt(0)) == 0 {
		err := l.br.Do(l.ctx, func() error {
			block, err := l.provider.BlockByNumber(context.Background(), nil)
			if err != nil {
				return err
			}
			latestBlockNum = block.Number()
			return nil
		})
		if err != nil || latestBlockNum == nil {
			return big.NewInt(0)
		}
		return latestBlockNum
	}
	return latestBlockNum
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

func collectOk[T any](in []T, oks []bool, okCond bool) []T {
	var out []T
	for i, v := range in {
		if oks[i] == okCond {
			out = append(out, v)
		}
	}
	return out
}

// func txnLogs(blockLogs []types.Log, txnHash ethkit.Hash) []*types.Log {
// 	txnLogs := []*types.Log{}
// 	for i, log := range blockLogs {
// 		if log.TxHash == txnHash {
// 			log := log // copy
// 			txnLogs = append(txnLogs, &log)
// 			if i+1 >= len(blockLogs) || blockLogs[i+1].TxHash != txnHash {
// 				break
// 			}
// 		}
// 	}
// 	return txnLogs
// }

func groupLogsByTransaction(logs []types.Log) map[string][]*types.Log {
	var out = make(map[string][]*types.Log)
	for _, log := range logs {
		log := log

		logTxHash := log.TxHash.Hex()
		outLogs, ok := out[logTxHash]
		if !ok {
			outLogs = []*types.Log{}
		}

		outLogs = append(outLogs, &log)
		out[logTxHash] = outLogs
	}
	return out
}

func blockLogsCount(numTxns int, logs []types.Log) uint {
	var max uint = uint(numTxns)
	for _, log := range logs {
		if log.TxIndex+1 > max {
			max = log.TxIndex + 1
		}
	}
	return max
}
