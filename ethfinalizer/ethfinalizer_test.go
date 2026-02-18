package ethfinalizer_test

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethfinalizer"
	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

const (
	TestDuration                     = 10 * time.Second
	MonitorPollInterval              = 100 * time.Millisecond
	FinalizerPollInterval            = 1 * time.Second
	FinalizerPollTimeout             = 1 * time.Second
	FinalizerRetryDelay              = 5 * time.Second
	FinalizerFeeMargin               = 20
	FinalizerPriceBump               = 10
	FinalizerNonceStuckTimeout       = 10 * time.Second
	FinalizerTransactionStuckTimeout = 5 * time.Second
	TransactionsPerSecond            = 2
	BlockPeriod                      = 2 * time.Second
	StallPeriod                      = 20 * time.Second
	MineProbability                  = 0.8
	ReorgProbability                 = 0.1
	ReorgLimit                       = 10
)

func TestFinalizerNoEIP1559(t *testing.T) {
	test(t, false)
}

func TestFinalizerEIP1559(t *testing.T) {
	test(t, true)
}

func test(t *testing.T, isEIP1559 bool) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	wallet, err := ethwallet.NewWalletFromMnemonic("major danger this key only test please avoid main net use okay")
	assert.NoError(t, err)

	chain, err := NewTestChain(TestChainOptions{
		IsEIP1559: isEIP1559,

		Logger: logger,

		MinGasPrice:    1000000000,
		MaxGasPrice:    100000000000,
		MinBaseFee:     1000000000,
		MaxBaseFee:     100000000000,
		MinPriorityFee: 1,
		MaxPriorityFee: 1000000000,
	})
	assert.NoError(t, err)

	mempool := ethfinalizer.NewMemoryMempool[struct{}]()

	ctx, cancel := context.WithTimeout(context.Background(), TestDuration)
	defer cancel()

	finalizer, err := ethfinalizer.NewFinalizer(ethfinalizer.FinalizerOptions[struct{}]{
		Wallet:                  wallet,
		Chain:                   chain,
		Mempool:                 mempool,
		Logger:                  logger,
		PollInterval:            FinalizerPollInterval,
		PollTimeout:             FinalizerPollTimeout,
		RetryDelay:              FinalizerRetryDelay,
		FeeMargin:               FinalizerFeeMargin,
		PriceBump:               FinalizerPriceBump,
		NonceStuckTimeout:       FinalizerNonceStuckTimeout,
		TransactionStuckTimeout: FinalizerTransactionStuckTimeout,
		OnStuck: func(first, latest *ethfinalizer.Status[struct{}]) {
			logger.DebugContext(
				ctx,
				"stuck",
				slog.String("first", first.Transaction.Hash().String()),
				slog.Uint64("firstNonce", first.Transaction.Nonce()),
				slog.Duration("firstAge", time.Since(first.Time)),
				slog.String("latest", latest.Transaction.Hash().String()),
				slog.Uint64("latestNonce", latest.Transaction.Nonce()),
				slog.Duration("latestAge", time.Since(latest.Time)),
			)
		},
		OnUnstuck: func() {
			logger.DebugContext(ctx, "unstuck")
		},
	})
	assert.NoError(t, err)

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(MonitorPollInterval):
				chain.Publish()
			}
		}
	})

	wg.Go(func() {
		for event := range finalizer.Subscribe(ctx) {
			if event.Removed == nil {
				logger.DebugContext(
					ctx,
					"event",
					slog.String("added", event.Added.Hash().String()),
					slog.Uint64("addedNonce", event.Added.Nonce()),
				)
			} else if event.Added == nil {
				logger.DebugContext(
					ctx,
					"event",
					slog.String("removed", event.Removed.Hash().String()),
					slog.Uint64("removedNonce", event.Removed.Nonce()),
				)
			} else {
				logger.DebugContext(
					ctx,
					"event",
					slog.String("added", event.Added.Hash().String()),
					slog.Uint64("addedNonce", event.Added.Nonce()),
					slog.String("removed", event.Removed.Hash().String()),
					slog.Uint64("removedNonce", event.Removed.Nonce()),
				)
			}
		}
	})

	wg.Go(func() {
		finalizer.Run(ctx)
	})

	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Duration(float64(time.Second) * rand.ExpFloat64() / TransactionsPerSecond)):
				to := wallet.Address()

				if !chain.IsEIP1559() || rand.Float64() < 0.5 {
					finalizer.Send(ctx, types.NewTx(&types.LegacyTx{
						To:    &to,
						Value: common.Big1,
					}), struct{}{})
				} else {
					finalizer.Send(ctx, types.NewTx(&types.DynamicFeeTx{
						To:    &to,
						Value: common.Big1,
					}), struct{}{})
				}
			}
		}
	})

	wg.Go(func() {
		for ctx.Err() == nil {
			x := rand.Float64()

			if x < MineProbability {
				chainNonce, _ := chain.LatestNonce(ctx, wallet.Address())
				mempoolNonce, _ := mempool.Nonce(ctx)

				if chainNonce+1 <= mempoolNonce {
					chain.SetNonce(uniform(chainNonce+1, mempoolNonce))
				}

				time.Sleep(time.Duration(float64(BlockPeriod) * rand.ExpFloat64()))
				continue
			}
			x -= MineProbability

			if x < ReorgProbability {
				chainNonce, _ := chain.LatestNonce(ctx, wallet.Address())

				if chainNonce != 0 {
					var limit uint64
					if chainNonce > ReorgLimit {
						limit = chainNonce - ReorgLimit
					}

					chain.SetNonce(uniform(limit, chainNonce-1))
				}

				time.Sleep(time.Duration(float64(BlockPeriod) * rand.ExpFloat64()))
				continue
			}
			x -= ReorgProbability

			time.Sleep(time.Duration(float64(StallPeriod) * rand.ExpFloat64()))
		}
	})

	wg.Wait()
}

