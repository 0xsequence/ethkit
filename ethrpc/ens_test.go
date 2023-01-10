package ethrpc_test

import (
	"context"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/stretchr/testify/assert"
)

var (
	ctx      context.Context
	provider *ethrpc.Provider
)

func init() {
	ctx = context.Background()

	// Mainnet Node only
	provider, _ = ethrpc.NewProvider("https://mainnet...node...url...")
}

func XXTestEns(t *testing.T) {
	{
		address, ok, err := ethrpc.ResolveEnsAddress(ctx, "0xsequence.eth", provider)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, address.Hex(), "0xdAd546CA5332d24c02f5834ce2fac07197677Eac")
	}
	{
		address, ok, err := ethrpc.ResolveEnsAddress(ctx, "vitalik.eth", provider)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, address.Hex(), "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045")
	}
	{
		_, ok, _ := ethrpc.ResolveEnsAddress(ctx, "0x22", provider)
		assert.False(t, ok)
	}
	{
		address, ok, err := ethrpc.ResolveEnsAddress(ctx, "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", provider)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, address.Hex(), "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045")
	}
}
