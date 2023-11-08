package ethproviders_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethproviders"
	"github.com/stretchr/testify/require"
)

func TestBasic(t *testing.T) {
	cfg := ethproviders.Config{
		"polygon": ethproviders.NetworkConfig{
			ID:  137,
			URL: "https://dev-nodes.sequence.app/polygon",
		},
	}

	ps, err := ethproviders.NewProviders(cfg) //, "xx")
	require.NoError(t, err)
	p := ps.Get("polygon")
	require.NotNil(t, p)

	block, err := p.BlockByNumber(context.Background(), big.NewInt(1_000_000))
	require.NoError(t, err)
	require.NotNil(t, block)
	require.Equal(t, uint64(1_000_000), block.NumberU64())
}
