package ethrpc2_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xsequence/ethkit/ethrpc2"
)

func TestETHRPC(t *testing.T) {
	t.Run("Single", func(t *testing.T) {
		p, err := ethrpc2.NewProvider(context.Background(), "https://dev-nodes.sequence.app/polygon")
		require.NoError(t, err)

		chainID, err := p.ChainID(context.Background())
		require.NoError(t, err)
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(137), chainID.Uint64())
	})

	t.Run("Batch", func(t *testing.T) {
		p, err := ethrpc2.NewProvider(context.Background(), "https://dev-nodes.sequence.app/polygon")
		require.NoError(t, err)

		var (
			chainID     *big.Int
			blockNumber uint64
		)
		err = p.Do(
			context.Background(),
			ethrpc2.ChainID().Into(&chainID),
			ethrpc2.BlockNumber().Into(&blockNumber),
		)
		require.NoError(t, err)
		require.NotNil(t, chainID)
		assert.Equal(t, uint64(137), chainID.Uint64())
		assert.Greater(t, blockNumber, uint64(0))
	})
}
