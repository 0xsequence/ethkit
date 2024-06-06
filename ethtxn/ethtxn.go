package ethtxn

import (
	"context"
	"fmt"
	"math/big"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

type TransactionRequest struct {
	// Ethereum account to send the transaction from. Optional, will automatically be set.
	From common.Address

	// To is the recipient address, can be account, contract or nil. If `to` is nil, it will assume contract creation
	To *common.Address

	// Nonce is the nonce of the transaction for the sender. If this value is left empty (nil), it will
	// automatically be assigned.
	Nonce *big.Int

	// GasLimit is the total gas the transaction is expected the consume. If this value is left empty (0), it will
	// automatically be estimated and assigned.
	GasLimit uint64

	// GasPrice (in WEI) offering to pay for per unit of gas. If this value is left empty (nil), it will
	// automatically be sampled and assigned.
	// Used as GasFeeCap, but name kept for compatibility reasons
	GasPrice *big.Int

	// GasTip (in WEI) optional offering to pay for per unit of gas to the miner.
	// If this value is left empty (nil), it will be considered a pre-EIP1559 or "legacy" transaction
	GasTip *big.Int

	// AccessList optional key-values to pre-import
	// saves cost by pre-importing storage related values before executing the tx
	AccessList types.AccessList

	// ETHValue (in WEI) amount of ETH currency to send with this transaction. Optional.
	ETHValue *big.Int

	// Data is calldata / input when calling or creating a contract. Optional.
	Data []byte
}

type WaitReceipt func(ctx context.Context) (*types.Receipt, error)

// NewTransaction prepares a transaction for delivery, however the transaction still needs to be signed
// before it can be sent.
func NewTransaction(ctx context.Context, provider *ethrpc.Provider, txnRequest *TransactionRequest) (*types.Transaction, error) {
	if txnRequest == nil {
		return nil, fmt.Errorf("ethtxn: txnRequest is required")
	}
	if provider == nil {
		return nil, fmt.Errorf("ethtxn: provider is not set")
	}

	if txnRequest.Nonce == nil {
		nonce, err := provider.PendingNonceAt(ctx, txnRequest.From)
		if err != nil {
			return nil, fmt.Errorf("ethtxn: failed to get pending nonce: %w", err)
		}
		txnRequest.Nonce = big.NewInt(0).SetUint64(nonce)
	}

	if txnRequest.GasPrice == nil {
		// Get suggested gas price, the user can change this on their own too
		gasPrice, err := provider.SuggestGasPrice(ctx)
		if err != nil {
			return nil, fmt.Errorf("ethtxn: %w", err)
		}
		txnRequest.GasPrice = gasPrice
	}

	if txnRequest.GasLimit == 0 {
		callMsg := ethereum.CallMsg{
			From:     txnRequest.From,
			To:       txnRequest.To,
			Gas:      0, // estimating this value
			GasPrice: txnRequest.GasPrice,
			Value:    txnRequest.ETHValue,
			Data:     txnRequest.Data,
		}

		gasLimit, err := provider.EstimateGas(ctx, callMsg)
		if err != nil {
			return nil, fmt.Errorf("ethtxn: %w", err)
		}
		txnRequest.GasLimit = gasLimit
	}

	if txnRequest.To == nil && len(txnRequest.Data) == 0 {
		return nil, fmt.Errorf("ethtxn: contract creation txn request requires data field")
	}

	var rawTx *types.Transaction
	if txnRequest.GasTip != nil {
		chainId, err := provider.ChainID(ctx)
		if err != nil {
			return nil, err
		}

		rawTx = types.NewTx(&types.DynamicFeeTx{
			ChainID:    chainId,
			To:         txnRequest.To,
			Nonce:      txnRequest.Nonce.Uint64(),
			Value:      txnRequest.ETHValue,
			GasFeeCap:  txnRequest.GasPrice,
			GasTipCap:  txnRequest.GasTip,
			Data:       txnRequest.Data,
			Gas:        txnRequest.GasLimit,
			AccessList: txnRequest.AccessList,
		})
	} else if txnRequest.AccessList != nil {
		chainId, err := provider.ChainID(ctx)
		if err != nil {
			return nil, err
		}

		rawTx = types.NewTx(&types.AccessListTx{
			ChainID:    chainId,
			To:         txnRequest.To,
			Gas:        txnRequest.GasLimit,
			GasPrice:   txnRequest.GasPrice,
			Data:       txnRequest.Data,
			Nonce:      txnRequest.Nonce.Uint64(),
			Value:      txnRequest.ETHValue,
			AccessList: txnRequest.AccessList,
		})
	} else {
		rawTx = types.NewTx(&types.LegacyTx{
			To:       txnRequest.To,
			Gas:      txnRequest.GasLimit,
			GasPrice: txnRequest.GasPrice,
			Data:     txnRequest.Data,
			Nonce:    txnRequest.Nonce.Uint64(),
			Value:    txnRequest.ETHValue,
		})
	}

	return rawTx, nil
}

func SendTransaction(ctx context.Context, provider *ethrpc.Provider, signedTx *types.Transaction) (*types.Transaction, WaitReceipt, error) {
	if provider == nil {
		return nil, nil, fmt.Errorf("ethtxn (SendTransaction): provider is not set")
	}

	waitFn := func(ctx context.Context) (*types.Receipt, error) {
		return ethrpc.WaitForTxnReceipt(ctx, provider, signedTx.Hash())
	}

	return signedTx, waitFn, provider.SendTransaction(ctx, signedTx)
}

var zeroBigInt = big.NewInt(0)

func AsMessage(txn *types.Transaction) (*core.Message, error) {
	return AsMessageWithSigner(txn, types.NewLondonSigner(txn.ChainId()), nil)
}

// AsMessageWithSigner decodes a transaction payload, and will check v, r, s values and skips
// zero'd numbers which is the case for Polygon state sync transactions:
// https://wiki.polygon.technology/docs/pos/state-sync/how-state-sync-works#state-sync-logs-and-bor-block-receipt
func AsMessageWithSigner(txn *types.Transaction, signer types.Signer, baseFee *big.Int) (*core.Message, error) {
	v, r, s := txn.RawSignatureValues()
	if v.Cmp(zeroBigInt) == 0 && r.Cmp(zeroBigInt) == 0 && s.Cmp(zeroBigInt) == 0 {
		return &core.Message{
			To:                txn.To(),
			Nonce:             txn.Nonce(),
			Value:             txn.Value(),
			GasLimit:          txn.Gas(),
			GasPrice:          txn.GasPrice(),
			GasFeeCap:         txn.GasFeeCap(),
			GasTipCap:         txn.GasTipCap(),
			Data:              txn.Data(),
			AccessList:        txn.AccessList(),
			SkipAccountChecks: true,
		}, nil
	} else {
		return core.TransactionToMessage(txn, signer, baseFee)
	}
}
