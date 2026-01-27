// Package ethfinalizer implements a wallet adapter for guaranteeing transaction inclusion on a specific chain.
//
// This fixes "nonce too low" issues that can happen if reorgs occur or if you trust your node's reported nonces.
package ethfinalizer

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

// FinalizerOptions defines finalizer options.
//
// Type parameters:
//   - T: transaction metadata type
type FinalizerOptions[T any] struct {
	// Wallet is the wallet to be managed by this finalizer, required.
	Wallet *ethwallet.Wallet
	// Chain is the provider for the chain where transactions will be sent, required.
	// See NewEthkitChain for an implementation using ethkit components.
	Chain Chain
	// Mempool stores transactions that this finalizer creates, required.
	// See NewMemoryMempool for a minimal in-memory implementation.
	Mempool Mempool[T]
	// Logger is used to log finalizer behaviour, optional.
	Logger *slog.Logger

	// PollInterval is the period between chain state checks.
	// Required, must be positive.
	PollInterval time.Duration
	// PollTimeout is the time limit for operations while checking chain state.
	// Required, must be positive.
	PollTimeout time.Duration
	// RetryDelay is the minimum time to wait before retrying a transaction.
	// Required, must be positive.
	RetryDelay time.Duration

	// FeeMargin is the percentage added on top of the estimated gas price.
	FeeMargin int
	// PriceBump is the percentage increase when replacing pending transactions.
	// Recommended to be at least 15, 10 is the default chosen by go-ethereum.
	PriceBump int

	// SubscriptionBuffer is the size of the buffer for transaction events.
	SubscriptionBuffer int
}

func (o FinalizerOptions[T]) IsValid() error {
	if o.Wallet == nil {
		return fmt.Errorf("no wallet")
	}

	if o.Chain == nil {
		return fmt.Errorf("no chain")
	}

	if o.Mempool == nil {
		return fmt.Errorf("no mempool")
	}

	if o.PollInterval <= 0 {
		return fmt.Errorf("non-positive poll interval %v", o.PollInterval)
	}

	if o.PollTimeout <= 0 {
		return fmt.Errorf("non-positive poll timeout %v", o.PollTimeout)
	}

	if o.RetryDelay <= 0 {
		return fmt.Errorf("non-positive retry delay %v", o.RetryDelay)
	}

	if o.FeeMargin < 0 {
		return fmt.Errorf("negative fee margin %v", o.FeeMargin)
	}

	if o.PriceBump < 0 {
		return fmt.Errorf("negative price bump %v", o.PriceBump)
	}

	if o.SubscriptionBuffer < 0 {
		return fmt.Errorf("negative subscription buffer %v", o.SubscriptionBuffer)
	}

	return nil
}

// Finalizer is a wallet adapter for guaranteeing transaction inclusion on a specific chain.
//
// Type parameters:
//   - T: transaction metadata type
type Finalizer[T any] struct {
	FinalizerOptions[T]

	isRunning atomic.Bool

	subscriptions   map[chan Event[T]]struct{}
	subscriptionsMu sync.RWMutex

	sendMu sync.Mutex
}

// Event describes when a Transaction is mined or reorged.
//   - Transaction mined: Removed is nil, Added is not nil.
//   - Transaction reorged: Removed is not nil, Added may or may not be nil.
//
// Type parameters:
//   - T: transaction metadata type
type Event[T any] struct {
	// Removed is the transaction that was reorged, nil if this was a mining event.
	Removed *Transaction[T]
	// Added is the transaction that was mined, may be nil if a reorg occurred and the reorged transaction was never replaced.
	Added *Transaction[T]
}

// Transaction is a transaction with metadata of type T.
//
// Type parameters:
//   - T: metadata type
type Transaction[T any] struct {
	*types.Transaction

	Metadata T
}

// NewFinalizer creates a new Finalizer with FinalizerOptions which must be Run.
//
// Type parameters:
//   - T: transaction metadata type
func NewFinalizer[T any](options FinalizerOptions[T]) (*Finalizer[T], error) {
	if err := options.IsValid(); err != nil {
		return nil, err
	}

	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}

	return &Finalizer[T]{
		FinalizerOptions: options,

		subscriptions: map[chan Event[T]]struct{}{},
	}, nil
}

