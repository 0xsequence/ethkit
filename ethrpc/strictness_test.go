package ethrpc_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/stretchr/testify/require"
)

func TestStrictnessE2E(t *testing.T) {
	// Ethereum mainnet, no validation
	{
		provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet", ethrpc.WithNoValidation())
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(100_000))
		require.NoError(t, err)
		require.Equal(t, "100000", block.Number().String())
		require.Equal(t, "0x91c90676cab257a59cd956d7cb0bceb9b1a71d79755c23c7277a0697ccfaf8c4", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(100_000))
		require.NoError(t, err)
		require.Equal(t, "100000", header.Number.String())
		require.Equal(t, "0x91c90676cab257a59cd956d7cb0bceb9b1a71d79755c23c7277a0697ccfaf8c4", header.Hash().Hex())
	}

	// Ethereum mainnet, default validation (aka, unspecified)
	{
		provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet")
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(100_000))
		require.NoError(t, err)
		require.Equal(t, "100000", block.Number().String())
		require.Equal(t, "0x91c90676cab257a59cd956d7cb0bceb9b1a71d79755c23c7277a0697ccfaf8c4", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(100_000))
		require.NoError(t, err)
		require.Equal(t, "100000", header.Number.String())
		require.Equal(t, "0x91c90676cab257a59cd956d7cb0bceb9b1a71d79755c23c7277a0697ccfaf8c4", header.Hash().Hex())
	}

	// Ethereum mainnet, strict validation
	{
		provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet", ethrpc.WithStrictnessLevel(ethrpc.StrictnessLevel_Strict))
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(100_000))
		require.NoError(t, err)
		require.Equal(t, "100000", block.Number().String())
		require.Equal(t, "0x91c90676cab257a59cd956d7cb0bceb9b1a71d79755c23c7277a0697ccfaf8c4", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(100_000))
		require.NoError(t, err)
		require.Equal(t, "100000", header.Number.String())
		require.Equal(t, "0x91c90676cab257a59cd956d7cb0bceb9b1a71d79755c23c7277a0697ccfaf8c4", header.Hash().Hex())
	}

	// Etherlink node, no validation
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com", ethrpc.WithNoValidation())
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(3750259))
		require.NoError(t, err)
		require.Equal(t, "3750259", block.Number().String())
		require.Equal(t, "0x84466d04eec7eb4f6de99e6ac5db9484c042ca7ad5148a61598165edcf93cb1c", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(3750259))
		require.NoError(t, err)
		require.Equal(t, "3750259", header.Number.String())
		require.Equal(t, "0x84466d04eec7eb4f6de99e6ac5db9484c042ca7ad5148a61598165edcf93cb1c", header.Hash().Hex())
	}

	// Etherlink node, strict validation – results in invalid block hash
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com", ethrpc.WithStrictnessLevel(ethrpc.StrictnessLevel_Strict))
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(3750259))
		require.NoError(t, err)
		require.Equal(t, "3750259", block.Number().String())
		// NOTE: this hash is actually invalid, because its computed from the rlp encoding,
		// which in the case of the etherlink.com is different then expected from other Ethereum nodes.
		require.Equal(t, "0x7adc57e8f52c66a78c6ae4ad2005576ea190c1d8c990b50660014b8e3dcfba0e", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(3750259))
		require.NoError(t, err)
		require.Equal(t, "3750259", header.Number.String())
		require.Equal(t, "0x7adc57e8f52c66a78c6ae4ad2005576ea190c1d8c990b50660014b8e3dcfba0e", header.Hash().Hex())
	}
}
