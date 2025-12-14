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
func NewTransaction(ctx context.Context, provider ethrpc.Provider, txnRequest *TransactionRequest) (*types.Transaction, error) {
	if txnRequest == nil {
		return nil, fmt.Errorf("ethtxn: txnRequest is required")
	}
	if provider == nil {
		return nil, fmt.Errorf("ethtxn: provider is not set")
	}

	if txnRequest.Nonce == nil && txnRequest.From == (common.Address{}) {
		return nil, fmt.Errorf("ethtxn: from address is required when nonce is not set on txnRequest")
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

	// If we're constructing a legacy (or access-list) transaction (ie. GasTip is nil) on a
	// post-London network, gasPrice MUST be >= baseFee. The node error you saw:
	//   "max fee per gas less than block base fee" also applies to legacy txs internally
	// because the validation unifies the code paths. eth_gasPrice can occasionally return
	// a value slightly below the current baseFee if the baseFee just jumped. We defensively
	// bump it here. We also (optionally) add a small priority tip using SuggestGasTipCap so
	// the tx isn't sent with zero miner incentive (gasPrice == baseFee).
	if txnRequest.GasTip == nil { // staying in legacy/access-list mode
		if head, err := provider.HeaderByNumber(ctx, nil); err == nil && head != nil && head.BaseFee != nil {
			baseFee := head.BaseFee

			// Base fee can increase by up to 12.5% per block (EIP-1559). We add headroom so a
			// transaction constructed right before a block is sealed doesn't become invalid if
			// the next block's baseFee rises. headroomBaseFee = baseFee * 1050/1000 (~+5.0%).
			// headroomBaseFee := new(big.Int).Mul(baseFee, big.NewInt(1125)) // 12.5%
			headroomBaseFee := new(big.Int).Mul(baseFee, big.NewInt(1050)) // 5%
			headroomBaseFee.Div(headroomBaseFee, big.NewInt(1000))

			// We treat legacy gasPrice as (baseFee + implicit priority). Ensure gasPrice >= headroomBaseFee.
			if txnRequest.GasPrice.Cmp(headroomBaseFee) < 0 {
				// Optionally attempt to get a tip suggestion and add it on top of headroom base fee.
				if tip, err2 := provider.SuggestGasTipCap(ctx); err2 == nil && tip != nil {
					candidate := new(big.Int).Add(headroomBaseFee, tip)
					if txnRequest.GasPrice.Cmp(candidate) < 0 {
						txnRequest.GasPrice = candidate
					}
				} else {
					// Fallback just headroom base fee
					txnRequest.GasPrice = headroomBaseFee
				}
			}
		}
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

func SendTransaction(ctx context.Context, provider ethrpc.Provider, signedTxn *types.Transaction) (*types.Transaction, WaitReceipt, error) {
	if provider == nil {
		return nil, nil, fmt.Errorf("ethtxn (SendTransaction): provider is not set")
	}

	waitFn := func(ctx context.Context) (*types.Receipt, error) {
		return ethrpc.WaitForTxnReceipt(ctx, provider, signedTxn.Hash())
	}

	return signedTxn, waitFn, provider.SendTransaction(ctx, signedTxn)
}

var zeroBigInt = big.NewInt(0)

func AsMessage(txn *types.Transaction, optChainID ...*big.Int) (*core.Message, error) {
	chainID := txn.ChainId()
	if len(optChainID) > 0 {
		chainID = optChainID[0]
	}
	return AsMessageWithSigner(txn, types.NewPragueSigner(chainID), nil)
}

// AsMessageWithSigner decodes a transaction payload, and will check v, r, s values and skips
// zero'd numbers which is the case for Polygon state sync transactions:
// https://wiki.polygon.technology/docs/pos/state-sync/how-state-sync-works#state-sync-logs-and-bor-block-receipt
func AsMessageWithSigner(txn *types.Transaction, signer types.Signer, baseFee *big.Int) (*core.Message, error) {
	v, r, s := txn.RawSignatureValues()
	if v.Cmp(zeroBigInt) == 0 && r.Cmp(zeroBigInt) == 0 && s.Cmp(zeroBigInt) == 0 {
		return &core.Message{
			To:              txn.To(),
			Nonce:           txn.Nonce(),
			Value:           txn.Value(),
			GasLimit:        txn.Gas(),
			GasPrice:        txn.GasPrice(),
			GasFeeCap:       txn.GasFeeCap(),
			GasTipCap:       txn.GasTipCap(),
			Data:            txn.Data(),
			AccessList:      txn.AccessList(),
			SkipNonceChecks: true,
		}, nil
	} else {
		return core.TransactionToMessage(txn, signer, baseFee)
	}
}