func (f *Finalizer[T]) IsRunning() bool {
	return f.isRunning.Load()
}

func (f *Finalizer[T]) Run(ctx context.Context) error {
	if !f.isRunning.CompareAndSwap(false, true) {
		return fmt.Errorf("already running")
	}
	defer f.isRunning.Store(false)

	diffs, err := f.Chain.Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("unable to subscribe to chain: %w", err)
	}

	go func() {
		for diff := range diffs {
			func() {
				events := map[uint64]*Event[T]{}

				ctx, cancel := context.WithTimeout(ctx, f.PollTimeout)
				defer cancel()

				removed, err := f.Mempool.Transactions(ctx, diff.Removed)
				if err != nil {
					f.Logger.ErrorContext(ctx, "unable to read removed transactions", slog.Any("error", err))
				}

				added, err := f.Mempool.Transactions(ctx, diff.Added)
				if err != nil {
					f.Logger.ErrorContext(ctx, "unable to read added transactions", slog.Any("error", err))
				}

				for _, transaction := range removed {
					if transaction != nil {
						event := events[transaction.Nonce()]
						if event == nil {
							event = &Event[T]{}
							events[transaction.Nonce()] = event
						}

						event.Removed = transaction

						f.Logger.DebugContext(
							ctx,
							"transaction reorged",
							slog.String("transaction", transaction.Hash().String()),
							slog.Uint64("nonce", transaction.Nonce()),
						)
					}
				}

				for _, transaction := range added {
					if transaction != nil {
						event := events[transaction.Nonce()]
						if event == nil {
							event = &Event[T]{}
							events[transaction.Nonce()] = event
						}

						event.Added = transaction

						f.Logger.DebugContext(
							ctx,
							"transaction mined",
							slog.String("transaction", transaction.Hash().String()),
							slog.Uint64("nonce", transaction.Nonce()),
							slog.String("gasPrice", transaction.GasPrice().String()),
							slog.String("priorityFee", transaction.GasTipCap().String()),
						)
					}
				}

				f.subscriptionsMu.RLock()
				defer f.subscriptionsMu.RUnlock()

				for _, event := range events {
					if event.Added == nil || event.Removed == nil || event.Added.Hash() != event.Removed.Hash() {
						for subscription := range f.subscriptions {
							subscription <- *event
						}
					}
				}
			}()
		}
	}()

	ticks := time.Tick(f.PollInterval)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticks:
			err := func() error {
				ctx, cancel := context.WithTimeout(ctx, f.PollTimeout)
				defer cancel()

				f.Logger.DebugContext(ctx, "polling", slog.Duration("interval", f.PollInterval), slog.Duration("timeout", f.PollTimeout))

				chainNonce, err := f.Chain.LatestNonce(ctx, f.Wallet.Address())
				if err != nil {
					return fmt.Errorf("unable to read chain nonce: %w", err)
				}

				transactions, err := f.Mempool.PriciestTransactions(ctx, chainNonce, time.Now().Add(-f.RetryDelay))
				if err != nil {
					return fmt.Errorf("unable to read mempool transactions: %w", err)
				}

				var gasPrice, priorityFee *big.Int
				if f.Chain.IsEIP1559() {
					baseFee, err := f.Chain.BaseFee(ctx)
					if err != nil || baseFee == nil {
						f.Logger.ErrorContext(ctx, "unable to read base fee", slog.Any("error", err))
						baseFee = new(big.Int)
					}
					baseFee = withMargin(baseFee, f.FeeMargin)

					if priorityFee, err = f.Chain.PriorityFee(ctx); err != nil || priorityFee == nil {
						f.Logger.ErrorContext(ctx, "unable to read priority fee", slog.Any("error", err))
						priorityFee = new(big.Int)
					}

					gasPrice = new(big.Int).Add(baseFee, priorityFee)
				} else {
					if gasPrice, err = f.Chain.GasPrice(ctx); err != nil || gasPrice == nil {
						f.Logger.ErrorContext(ctx, "unable to read gas price", slog.Any("error", err))
						gasPrice = new(big.Int)
					}
					gasPrice = withMargin(gasPrice, f.FeeMargin)
				}

				nonce := chainNonce
				for ; transactions[nonce] != nil; nonce++ {
					transaction := transactions[nonce]

					var replacement *types.Transaction
					switch transaction.Type() {
					case types.LegacyTxType:
						if transaction.GasPrice().Cmp(gasPrice) < 0 {
							gasPrice := maxBigInt(withMargin(transaction.GasPrice(), f.PriceBump), gasPrice)

							replacement = types.NewTx(&types.LegacyTx{
								Nonce:    transaction.Nonce(),
								GasPrice: gasPrice,
								Gas:      transaction.Gas(),
								To:       transaction.To(),
								Value:    transaction.Value(),
								Data:     transaction.Data(),
							})

							f.Logger.DebugContext(
								ctx,
								"replacing",
								slog.String("transaction", transaction.Hash().String()),
								slog.Uint64("nonce", transaction.Nonce()),
								slog.String("oldGasPrice", transaction.GasPrice().String()),
								slog.String("newGasPrice", replacement.GasPrice().String()),
							)
						}

					case types.AccessListTxType:
						if transaction.GasPrice().Cmp(gasPrice) < 0 {
							gasPrice := maxBigInt(withMargin(transaction.GasPrice(), f.PriceBump), gasPrice)

							replacement = types.NewTx(&types.AccessListTx{
								ChainID:    transaction.ChainId(),
								Nonce:      transaction.Nonce(),
								GasPrice:   gasPrice,
								Gas:        transaction.Gas(),
								To:         transaction.To(),
								Value:      transaction.Value(),
								Data:       transaction.Data(),
								AccessList: transaction.AccessList(),
							})

							f.Logger.DebugContext(
								ctx,
								"replacing",
								slog.String("transaction", transaction.Hash().String()),
								slog.Uint64("nonce", transaction.Nonce()),
								slog.String("oldGasPrice", transaction.GasPrice().String()),
								slog.String("newGasPrice", replacement.GasPrice().String()),
							)
						}

					case types.DynamicFeeTxType:
						if transaction.GasFeeCapIntCmp(gasPrice) < 0 || transaction.GasTipCapIntCmp(priorityFee) < 0 {
							gasPrice := maxBigInt(withMargin(transaction.GasFeeCap(), f.PriceBump), gasPrice)
							priorityFee := maxBigInt(withMargin(transaction.GasTipCap(), f.PriceBump), priorityFee)

							replacement = types.NewTx(&types.DynamicFeeTx{
								ChainID:    transaction.ChainId(),
								Nonce:      transaction.Nonce(),
								GasTipCap:  priorityFee,
								GasFeeCap:  gasPrice,
								Gas:        transaction.Gas(),
								To:         transaction.To(),
								Value:      transaction.Value(),
								Data:       transaction.Data(),
								AccessList: transaction.AccessList(),
							})

							f.Logger.DebugContext(
								ctx,
								"replacing",
								slog.String("transaction", transaction.Hash().String()),
								slog.Uint64("nonce", transaction.Nonce()),
								slog.String("oldGasPrice", transaction.GasFeeCap().String()),
								slog.String("newGasPrice", replacement.GasFeeCap().String()),
								slog.String("oldPriorityFee", transaction.GasTipCap().String()),
								slog.String("newPriorityFee", replacement.GasTipCap().String()),
							)
						}

					case types.BlobTxType:
						if transaction.GasFeeCapIntCmp(gasPrice) < 0 || transaction.GasTipCapIntCmp(priorityFee) < 0 {
							gasPrice := maxBigInt(withMargin(transaction.GasFeeCap(), f.PriceBump), gasPrice)
							priorityFee := maxBigInt(withMargin(transaction.GasTipCap(), f.PriceBump), priorityFee)

							replacement = types.NewTx(&types.BlobTx{
								ChainID:    uint256.MustFromBig(transaction.ChainId()),
								Nonce:      transaction.Nonce(),
								GasTipCap:  uint256.MustFromBig(priorityFee),
								GasFeeCap:  uint256.MustFromBig(gasPrice),
								Gas:        transaction.Gas(),
								To:         dereference(transaction.To()),
								Value:      uint256.MustFromBig(transaction.Value()),
								Data:       transaction.Data(),
								AccessList: transaction.AccessList(),
								BlobFeeCap: uint256.MustFromBig(withMargin(transaction.BlobGasFeeCap(), f.PriceBump)),
								BlobHashes: transaction.BlobHashes(),
								Sidecar:    transaction.BlobTxSidecar(),
							})

							f.Logger.DebugContext(
								ctx,
								"replacing",
								slog.String("transaction", transaction.Hash().String()),
								slog.Uint64("nonce", transaction.Nonce()),
								slog.String("oldGasPrice", transaction.GasFeeCap().String()),
								slog.String("newGasPrice", replacement.GasFeeCap().String()),
								slog.String("oldPriorityFee", transaction.GasTipCap().String()),
								slog.String("newPriorityFee", replacement.GasTipCap().String()),
							)
						}

					case types.SetCodeTxType:
						if transaction.GasFeeCapIntCmp(gasPrice) < 0 || transaction.GasTipCapIntCmp(priorityFee) < 0 {
							gasPrice := maxBigInt(withMargin(transaction.GasFeeCap(), f.PriceBump), gasPrice)
							priorityFee := maxBigInt(withMargin(transaction.GasTipCap(), f.PriceBump), priorityFee)

							replacement = types.NewTx(&types.SetCodeTx{
								ChainID:    uint256.MustFromBig(transaction.ChainId()),
								Nonce:      transaction.Nonce(),
								GasTipCap:  uint256.MustFromBig(priorityFee),
								GasFeeCap:  uint256.MustFromBig(gasPrice),
								Gas:        transaction.Gas(),
								To:         dereference(transaction.To()),
								Value:      uint256.MustFromBig(transaction.Value()),
								Data:       transaction.Data(),
								AccessList: transaction.AccessList(),
								AuthList:   transaction.SetCodeAuthorizations(),
							})

							f.Logger.DebugContext(
								ctx,
								"replacing",
								slog.String("transaction", transaction.Hash().String()),
								slog.Uint64("nonce", transaction.Nonce()),
								slog.String("oldGasPrice", transaction.GasFeeCap().String()),
								slog.String("newGasPrice", replacement.GasFeeCap().String()),
								slog.String("oldPriorityFee", transaction.GasTipCap().String()),
								slog.String("newPriorityFee", replacement.GasTipCap().String()),
							)
						}
					}

					if replacement == nil {
						f.Logger.DebugContext(
							ctx,
							"resending",
							slog.String("transaction", transaction.Hash().String()),
							slog.Uint64("nonce", transaction.Nonce()),
							slog.String("gasPrice", transaction.GasPrice().String()),
							slog.String("priorityFee", transaction.GasTipCap().String()),
						)

						if err := f.Mempool.Commit(ctx, transaction.Transaction, transaction.Metadata); err != nil {
							f.Logger.ErrorContext(ctx, "unable to update transaction in mempool", slog.Any("error", err), slog.String("transaction", transaction.Hash().String()))
						}

						if err := f.Chain.Send(ctx, transaction.Transaction); err != nil {
							f.Logger.ErrorContext(ctx, "unable to resend transaction to chain", slog.Any("error", err), slog.String("transaction", transaction.Hash().String()))
						}
					} else {
						if replacement, err = f.Wallet.SignTransaction(replacement, f.Chain.ChainID()); err == nil {
							if err := f.Mempool.Commit(ctx, replacement, transaction.Metadata); err != nil {
								f.Logger.ErrorContext(ctx, "unable to commit replacement transaction to mempool", slog.Any("error", err))
								continue
							}

							if err := f.Chain.Send(ctx, replacement); err != nil {
								f.Logger.ErrorContext(ctx, "unable to send replacement transaction to chain", slog.Any("error", err), slog.String("transaction", replacement.Hash().String()))
							}
						} else {
							f.Logger.ErrorContext(ctx, "unable to sign replacement transaction", slog.Any("error", err))
							continue
						}
					}
				}

				if nonce-chainNonce != uint64(len(transactions)) {
					f.Logger.WarnContext(ctx, "missing nonce", slog.Uint64("from", chainNonce), slog.Uint64("missing", nonce), slog.Int("transactions", len(transactions)))
				}

				return nil
			}()
			if err != nil {
				f.Logger.ErrorContext(ctx, "unable to poll", slog.Any("error", err))
			}
		}
	}
}

