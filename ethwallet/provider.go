package ethwallet

import (
	"context"
	"math/big"
	"net/http"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi/bind"
)

// WalletProvider is a helper to query the provider in context of the wallet address
type WalletProvider struct {
	wallet   *Wallet
	provider *ethrpc.Provider
}

func (w *WalletProvider) Backend() *ethrpc.Provider {
	return w.provider
}

func (w *WalletProvider) NewTransactor(ctx context.Context) (*bind.TransactOpts, *http.Request, *http.Response, error) {
	// Get suggested gas price, the user can change this on their own too
	gasPrice, req, resp, err := w.provider.SuggestGasPrice(ctx)
	if err != nil {
		return nil, req, resp, err
	}

	auth := w.wallet.Transactor()
	auth.Value = big.NewInt(0)
	auth.GasLimit = 0 // (0 = estimate)
	auth.GasPrice = gasPrice
	auth.Nonce = nil // remains unset, will be auto-set or user can specify

	return auth, req, resp, nil
}

func (w *WalletProvider) GetEtherBalanceAt(ctx context.Context, blockNumber *big.Int) (*big.Int, *http.Request, *http.Response, error) {
	balance, req, resp, err := w.provider.BalanceAt(ctx, w.wallet.Address(), blockNumber)
	if err != nil {
		return nil, req, resp, err
	}
	return balance, req, resp, nil
}

func (w *WalletProvider) GetTransactionCount(ctx context.Context) (uint64, *http.Request, *http.Response, error) {
	nonce, req, resp, err := w.provider.PendingNonceAt(ctx, w.wallet.Address())
	if err != nil {
		return 0, req, resp, err
	}
	return nonce, req, resp, nil
}

// TODO

// func (w *WalletProvider) SendTransaction(abi abi.ABI, contractAddress common.Address, ) {
// }
