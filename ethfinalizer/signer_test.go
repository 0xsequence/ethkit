package ethfinalizer

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

type externalTestSigner struct {
	Signer
	fail  atomic.Bool
	calls atomic.Int32
}

func (s *externalTestSigner) SignTransaction(ctx context.Context, tx *types.Transaction, chain *big.Int) (*types.Transaction, error) {
	s.calls.Add(1)
	if s.fail.Load() {
		return nil, errors.New("signer unavailable")
	}
	return s.Signer.SignTransaction(ctx, tx, chain)
}

type externalTestChain struct {
	gas  atomic.Int64
	sent chan *types.Transaction
}

func (c *externalTestChain) ChainID() *big.Int { return big.NewInt(1) }
func (c *externalTestChain) IsEIP1559() bool   { return false }
func (c *externalTestChain) LatestNonce(context.Context, common.Address) (uint64, error) {
	return 0, nil
}
func (c *externalTestChain) PendingNonce(context.Context, common.Address) (uint64, error) {
	return 0, nil
}
func (c *externalTestChain) GasPrice(context.Context) (*big.Int, error) {
	return big.NewInt(c.gas.Load()), nil
}
func (c *externalTestChain) BaseFee(context.Context) (*big.Int, error)     { return nil, nil }
func (c *externalTestChain) PriorityFee(context.Context) (*big.Int, error) { return nil, nil }
func (c *externalTestChain) Send(ctx context.Context, tx *types.Transaction) error {
	select {
	case c.sent <- tx:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *externalTestChain) Subscribe(ctx context.Context) (<-chan Diff, error) {
	ch := make(chan Diff)
	go func() { <-ctx.Done(); close(ch) }()
	return ch, nil
}

func TestNewWalletSignerNil(t *testing.T) {
	signer := NewWalletSigner(nil)
	require.True(t, signer == nil, "nil wallet must produce a nil interface")

	finalizer, err := NewFinalizer(FinalizerOptions[string]{
		Signer:       signer,
		Chain:        &externalTestChain{},
		Mempool:      NewMemoryMempool[string](),
		PollInterval: time.Second,
		PollTimeout:  time.Second,
		RetryDelay:   time.Second,
	})
	require.ErrorContains(t, err, "exactly one of wallet or signer is required")
	require.Nil(t, finalizer)
}

func TestExternalSignerLifecycle(t *testing.T) {
	wallet, err := ethwallet.NewWalletFromRandomEntropy()
	require.NoError(t, err)
	signer := &externalTestSigner{Signer: NewWalletSigner(wallet)}
	chain := &externalTestChain{sent: make(chan *types.Transaction, 100)}
	chain.gas.Store(10)
	mempool := NewMemoryMempool[string]()
	options := FinalizerOptions[string]{Signer: signer, Chain: chain, Mempool: mempool, PollInterval: 5 * time.Millisecond, PollTimeout: time.Second, RetryDelay: time.Millisecond, PriceBump: 15}
	f, err := NewFinalizer(options)
	require.NoError(t, err)
	options.Wallet = wallet
	_, err = NewFinalizer(options)
	require.Error(t, err)
	options.Signer = nil
	_, err = NewFinalizer(options)
	require.NoError(t, err)
	options.Wallet = nil
	_, err = NewFinalizer(options)
	require.Error(t, err)
	tx := types.NewTx(&types.LegacyTx{Gas: 21000, GasPrice: big.NewInt(10)})
	signer.fail.Store(true)
	_, err = f.Send(context.Background(), tx, "metadata")
	require.Error(t, err)
	nonce, err := mempool.Nonce(context.Background())
	require.NoError(t, err)
	require.Zero(t, nonce)
	require.Empty(t, chain.sent)
	signer.fail.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = f.Send(ctx, tx, "metadata")
	require.ErrorIs(t, err, context.Canceled)
	signed, err := f.Send(context.Background(), tx, "metadata")
	require.NoError(t, err)
	require.Equal(t, signed.Hash(), (<-chain.sent).Hash())
	calls := signer.calls.Load()
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.Run(ctx) }()
	select {
	case resent := <-chain.sent:
		require.Equal(t, signed.Hash(), resent.Hash())
	case <-time.After(time.Second):
		t.Fatal("no rebroadcast")
	}
	require.Equal(t, calls, signer.calls.Load(), "rebroadcast must reuse persisted signature")
	signer.fail.Store(true)
	chain.gas.Store(20)
	require.Eventually(t, func() bool { return signer.calls.Load() > calls }, time.Second, time.Millisecond)
	_, latest, err := mempool.Status(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, signed.Hash(), latest.Transaction.Hash())
	signer.fail.Store(false)
	require.Eventually(t, func() bool {
		_, latest, err := mempool.Status(context.Background(), 0)
		return err == nil && latest.Transaction.GasPrice().Int64() == 20
	}, time.Second, time.Millisecond)
	_, latest, err = mempool.Status(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, "metadata", latest.Transaction.Metadata)
	require.Zero(t, latest.Transaction.Nonce())
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("finalizer did not stop")
	}
}
