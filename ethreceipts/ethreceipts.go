package ethreceipts

import (
	"context"
	"errors"
	"fmt"
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

const (
	maxConcurrentFetchReceiptWorkers = 50
	maxConcurrentFilterWorkers       = 20
	pastReceiptsCacheSize            = 10_000
	waitForTransactionReceiptTimeout = 300 * time.Second
)

type ReceiptListener struct {
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

	subscribers []*subscriber
	filterSem   chan struct{}

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
	Filter  Filter        // reference to filter
}

var (
	ErrBlah = errors.New("ethreceipts: x")
)

// TODO: kinda need a finalizer..
// we send it txn hashes, and it will send us final receipts once a txn has reached finality
// we can add .Enqueue() and also have .Subscribe() where we listen for a Receipt or just Txn
// --
// lets write it as a separate subpkg, as we will have to handle stuff like reorg notification, etc.
// and rechecking, or if a txn was reorged and not re-included ..

func NewReceiptListener(log logger.Logger, provider *ethrpc.Provider, monitor *ethmonitor.Monitor) (*ReceiptListener, error) {
	if !monitor.Options().WithLogs {
		return nil, fmt.Errorf("ethreceipts: ReceiptListener needs a monitor with WithLogs enabled to function")
	}

	pastReceipts, err := memlru.NewWithSize[*types.Receipt](pastReceiptsCacheSize)
	if err != nil {
		return nil, err
	}

	notFoundTxnHashes, err := memlru.NewWithSize[uint32](5000) //, cachestore.WithDefaultKeyExpiry(2*time.Minute))
	if err != nil {
		return nil, err
	}

	return &ReceiptListener{
		log:               log,
		provider:          provider,
		monitor:           monitor,
		br:                breaker.New(log, 1*time.Second, 2, 20),
		fetchSem:          make(chan struct{}, maxConcurrentFetchReceiptWorkers),
		pastReceipts:      pastReceipts,
		notFoundTxnHashes: notFoundTxnHashes,
		subscribers:       make([]*subscriber, 0),
		filterSem:         make(chan struct{}, maxConcurrentFilterWorkers),
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
		filters:  filters,
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

	return subscriber
}

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
			ctx, cancel = context.WithTimeout(ctx, waitForTransactionReceiptTimeout)
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
	if _, ok, _ := l.notFoundTxnHashes.Get(ctx, txnHashHex); !ok || txn != nil {
		// TODO: what if there is a node failure, etc... we should add separate method, fetchTransactionReceipt() ..
		// receipt, err = l.provider.TransactionReceipt(ctx, txnHash)
		// if err == ethereum.NotFound {
		// 	l.notFoundTxnHashes.Set(ctx, txnHashHex, 1)
		// }
		receipt, _ = l.fetchTransactionReceipt(ctx, txnHash)
	}

	if receipt != nil {
		return receipt, nil
	}

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
						// fetchTxnReceipt() ..
						// NOTE: it could say not found.., but monitor has it, so node might be slow..
						receipt, err := l.fetchTransactionReceipt(ctx, txnHash)
						if err != nil {
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
		// or not..?

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
	sub := l.monitor.Subscribe()
	defer sub.Unsubscribe()

	for {
		select {

		case <-l.ctx.Done():
			l.log.Debug("ethreceipts: parent signaled to cancel - receipt listener is quitting")
			return nil

		case <-sub.Done():
			l.log.Info("ethreceipts: receipt listener is stopped because monitor signaled its stopping")
			return nil

		case blocks := <-sub.Blocks():

			if len(l.subscribers) == 0 {
				continue
			}

			l.mu.Lock()
			subscribers := make([]*subscriber, len(l.subscribers))
			copy(subscribers, l.subscribers)
			l.mu.Unlock()

			// check each block against each subscriber X filter .. seems like a lot of iterations
			// i wonder if there is a faster way to do this..
			for _, block := range blocks {
				// report if the txn was removed
				removed := block.Event == ethmonitor.Removed

				// TODO: would be cool to do some boolean logic..
				// like from AND log event..
				//
				// we can do this if Filter() was a struct with methods
				// ie..
				/*
					type Filter struct {
						From *common.Address
						To *common.Address
						Log func(etc..)
					}

					then, add NewFilter.From(address).To(to)

					// then just a single Match() method..
				*/

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

				// TODO: maybe we allow to specify an arbitrary id on the filter
				// and it will return back? kinda useful to track those matches..
				// we should also offer a label option?

				// TODO: allow filter to pass FromBlock: XXX
				// and if unspecified, then just go from latest. We should allow
				// to specify -100 as well... so maybe like can pass the block number itself
				// or pass -100 to track from head.. for GetTxnReceipt(hash) we'll always use -10_000
				// which, would use the limit.. but that method will be a bit different too as we'll check the entire chain

				for _, sub := range subscribers {
					l.filterSem <- struct{}{}
					go func(sub *subscriber) {
						defer func() {
							<-l.filterSem
						}()

						err := sub.processFilters(l.ctx, receipts)
						if err != nil {
							l.log.Warnf("error while processing filters: %s", err)
						}
					}(sub)
				}
			}
		}
	}
}
