package ethrpc_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ExampleBatchCall() {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon/test")
	if err != nil {
		panic(err)
	}

	var (
		chainID  *big.Int
		header   *types.Header
		errBlock *types.Block
	)
	err = p.Do(
		context.Background(),
		ethrpc.ChainID().Into(&chainID),
		ethrpc.HeaderByNumber(big.NewInt(38470000)).Into(&header),
		ethrpc.BlockByHash(common.BytesToHash([]byte("a1b2c3"))).Into(&errBlock),
	)
	fmt.Printf("polygon ID: %s\n", chainID.String())
	if err != nil {
		if batchErr, ok := err.(ethrpc.BatchError); ok {
			for i, err := range batchErr {
				fmt.Printf("error at %d: %s\n", i, err)
			}
		}
	}
	// Output:
	// polygon ID: 137
	// error at 2: not found
}

func TestETHRPC(t *testing.T) {
	t.Run("Single", func(t *testing.T) {
		p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon/test")
		require.NoError(t, err)

		chainID, err := p.ChainID(context.Background())
		require.NoError(t, err)
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(137), chainID.Uint64())
	})

	t.Run("Batch", func(t *testing.T) {
		p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon/test")
		require.NoError(t, err)

		var (
			chainID     *big.Int
			blockNumber uint64
			header      *types.Header
		)
		err = p.Do(
			context.Background(),
			ethrpc.ChainID().Into(&chainID),
			ethrpc.BlockNumber().Into(&blockNumber),
			ethrpc.HeaderByNumber(big.NewInt(38470000)).Into(&header),
		)
		require.NoError(t, err)
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(137), chainID.Uint64())
		assert.Greater(t, blockNumber, uint64(0))
		assert.Equal(t, uint64(38470000), header.Number.Uint64())
	})
}
