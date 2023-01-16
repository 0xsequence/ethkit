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
	"github.com/0xsequence/ethkit/ethtxn"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/goware/breaker"
	"github.com/goware/cachestore"
	"github.com/goware/cachestore/memlru"
	"github.com/goware/channel"
	"github.com/goware/logger"
)

var DefaultOptions = Options{
	MaxConcurrentFetchReceiptWorkers: 50,
	MaxConcurrentFilterWorkers:       20,
	PastReceiptsCacheSize:            5_000,
	NumBlocksToFinality:              0, // value of <=0 here will select from ethrpc.Networks[chainID].NumBlocksToFinality
	FilterMaxWaitNumBlocks:           0, // value of 0 here means no limit, and will listen until manually unsubscribed
	// DefaultFetchTransactionReceiptTimeout: 300 * time.Second,
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

	// ..
	// NOTE: not in use -- probably should delete this.
	// DefaultFetchTransactionReceiptTimeout time.Duration

	// Cache backend ...
	// CacheBackend cachestore.Backend
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

	// TODO: use opts.CacheBackend if set..
	// but, could be a lot for redis.. so, make sure to use Compose if we do it..
	pastReceipts, err := memlru.NewWithSize[*types.Receipt](opts.PastReceiptsCacheSize)
	if err != nil {
		return nil, err
	}

	// TODO: use opts.CacheBackend if set.. maybe combine with cachestore.Compose and memlru..?
	notFoundTxnHashes, err := memlru.NewWithSize[uint64](5000) //, cachestore.WithDefaultKeyExpiry(2*time.Minute))
	if err != nil {
		return nil, err
	}

	if opts.NumBlocksToFinality <= 0 {
		chainID, err := getChainID(provider)
		if err != nil {
			chainID = big.NewInt(1) // assume mainnet in case of unlikely error
		}
		network, ok := ethrpc.Networks[chainID.Uint64()]
		if ok {
			opts.NumBlocksToFinality = network.NumBlocksToFinality
		}
	}
	if opts.NumBlocksToFinality <= 0 {
		opts.NumBlocksToFinality = 1 // absolute min is 1
	}

	return &ReceiptListener{
		options:           opts,
		log:               log,
		provider:          provider,
		monitor:           monitor,
		br:                breaker.New(log, 1*time.Second, 2, 10),
		fetchSem:          make(chan struct{}, opts.MaxConcurrentFetchReceiptWorkers),
		pastReceipts:      pastReceipts,
		notFoundTxnHashes: notFoundTxnHashes,
		subscribers:       make([]*subscriber, 0),
		registerFiltersCh: make(chan registerFilters, 100),
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

	l.log.Info("ethreceipts: running")

	return l.listener()
}

func (l *ReceiptListener) Stop() {
	l.log.Info("ethreceipts: stop")
	l.ctxStop()
}

func (l *ReceiptListener) IsRunning() bool {
	return atomic.LoadInt32(&l.running) == 1
}

func (l *ReceiptListener) Subscribe(filterQueries ...FilterQuery) Subscription {
	l.mu.Lock()
	defer l.mu.Unlock()

	subscriber := &subscriber{
		listener: l,
		ch:       channel.NewUnboundedChan[Receipt](l.log, 100, 5000),
		done:     make(chan struct{}),
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

func (l *ReceiptListener) PurgeHistory() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pastReceipts.ClearAll(context.Background())
	l.notFoundTxnHashes.ClearAll(context.Background())
}

type WaitReceiptFinalityFunc func(ctx context.Context) (*Receipt, error)

func (l *ReceiptListener) FetchTransactionReceipt(ctx context.Context, txnHash common.Hash, optMaxBlockWait ...int) (*Receipt, WaitReceiptFinalityFunc, error) {
	maxWait := -1 // default use -1 maxWait, which is finality*2 value
	if len(optMaxBlockWait) > 0 {
		maxWait = optMaxBlockWait[0]
	}
	filter := FilterTxnHash(txnHash).MaxWait(maxWait)
	return l.FetchTransactionReceiptWithFilter(ctx, filter)
}

func (l *ReceiptListener) FetchTransactionReceiptWithFilter(ctx context.Context, filter FilterQuery) (*Receipt, WaitReceiptFinalityFunc, error) {
	// Fetch method searches for just a single filter match. If you'd like to keep the filter
	// open to listen to many similar receipts, use .Subscribe(filter) directly instead.
	query := filter.LimitOne(true).SearchCache(true).SearchOnChain(true).Finalize(true)

	filterer, ok := query.(Filterer)
	if !ok {
		return nil, nil, fmt.Errorf("ethreceipts: unable to cast Filterer from FilterQuery")
	}

	sub := l.Subscribe(query)

	exhausted := make(chan struct{})
	mined := make(chan Receipt)
	finalized := make(chan Receipt, 1)

	finalityFunc := func(ctx context.Context) (*Receipt, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-exhausted:
			return nil, ErrFilterExhausted
		case receipt, ok := <-finalized:
			if !ok {
				return nil, ErrFilterExhausted
			}
			return &receipt, nil
		}
	}

	go func() {
		defer sub.Unsubscribe()
		defer close(mined)
		defer close(finalized)

		for {
			select {
			case <-ctx.Done():
				return
			case <-filterer.Exhausted():
				close(exhausted)
				return
			case receipt, ok := <-sub.TransactionReceipt():
				if !ok {
					return
				}
				if receipt.Final {
					// non-blocking write to mined chan
					select {
					case mined <- receipt:
					default:
					}
					// write to finalized chan and return -- were done
					finalized <- receipt
					return
				} else {
					if receipt.Reorged {
						// skip reporting reoreged receipts in this method
						continue
					}
					// non-blocking write to mined chan
					select {
					case mined <- receipt:
					default:
					}
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-sub.Done():
		return nil, nil, ErrSubscriptionClosed
	case <-exhausted:
		return nil, finalityFunc, ErrFilterExhausted
	case receipt, ok := <-mined:
		if !ok {
			return nil, nil, ErrSubscriptionClosed
		}
		return &receipt, finalityFunc, nil
	}
}

func (l *ReceiptListener) fetchTransactionReceipt(ctx context.Context, txnHash common.Hash, forceFetch bool) (*types.Receipt, error) {
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
		notFoundBlockNum, notFound, _ := l.notFoundTxnHashes.Get(ctx, txnHashHex)
		if notFound && notFoundBlockNum >= oldestBlockNum {
			txn := l.monitor.GetTransaction(txnHash)
			if txn != nil {
				l.log.Debugf("fetchTransactionReceipt(%s) previously not found receipt has now been found in our monitor retention cache", txnHashHex)
				l.notFoundTxnHashes.Delete(ctx, txnHashHex)
				notFound = false
			}
		}
		if notFound {
			errCh <- ethereum.NotFound
			return
		}

		// Fetch the transaction receipt from the node, and use the breaker in case of node failures.
		err := l.br.Do(ctx, func() error {
			receipt, err := l.provider.TransactionReceipt(ctx, txnHash)
			if !forceFetch && err == ethereum.NotFound {
				// record the blockNum, maybe this receipt is just too new and nodes are telling
				// us they can't find it yet, in which case we will rely on the monitor to
				// clear this flag for us.
				l.log.Debugf("fetchTransactionReceipt(%s) receipt not found -- flagging in notFoundTxnHashes cache", txnHashHex)
				l.notFoundTxnHashes.Set(ctx, txnHashHex, latestBlockNum)
				errCh <- err
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

	latestBlockNum := l.latestBlockNum().Uint64()
	l.log.Debugf("latestBlockNum %d", latestBlockNum)

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
			blocks := l.monitor.Chain().Blocks()

			// Search our local blocks cache from monitor retention list
			matched, err := l.processBlocks(blocks, []*subscriber{reg.subscriber}, [][]Filterer{filters})
			if err != nil {
				l.log.Warnf("ethreceipts: failed to process blocks during new filter registration: %v", err)
			}

			// Finally, search on chain with filters which have had no results. Note, this strategy only
			// works for txnHash conditions as other filters could have multiple matches.
			err = l.searchFilterOnChain(context.Background(), reg.subscriber, collectOk(filters, matched[0], false))
			if err != nil {
				l.log.Warnf("ethreceipts: failed to search filter on-chain during new filter registration: %v", err)
			}

		// monitor newly mined blocks
		case blocks := <-monitor.Blocks():
			if len(blocks) == 0 {
				continue
			}

			latestBlockNum = l.latestBlockNum().Uint64()

			// delete past receipts of removed blocks
			for _, block := range blocks {
				if block.Event == ethmonitor.Removed {
					for _, txn := range block.Transactions() {
						txnHashHex := txn.Hash().Hex()
						l.pastReceipts.Delete(l.ctx, txnHashHex)
						l.notFoundTxnHashes.Delete(l.ctx, txnHashHex)
					}
				}
			}

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

			// Match blocks against subscribers[i] X filters[i][..]
			matched, err := l.processBlocks(blocks, subscribers, filters)
			if err != nil {
				l.log.Warnf("ethreceipts: failed to process blocks: %v", err)
			}

			// MaxWait exhaust check
			for x, list := range matched {
				for y, ok := range list {
					filterer := filters[x][y]
					if ok || filterer.LastMatchBlockNum() == 0 {
						if f, _ := filterer.(*filter); f != nil {
							f.lastMatchBlockNum = latestBlockNum
						}
					} else {
						// NOTE: even if a filter is exhausted, the finalizer will still run
						// for those transactions which were previously mined and marked by the finalizer.
						// Therefore, the code below will not impact the functionality of the finalizer.
						maxWait := l.getMaxWaitBlocks(filterer.Options().MaxWait)
						if maxWait != 0 && (latestBlockNum-filterer.LastMatchBlockNum()) >= maxWait {
							l.log.Debugf("filter exhausted! last block matched:%d maxWait:%d filterID:%d", filterer.LastMatchBlockNum(), maxWait, filterer.FilterID())

							subscriber := subscribers[x]
							subscriber.RemoveFilter(filterer)

							if f, _ := filterer.(*filter); f != nil {
								select {
								case <-f.Exhausted():
								default:
									close(f.exhausted)
								}
							} else {
								panic("ethreceipts: unexpected")
							}
						}
					}
				}
			}
		}
	}
}

// processBlocks attempts to match blocks against subscriber[i] X filterers[i].. list of filters. There is
// a corresponding list of filters[i] for each subscriber[i].
func (l *ReceiptListener) processBlocks(blocks ethmonitor.Blocks, subscribers []*subscriber, filterers [][]Filterer) ([][]bool, error) {
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
		logs := groupLogsByTransaction(block.Logs, len(block.Transactions()))

		for i, txn := range block.Transactions() {
			receipts[i] = Receipt{
				Reorged:     reorged,
				Final:       l.isBlockFinal(block.Number()),
				logs:        logs[i],
				transaction: txn,
			}
			txnMsg, err := ethtxn.AsMessage(txn)
			if err != nil {
				// NOTE: this should never happen, but lets log in case it does. In the
				// future, we should just not use go-ethereum for these types.
				l.log.Warnf("unexpected failure of txn (%s index %d) on block %d (total txns=%d) AsMessage(..): %s",
					txn.Hash(), i, block.NumberU64(), len(block.Transactions()), err,
				)
			} else {
				receipts[i].message = &txnMsg
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
					l.log.Warnf("error while processing filters: %s", err)
				}
				oks[i] = matched

				// check subscriber to finalize any receipts
				err = sub.finalizeReceipts(block.Number())
				if err != nil {
					l.log.Errorf("finalizeReceipts failed: %v", err)
				}
			}(i, sub)
		}
		wg.Wait()
	}

	return oks, nil
}

func (l *ReceiptListener) searchFilterOnChain(ctx context.Context, subscriber *subscriber, filterers []Filterer) error {
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
			l.log.Errorf("searchFilterOnChain fetchTransactionReceipt failed: %v", err)
		}
		if r == nil {
			continue
		}

		receipt := Receipt{
			receipt: r,
			// NOTE: we do not include the transaction at this point, as we don't have it.
			// transaction: txn,
			Final: l.isBlockFinal(r.BlockNumber),
		}

		_, err = subscriber.matchFilters(ctx, []Filterer{filterer}, []Receipt{receipt})
		if err != nil {
			l.log.Errorf("searchFilterOnChain matchFilters failed: %v", err)
		}
	}

	return nil
}

func (l *ReceiptListener) getMaxWaitBlocks(maxWait *int) uint64 {
	if maxWait == nil {
		return uint64(l.options.FilterMaxWaitNumBlocks)
	} else if *maxWait < 0 {
		return uint64(l.options.NumBlocksToFinality * 2)
	} else {
		return uint64(*maxWait)
	}
}

func (l *ReceiptListener) isBlockFinal(blockNum *big.Int) bool {
	diff := big.NewInt(0).Sub(l.latestBlockNum(), blockNum)
	return diff.Cmp(big.NewInt(int64(l.options.NumBlocksToFinality))) >= 0
}

func (l *ReceiptListener) latestBlockNum() *big.Int {
	latestBlockNum := l.monitor.LatestBlockNum()
	if latestBlockNum == nil {
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

func getChainID(provider *ethrpc.Provider) (*big.Int, error) {
	var chainID *big.Int
	err := breaker.Do(context.Background(), func() error {
		id, err := provider.ChainID(context.Background())
		if err != nil {
			return err
		}
		chainID = id
		return nil
	}, nil, 1*time.Second, 2, 20)
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

func groupLogsByTransaction(logs []types.Log, numTxns int) [][]*types.Log {
	out := make([][]*types.Log, numTxns)
	for _, log := range logs {
		log := log
		out[log.TxIndex] = append(out[log.TxIndex], &log)
	}
	for i, logs := range out {
		if logs == nil {
			out[i] = []*types.Log{}
		}
	}
	return out
}