// Subscribe returns a channel for receiving transaction mining and reorg events.
//
// The finalizer must be running to receive events.
func (f *Finalizer[T]) Subscribe(ctx context.Context) <-chan Event[T] {
	f.subscriptionsMu.Lock()
	defer f.subscriptionsMu.Unlock()

	subscription := make(chan Event[T], f.SubscriptionBuffer)
	f.subscriptions[subscription] = struct{}{}

	go func() {
		<-ctx.Done()

		f.subscriptionsMu.Lock()
		defer f.subscriptionsMu.Unlock()

		delete(f.subscriptions, subscription)
		close(subscription)
	}()

	return subscription
}

// Send sends an unsigned transaction on chain.
//
// metadata is optional transaction data that is stored in the mempool and reappears in events. It is not sent on chain.
// A signed transaction is returned. This may or may not be the final transaction included on chain.
func (f *Finalizer[T]) Send(ctx context.Context, transaction *types.Transaction, metadata T) (*types.Transaction, error) {
	f.sendMu.Lock()
	defer f.sendMu.Unlock()

	var gasPrice, priorityFee *big.Int
	var err error
	if f.Chain.IsEIP1559() {
		baseFee, err := f.Chain.BaseFee(ctx)
		if err != nil || baseFee == nil {
			f.Logger.ErrorContext(ctx, "unable to read base fee", slog.Any("error", err))
			baseFee = new(big.Int)
		}
		baseFee = withMargin(baseFee, f.FeeMargin)

		if priorityFee, err = f.Chain.PriorityFee(ctx); err != nil || priorityFee == nil {
			f.Logger.ErrorContext(ctx, "unable to read priority fee", slog.Any("error", err))
			priorityFee = new(big.Int)
		}

		gasPrice = new(big.Int).Add(baseFee, priorityFee)
	} else {
		if gasPrice, err = f.Chain.GasPrice(ctx); err != nil || gasPrice == nil {
			f.Logger.ErrorContext(ctx, "unable to read gas price", slog.Any("error", err))
			gasPrice = new(big.Int)
		}
		gasPrice = withMargin(gasPrice, f.FeeMargin)
	}

	switch transaction.Type() {
	case types.LegacyTxType:
		transaction = types.NewTx(&types.LegacyTx{
			Nonce:    transaction.Nonce(),
			GasPrice: maxBigInt(transaction.GasPrice(), gasPrice),
			Gas:      transaction.Gas(),
			To:       transaction.To(),
			Value:    transaction.Value(),
			Data:     transaction.Data(),
		})

	case types.AccessListTxType:
		transaction = types.NewTx(&types.AccessListTx{
			ChainID:    transaction.ChainId(),
			Nonce:      transaction.Nonce(),
			GasPrice:   maxBigInt(transaction.GasPrice(), gasPrice),
			Gas:        transaction.Gas(),
			To:         transaction.To(),
			Value:      transaction.Value(),
			Data:       transaction.Data(),
			AccessList: transaction.AccessList(),
		})

	case types.DynamicFeeTxType:
		if !f.Chain.IsEIP1559() {
			return nil, fmt.Errorf("chain does not support eip-1559 transactions")
		}

		transaction = types.NewTx(&types.DynamicFeeTx{
			ChainID:    transaction.ChainId(),
			Nonce:      transaction.Nonce(),
			GasTipCap:  maxBigInt(transaction.GasTipCap(), priorityFee),
			GasFeeCap:  maxBigInt(transaction.GasFeeCap(), gasPrice),
			Gas:        transaction.Gas(),
			To:         transaction.To(),
			Value:      transaction.Value(),
			Data:       transaction.Data(),
			AccessList: transaction.AccessList(),
		})

	case types.BlobTxType:
		if !f.Chain.IsEIP1559() {
			return nil, fmt.Errorf("chain does not support eip-1559 transactions")
		}

		transaction = types.NewTx(&types.BlobTx{
			ChainID:    uint256.MustFromBig(transaction.ChainId()),
			Nonce:      transaction.Nonce(),
			GasTipCap:  uint256.MustFromBig(maxBigInt(transaction.GasTipCap(), priorityFee)),
			GasFeeCap:  uint256.MustFromBig(maxBigInt(transaction.GasFeeCap(), gasPrice)),
			Gas:        transaction.Gas(),
			To:         dereference(transaction.To()),
			Value:      uint256.MustFromBig(transaction.Value()),
			Data:       transaction.Data(),
			AccessList: transaction.AccessList(),
			BlobFeeCap: uint256.MustFromBig(transaction.BlobGasFeeCap()),
			BlobHashes: transaction.BlobHashes(),
			Sidecar:    transaction.BlobTxSidecar(),
		})

	case types.SetCodeTxType:
		if !f.Chain.IsEIP1559() {
			return nil, fmt.Errorf("chain does not support eip-1559 transactions")
		}

		transaction = types.NewTx(&types.SetCodeTx{
			ChainID:    uint256.MustFromBig(transaction.ChainId()),
			Nonce:      transaction.Nonce(),
			GasTipCap:  uint256.MustFromBig(maxBigInt(transaction.GasTipCap(), priorityFee)),
			GasFeeCap:  uint256.MustFromBig(maxBigInt(transaction.GasFeeCap(), gasPrice)),
			Gas:        transaction.Gas(),
			To:         dereference(transaction.To()),
			Value:      uint256.MustFromBig(transaction.Value()),
			Data:       transaction.Data(),
			AccessList: transaction.AccessList(),
			AuthList:   transaction.SetCodeAuthorizations(),
		})
	}

	mempoolNonce, err := f.Mempool.Nonce(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to read mempool nonce: %w", err)
	}

	chainNonce, err := f.Chain.PendingNonce(ctx, f.Wallet.Address())
	if err != nil {
		return nil, fmt.Errorf("unable to read chain nonce: %w", err)
	}

	if chainNonce > mempoolNonce {
		f.Logger.WarnContext(ctx, "chain nonce > mempool nonce", slog.Uint64("chain", chainNonce), slog.Uint64("mempool", mempoolNonce))
	}

	transaction = withNonce(transaction, max(mempoolNonce, chainNonce))

	transaction, err = f.Wallet.SignTransaction(transaction, f.Chain.ChainID())
	if err != nil {
		return nil, fmt.Errorf("unable to sign transaction: %w", err)
	}

	f.Logger.DebugContext(
		ctx,
		"sending",
		slog.String("transaction", transaction.Hash().String()),
		slog.Uint64("nonce", transaction.Nonce()),
		slog.String("gasPrice", transaction.GasPrice().String()),
		slog.String("priorityFee", transaction.GasTipCap().String()),
	)

	if err := f.Mempool.Commit(ctx, transaction, metadata); err != nil {
		return nil, fmt.Errorf("unable to commit transaction to mempool: %w", err)
	}

	if err := f.Chain.Send(ctx, transaction); err != nil {
		f.Logger.ErrorContext(ctx, "unable to send transaction to chain", slog.Any("error", err), slog.String("transaction", transaction.Hash().String()))
	}

	return transaction, nil
}
