package ethreceipts

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/ethmonitor"
	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/rs/zerolog"
)

type Receipts struct {
	options Options

	log      zerolog.Logger
	provider *ethrpc.Provider

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
}

type Options struct {
	// NumBlocksUntilTxnFinality is the number of blocks that have passed after a txn has
	// been mined to consider it have reached *finality* (aka Confirmations).
	WaitTimeUntilTxnFinality time.Duration

	// WaitNumBlocksBeforeExhaustion is the number of blocks that have passed since a get receipt
	// request is made until we error with ErrExhausted. The client may want to try to find the receipt
	// again, or it might re-price a txn, or other.
	WaitTimeBeforeExhaustion time.Duration
}

var DefaultOptions = Options{
	WaitTimeUntilTxnFinality: 12 * time.Second, // Confirmation blocks
	WaitTimeBeforeExhaustion: 18 * time.Second,
}

type Event uint32

// We might not need Unknown here, we can use Discarded instead.
const (
	Finalized Event = iota
	Dropped
	Unknown
)

var (
	ErrExhausted = errors.New("ethreceipts: exhausted looking for receipt")
)

func NewReceipts(log zerolog.Logger, provider *ethrpc.Provider, monitor *ethmonitor.Monitor, opts ...Options) (*Receipts, error) {
	options := DefaultOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	if options.WaitTimeUntilTxnFinality <= 0 {
		return nil, fmt.Errorf("ethreceipts: invalid option, WaitTimeUntilTxnFinality")
	}
	if options.WaitTimeBeforeExhaustion <= 2 {
		return nil, fmt.Errorf("ethreceipts: invalid option, WaitTimeBeforeExhaustion")
	}

	return &Receipts{
		options:  options,
		log:      log.With().Str("ps", "receipts").Logger(),
		provider: provider,
	}, nil
}

func (r *Receipts) Run(ctx context.Context) error {
	if r.IsRunning() {
		return fmt.Errorf("ethreceipts: already running")
	}

	r.ctx, r.ctxStop = context.WithCancel(ctx)

	atomic.StoreInt32(&r.running, 1)
	defer atomic.StoreInt32(&r.running, 0)

	// NOTE: this first version new of ethreceipts doesn't have a sub-process running. The Run`ner
	// is just mocked here for when we do something more advanced described below with ethmonitor.
	<-r.ctx.Done()

	// TODO: so, the current version of ethreceipts will poll for the receipts. However,
	// this will be a ton of http requests to the node coming from this service. It's fine for now,
	// but in the next iteration we can use a single ethmonitor instance and listen for txn hashes
	// as they are mined, and then fetch the receipt afterwards once we know it'll be available for sure.
	// We can also use the monitor's cache and block retention to look for historical receipts
	// up to a certain limit too.

	return nil
}

func (r *Receipts) Stop() {
	if !r.IsRunning() {
		return
	}
	r.log.Info().Str("op", "stop").Msg("-> receipts: stopping..")
	r.ctxStop()
	r.log.Info().Str("op", "stop").Msg("-> receipts: stopped")
}

func (r *Receipts) IsRunning() bool {
	return atomic.LoadInt32(&r.running) == 1
}

func (r *Receipts) Options() Options {
	return r.options
}

// GetTransactionReceipt is a blocking operation that will listen for chain blocks looking for the txn hash
// provided and will then respond with the receipt.
func (r *Receipts) GetTransactionReceipt(ctx context.Context, txnHash common.Hash) (*types.Receipt, error) {
	startTime := time.Now()
	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("%w: unable to find %v: %v", ErrExhausted, txnHash.Hex(), err)
			}

		// TODO: can subscription.Blocks block forever sometimes..?
		default:
			// Ignore errors
			// if the transaction fails, the wait will expire eventually anyway
			receipt, _ := r.provider.TransactionReceipt(ctx, txnHash)

			if receipt != nil {
				return receipt, nil
			}

			// At this time, we're just returning exhaustion based on time and not blocks,
			// but the PollTime should be set to be around the time of the chain's ave block-time.
			// In future we will use the ethmonitor for more precision.
			if startTime.Add(r.options.WaitTimeBeforeExhaustion).After(time.Now()) {
				return nil, fmt.Errorf("%w: unable to find %v after waiting %v seconds", ErrExhausted, txnHash.Hex(), r.options.WaitTimeBeforeExhaustion.Seconds())
			}
		}
	}
}

// TODO: we can add a method....... GetTransactionReceiptAfterBlock .. #, so we can specify
// after certain block height, to then fetch the receipt.. or call the method GetFinalTransactionReceipt is pretty obvious too?
//
// we might want to setup a "finalizer" process that uses ethreceipts and has its own workers..
// might be better..
// Or maybe we can just requeue the txns to our senders

// GetFinalTransactionReceipt is a blocking operation that will listen for chain blocks looking for the txn hash and will wait till K number
// of blocks before returning the receipt.

// Will that even work? (Lol)
func (r *Receipts) GetFinalTransactionReceipt(ctx context.Context, txnHash common.Hash) (*types.Receipt, Event, error) {
	receipt, err := r.GetTransactionReceipt(ctx, txnHash)
	if err != nil {
		return nil, Dropped, err
	}

	if receipt == nil {
		return nil, Dropped, fmt.Errorf("ethreceipts: receipt is nil")
	}

	time.Sleep(r.options.WaitTimeUntilTxnFinality)
	receipt, err = r.GetTransactionReceipt(ctx, txnHash)
	if err != nil {
		return nil, Dropped, err
	}

	if receipt == nil {
		return nil, Dropped, fmt.Errorf("ethreceipts: receipt is nil")
	}

	return receipt, Finalized, nil
}
