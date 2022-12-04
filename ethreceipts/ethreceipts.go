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

	monitor *ethmonitor.Monitor

	ctx     context.Context
	ctxStop context.CancelFunc
	running int32
}

type Options struct {
	// NumBlocksUntilTxnFinality is the number of blocks that have passed after a txn has
	// been mined to consider it have reached *finality* (aka its there for good now).
	NumBlocksUntilTxnFinality int

	// WaitNumBlocksBeforeExhaustion is the number of blocks that have passed since a get receipt
	// request is made until we error with ErrExhausted. The client may want to try to find the receipt
	// again, or it might re-price a txn, or other.
	WaitNumBlocksBeforeExhaustion int
}

var DefaultOptions = Options{
	NumBlocksUntilTxnFinality:     12,
	WaitNumBlocksBeforeExhaustion: 3,
}

type Event uint32

// We might not need Unknown here, we can use Dropped instead.
const (
	Unknown Event = iota
	Dropped
	Finalized
)

var (
	ErrExhausted = errors.New("ethreceipts: exhausted looking for receipt")
)

func NewReceipts(log zerolog.Logger, provider *ethrpc.Provider, monitor *ethmonitor.Monitor, opts ...Options) (*Receipts, error) {
	options := DefaultOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	if options.NumBlocksUntilTxnFinality <= 0 {
		return nil, fmt.Errorf("ethreceipts: invalid option, NumBlocksUntilTxnFinality")
	}
	if options.WaitNumBlocksBeforeExhaustion <= 2 {
		return nil, fmt.Errorf("ethreceipts: invalid option, WaitNumBlocksBeforeExhaustion")
	}

	return &Receipts{
		options:  options,
		log:      log.With().Str("ps", "receipts").Logger(),
		provider: provider,
		monitor:  monitor,
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
	// as they are mined, and then fetch the receipt afterwards once we know it'll be available for sure. (Done)
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

		default:
			tx := r.monitor.GetTransaction(txnHash)
			if tx != nil {
				receipt, _ := r.provider.TransactionReceipt(ctx, txnHash)
				if receipt != nil {
					return receipt, nil
				}
			}

			if startTime.Add(time.Duration(r.monitor.GetAverageBlockTime() * float64(r.options.WaitNumBlocksBeforeExhaustion))).After(time.Now()) {
				return nil, fmt.Errorf("%w: unable to find %v after %v blocks", ErrExhausted, txnHash.Hex(), r.options.WaitNumBlocksBeforeExhaustion)
			}

		}
	}
}

// GetFinalTransactionReceipt is a blocking operation that will listen for txn hash and retrieve tx receipt and will wait till K number
// of blocks before returning the receipt. (Always send in a go routine to prevent blocking the main thread)
func (r *Receipts) GetFinalTransactionReceipt(ctx context.Context, txnHash common.Hash) (*types.Receipt, Event, error) {
	startTime := time.Now()
	receiptBlock := uint64(0)
	var receipt *types.Receipt
	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return nil, Unknown, fmt.Errorf("%w: unable to find %v: %v", ErrExhausted, txnHash.Hex(), err)
			}
		default:
			if receipt == nil {
				// checking for a transaction receipt from the node for our transaction hash
				tx := r.monitor.GetTransaction(txnHash)
				if tx != nil {
					receipt, _ = r.provider.TransactionReceipt(ctx, txnHash)
					if receipt != nil {
						r.log.Info().Msgf("Received the first receipt of txn %v at %v", txnHash.Hex(), receipt.BlockNumber)
						receiptBlock = receipt.BlockNumber.Uint64()
					}
				} else {
					if startTime.Add(time.Duration(r.monitor.GetAverageBlockTime() * float64(r.options.WaitNumBlocksBeforeExhaustion))).After(time.Now()) {
						return nil, Dropped, fmt.Errorf("%w: unable to find %v after %v blocks", ErrExhausted, txnHash.Hex(), r.options.WaitNumBlocksBeforeExhaustion)
					}
				}
			}
			if receipt != nil {
				// Let's wait for blocks*avgBlockTime time before fetching the receipt again
				// TODO: Decrease the time in next iteration
				timeToSleep := r.monitor.GetAverageBlockTime() * float64(r.options.NumBlocksUntilTxnFinality-(int(receiptBlock)-int(receipt.BlockNumber.Uint64())))
				time.Sleep(time.Second * time.Duration(timeToSleep))

				if receiptBlock+uint64(r.options.NumBlocksUntilTxnFinality) <= r.monitor.LatestBlock().Number().Uint64() {
					receipt, _ := r.provider.TransactionReceipt(ctx, txnHash)
					if receipt != nil {
						r.log.Info().Msgf("Received the second receipt of txn %v at %v", txnHash.Hex(), receipt.BlockNumber)
						return receipt, Finalized, nil
					} else {
						return nil, Dropped, fmt.Errorf("%w: unable to find %v after %v blocks. txn was dropped after reorg", ErrExhausted, txnHash.Hex(), r.options.NumBlocksUntilTxnFinality)
					}
				}
			}

		}
	}
}
