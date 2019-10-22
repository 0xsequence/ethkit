package ethwallet

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/arcadeum/ethkit/ethrpc"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
)

// WalletProvider is a helper to query the provider in context of the wallet address
type WalletProvider struct {
	wallet   *Wallet
	provider *ethrpc.Provider

	lastBlockNum  *big.Int
	lastBlockPoll time.Time

	nonceIndex map[common.Address][2]*big.Int

	mu sync.Mutex
}

func (w *WalletProvider) NewTransactor(ctx context.Context) (*bind.TransactOpts, error) {
	// Get the next txn nonce for the wallet in a thread-safe way
	nonce, err := w.GetNextTxnNonce(ctx)
	if err != nil {
		return nil, err
	}

	// Get suggested gas price, the user can change this on their own too
	gasPrice, err := w.provider.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	auth := w.wallet.Transactor()
	auth.Nonce = nonce
	auth.Value = big.NewInt(0)
	auth.GasLimit = 0 // (0 = estimate)
	auth.GasPrice = gasPrice

	return auth, nil
}

func (w *WalletProvider) GetEtherBalanceAt(ctx context.Context, blockNumber *big.Int) (*big.Int, error) {
	balance, err := w.provider.BalanceAt(ctx, w.wallet.Address(), blockNumber)
	if err != nil {
		return nil, err
	}
	return balance, nil
}

func (w *WalletProvider) GetTransactionCount(ctx context.Context) (uint64, error) {
	nonce, err := w.provider.PendingNonceAt(ctx, w.wallet.Address())
	if err != nil {
		return 0, err
	}
	return nonce, nil
}

// GetNextTxnNonce will return the next account txn nonce in a thread-safe way.
func (w *WalletProvider) GetNextTxnNonce(ctx context.Context) (*big.Int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	blockNum := w.lastBlockNum
	if blockNum == nil || time.Since(w.lastBlockPoll) > (2*time.Second) {
		block, err := w.provider.HeaderByNumber(ctx, nil)
		if err != nil {
			return nil, err
		}
		blockNum = block.Number
		w.lastBlockNum = blockNum
		w.lastBlockPoll = time.Now()
	}

	if w.nonceIndex == nil {
		w.nonceIndex = map[common.Address][2]*big.Int{}
	}

	address := w.wallet.Address()
	idx, ok := w.nonceIndex[address]
	if !ok {
		idx = [2]*big.Int{blockNum, nil}
	}

	if idx[1] == nil || blockNum.Uint64() > idx[0].Uint64() {
		txnCount, err := w.GetTransactionCount(ctx)
		if err != nil {
			return nil, err
		}

		idx[0] = blockNum
		if idx[1] == nil || txnCount > idx[1].Uint64() {
			idx[1] = big.NewInt(int64(txnCount))
		} else if txnCount == idx[1].Uint64() {
			idx[1] = idx[1].Add(idx[1], big.NewInt(1))
		} else {
			return nil, errors.Errorf("ethwallet: nonce index out-of-sync for address %s", address.String())
		}
	} else {
		idx[1] = idx[1].Add(idx[1], big.NewInt(1))
	}

	w.nonceIndex[address] = idx
	return idx[1], nil
}

// TODO
// func (w *WalletProvider) SendTransaction() {
// }