type TestChainOptions struct {
	ChainID   *big.Int
	IsEIP1559 bool

	Logger *slog.Logger

	MinGasPrice, MaxGasPrice       uint64
	MinBaseFee, MaxBaseFee         uint64
	MinPriorityFee, MaxPriorityFee uint64
}

func (o TestChainOptions) IsValid() error {
	if o.ChainID != nil && o.ChainID.Sign() <= 0 {
		return fmt.Errorf("non-positive chain id %v", o.ChainID)
	}

	if o.MaxGasPrice < o.MinGasPrice {
		return fmt.Errorf("max gas price %v < min gas price %v", o.MaxGasPrice, o.MinGasPrice)
	}
	if o.MaxBaseFee < o.MinBaseFee {
		return fmt.Errorf("max base fee %v < min base fee %v", o.MaxBaseFee, o.MinBaseFee)
	}
	if o.MaxPriorityFee < o.MinPriorityFee {
		return fmt.Errorf("max priority fee %v < min priority fee %v", o.MaxPriorityFee, o.MinPriorityFee)
	}

	return nil
}

type TestChain struct {
	TestChainOptions

	chain, snapshot []*types.Transaction
	fork            uint64
	mempool         map[uint64][]*types.Transaction
	highestNonce    *uint64
	mu              sync.RWMutex

	subscriptions   map[chan ethfinalizer.Diff]struct{}
	subscriptionsMu sync.RWMutex
}

func NewTestChain(options TestChainOptions) (*TestChain, error) {
	if err := options.IsValid(); err != nil {
		return nil, err
	}

	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}

	return &TestChain{
		TestChainOptions: options,

		mempool: map[uint64][]*types.Transaction{},

		subscriptions: map[chan ethfinalizer.Diff]struct{}{},
	}, nil
}

