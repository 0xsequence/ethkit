package ethfinalizer

import (
	"context"
	"math/big"

	"github.com/0xsequence/ethkit/ethwallet"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

// Signer signs transactions without requiring access to private key material.
// Implementations must respect cancellation and return a transaction signed by Address.
type Signer interface {
	Address() common.Address
	SignTransaction(context.Context, *types.Transaction, *big.Int) (*types.Transaction, error)
}

type walletSigner struct{ wallet *ethwallet.Wallet }

// NewWalletSigner adapts a local wallet to Signer. The wallet must be non-nil.
func NewWalletSigner(wallet *ethwallet.Wallet) Signer { return &walletSigner{wallet: wallet} }

func (s *walletSigner) Address() common.Address { return s.wallet.Address() }

func (s *walletSigner) SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.wallet.SignTransaction(tx, chainID)
}
