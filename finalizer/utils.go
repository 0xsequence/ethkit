package finalizer

import (
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

func withNonce(transaction *types.Transaction, nonce uint64) *types.Transaction {
	switch transaction.Type() {
	case types.LegacyTxType:
		return types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: transaction.GasPrice(),
			Gas:      transaction.Gas(),
			To:       transaction.To(),
			Value:    transaction.Value(),
			Data:     transaction.Data(),
		})

	case types.AccessListTxType:
		return types.NewTx(&types.AccessListTx{
			ChainID:    transaction.ChainId(),
			Nonce:      nonce,
			GasPrice:   transaction.GasPrice(),
			Gas:        transaction.Gas(),
			To:         transaction.To(),
			Value:      transaction.Value(),
			Data:       transaction.Data(),
			AccessList: transaction.AccessList(),
		})

	case types.DynamicFeeTxType:
		return types.NewTx(&types.DynamicFeeTx{
			ChainID:    transaction.ChainId(),
			Nonce:      nonce,
			GasTipCap:  transaction.GasTipCap(),
			GasFeeCap:  transaction.GasFeeCap(),
			Gas:        transaction.Gas(),
			To:         transaction.To(),
			Value:      transaction.Value(),
			Data:       transaction.Data(),
			AccessList: transaction.AccessList(),
		})

	case types.BlobTxType:
		return types.NewTx(&types.BlobTx{
			ChainID:    uint256.MustFromBig(transaction.ChainId()),
			Nonce:      nonce,
			GasTipCap:  uint256.MustFromBig(transaction.GasTipCap()),
			GasFeeCap:  uint256.MustFromBig(transaction.GasFeeCap()),
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
		return types.NewTx(&types.SetCodeTx{
			ChainID:    uint256.MustFromBig(transaction.ChainId()),
			Nonce:      nonce,
			GasTipCap:  uint256.MustFromBig(transaction.GasTipCap()),
			GasFeeCap:  uint256.MustFromBig(transaction.GasFeeCap()),
			Gas:        transaction.Gas(),
			To:         dereference(transaction.To()),
			Value:      uint256.MustFromBig(transaction.Value()),
			Data:       transaction.Data(),
			AccessList: transaction.AccessList(),
			AuthList:   transaction.SetCodeAuthorizations(),
		})

	default:
		panic(fmt.Errorf("unknown transaction type %v", transaction.Type()))
	}
}

func withMargin(value *big.Int, margin int) *big.Int {
	if value == nil {
		return new(big.Int)
	}

	return new(big.Int).Div(
		new(big.Int).Add(
			new(big.Int).Mul(
				value,
				big.NewInt(100+int64(margin)),
			),
			big.NewInt(99),
		),
		big.NewInt(100),
	)
}

func maxBigInt(a, b *big.Int) *big.Int {
	if a == nil {
		a = new(big.Int)
	}
	if b == nil {
		b = new(big.Int)
	}

	if a.Cmp(b) >= 0 {
		return a
	} else {
		return b
	}
}

func dereference[T any](value *T) T {
	if value != nil {
		return *value
	} else {
		var zero T
		return zero
	}
}