func (c *TestChain) SetNonce(nonce uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if nonce < uint64(len(c.chain)) {
		c.Logger.Debug("revert", slog.Int("from", len(c.chain)), slog.Uint64("to", nonce))

		c.chain = c.chain[:nonce]

		if nonce < c.fork {
			c.fork = nonce
		}
	}

	for uint64(len(c.chain)) < nonce {
		mempool := c.mempool[uint64(len(c.chain))]
		if len(mempool) == 0 {
			break
		}

		transaction := mempool[rand.Intn(len(mempool))]
		c.chain = append(c.chain, transaction)

		c.Logger.Debug("mine", slog.String("transaction", transaction.Hash().String()), slog.Uint64("nonce", transaction.Nonce()))
	}
}

func (c *TestChain) Publish() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.subscriptionsMu.RLock()
	defer c.subscriptionsMu.RUnlock()

	diff := ethfinalizer.Diff{
		Removed: map[common.Hash]struct{}{},
		Added:   map[common.Hash]struct{}{},
	}

	for _, transaction := range c.snapshot[c.fork:] {
		diff.Removed[transaction.Hash()] = struct{}{}
	}

	for _, transaction := range c.chain[c.fork:] {
		diff.Added[transaction.Hash()] = struct{}{}
	}

	c.snapshot = make([]*types.Transaction, len(c.chain))
	copy(c.snapshot, c.chain)

	c.fork = uint64(len(c.chain))

	for subscription := range c.subscriptions {
		subscription <- diff
	}
}

func (c *TestChain) ChainID() *big.Int {
	if c.TestChainOptions.ChainID == nil {
		return big.NewInt(31337)
	}

	return new(big.Int).Set(c.TestChainOptions.ChainID)
}

func (c *TestChain) IsEIP1559() bool {
	return c.TestChainOptions.IsEIP1559
}

func (c *TestChain) LatestNonce(ctx context.Context, address common.Address) (uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return uint64(len(c.chain)), nil
}

func (c *TestChain) PendingNonce(ctx context.Context, address common.Address) (uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.highestNonce == nil {
		return 0, nil
	} else {
		return *c.highestNonce + 1, nil
	}
}

func (c *TestChain) Send(ctx context.Context, transaction *types.Transaction) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if transaction.Nonce() < uint64(len(c.chain)) {
		return fmt.Errorf("nonce too low")
	}

	c.mempool[transaction.Nonce()] = append(c.mempool[transaction.Nonce()], transaction)

	if c.highestNonce == nil || transaction.Nonce() > *c.highestNonce {
		c.highestNonce = new(uint64)
		*c.highestNonce = transaction.Nonce()
	}

	return nil
}

func (c *TestChain) GasPrice(ctx context.Context) (*big.Int, error) {
	return new(big.Int).SetUint64(uniform(c.MinGasPrice, c.MaxGasPrice)), nil
}

func (c *TestChain) BaseFee(ctx context.Context) (*big.Int, error) {
	return new(big.Int).SetUint64(uniform(c.MinBaseFee, c.MaxBaseFee)), nil
}

func (c *TestChain) PriorityFee(ctx context.Context) (*big.Int, error) {
	return new(big.Int).SetUint64(uniform(c.MinPriorityFee, c.MaxPriorityFee)), nil
}

func (c *TestChain) Subscribe(ctx context.Context) (<-chan ethfinalizer.Diff, error) {
	c.subscriptionsMu.Lock()
	defer c.subscriptionsMu.Unlock()

	subscription := make(chan ethfinalizer.Diff)
	c.subscriptions[subscription] = struct{}{}

	go func() {
		<-ctx.Done()

		c.subscriptionsMu.Lock()
		defer c.subscriptionsMu.Unlock()

		delete(c.subscriptions, subscription)
		close(subscription)
	}()

	return subscription, nil
}

func uniform(min, max uint64) uint64 {
	if max < min {
		panic(fmt.Errorf("max %v < min %v", max, min))
	}

	width := max - min + 1
	if width == 0 {
		return rand.Uint64()
	}

	limit := ^uint64(0) / width * width
	for {
		value := rand.Uint64()
		if value < limit {
			return min + value%width
		}
	}
}
