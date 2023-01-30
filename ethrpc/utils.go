package ethrpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
)

func WaitForTxnReceipt(ctx context.Context, provider *Provider, txHash common.Hash) (*types.Receipt, error) {
	var clearTimeout context.CancelFunc
	if _, ok := ctx.Deadline(); !ok {
		ctx, clearTimeout = context.WithTimeout(ctx, 120*time.Second) // default timeout of 120 seconds
		defer clearTimeout()
	}

	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("ethwallet, WaitReceipt for %v: %w", txHash, err)
			}
		default:
		}

		receipt, err := provider.TransactionReceipt(ctx, txHash)
		if err != nil && !errors.Is(err, ethereum.NotFound) {
			return nil, err
		}

		if receipt != nil {
			return receipt, nil
		}

		time.Sleep(1 * time.Second)
	}
}
