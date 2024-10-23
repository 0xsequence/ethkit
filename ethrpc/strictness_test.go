package ethrpc_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// TODO: add tests for transactionByHash , etc..

func TestStrictnessE2E(t *testing.T) {
	// Ethereum mainnet, no validation (default)
	{
		provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet")
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(1_000_000))
		require.NoError(t, err)
		require.Equal(t, "1000000", block.Number().String())
		require.Equal(t, "0x8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(1_000_000))
		require.NoError(t, err)
		require.Equal(t, "1000000", header.Number.String())
		require.Equal(t, "0x8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e", header.Hash().Hex())

		txnHash := "0xea1093d492a1dcb1bef708f771a99a96ff05dcab81ca76c31940300177fcf49f"
		require.Equal(t, 2, len(block.Transactions()))
		require.Equal(t, txnHash, block.Transactions()[0].Hash().Hex())

		txn, pending, err := provider.TransactionByHash(context.Background(), common.HexToHash(txnHash))
		require.NoError(t, err)
		require.False(t, pending)
		require.Equal(t, txnHash, txn.Hash().Hex())
	}

	// Ethereum mainnet, semi-strict validation
	{
		provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet", ethrpc.WithSemiValidation())
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(1_000_000))
		require.NoError(t, err)
		require.Equal(t, "1000000", block.Number().String())
		require.Equal(t, "0x8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(1_000_000))
		require.NoError(t, err)
		require.Equal(t, "1000000", header.Number.String())
		require.Equal(t, "0x8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e", header.Hash().Hex())

		txnHash := "0xea1093d492a1dcb1bef708f771a99a96ff05dcab81ca76c31940300177fcf49f"
		require.Equal(t, 2, len(block.Transactions()))
		require.Equal(t, txnHash, block.Transactions()[0].Hash().Hex())

		txn, pending, err := provider.TransactionByHash(context.Background(), common.HexToHash(txnHash))
		require.NoError(t, err)
		require.False(t, pending)
		require.Equal(t, txnHash, txn.Hash().Hex())
	}

	// Ethereum mainnet, strict validation
	{
		provider, err := ethrpc.NewProvider("https://nodes.sequence.app/mainnet", ethrpc.WithStrictValidation())
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(1_000_000))
		require.NoError(t, err)
		require.Equal(t, "1000000", block.Number().String())
		require.Equal(t, "0x8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(1_000_000))
		require.NoError(t, err)
		require.Equal(t, "1000000", header.Number.String())
		require.Equal(t, "0x8e38b4dbf6b11fcc3b9dee84fb7986e29ca0a02cecd8977c161ff7333329681e", header.Hash().Hex())

		txnHash := "0xea1093d492a1dcb1bef708f771a99a96ff05dcab81ca76c31940300177fcf49f"
		require.Equal(t, 2, len(block.Transactions()))
		require.Equal(t, txnHash, block.Transactions()[0].Hash().Hex())

		txn, pending, err := provider.TransactionByHash(context.Background(), common.HexToHash(txnHash))
		require.NoError(t, err)
		require.False(t, pending)
		require.Equal(t, txnHash, txn.Hash().Hex())
	}

	// Etherlink node, no validation (default)
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com")
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

	// Etherlink node, no validation (default) – testing txns
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com")
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(3750261))
		require.NoError(t, err)
		require.Equal(t, "3750261", block.Number().String())
		require.Equal(t, "0x9bda3d7b3e253112ab99f5d7f52bde8a4b206ece6cbf02bf6c1426927704e4d3", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(3750261))
		require.NoError(t, err)
		require.Equal(t, "3750261", header.Number.String())
		require.Equal(t, "0x9bda3d7b3e253112ab99f5d7f52bde8a4b206ece6cbf02bf6c1426927704e4d3", header.Hash().Hex())

		txnHash := "0xaf7cff635f61ec84e139a190945d9b9ab3f07d0524bef592e7a9b6e6bfc5b80e"
		require.Equal(t, 1, len(block.Transactions()))
		require.Equal(t, txnHash, block.Transactions()[0].Hash().Hex())

		txn, pending, err := provider.TransactionByHash(context.Background(), common.HexToHash(txnHash))
		require.NoError(t, err)
		require.False(t, pending)
		require.Equal(t, txnHash, txn.Hash().Hex())
	}

	// Etherlink node, semi-strict validation
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com", ethrpc.WithSemiValidation())
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

	// Etherlink node, semi-strict validation – testing txns
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com", ethrpc.WithSemiValidation())
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(3750261))
		require.NoError(t, err)
		require.Equal(t, "3750261", block.Number().String())
		require.Equal(t, "0x9bda3d7b3e253112ab99f5d7f52bde8a4b206ece6cbf02bf6c1426927704e4d3", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(3750261))
		require.NoError(t, err)
		require.Equal(t, "3750261", header.Number.String())
		require.Equal(t, "0x9bda3d7b3e253112ab99f5d7f52bde8a4b206ece6cbf02bf6c1426927704e4d3", header.Hash().Hex())

		txnHash := "0xaf7cff635f61ec84e139a190945d9b9ab3f07d0524bef592e7a9b6e6bfc5b80e"
		require.Equal(t, 1, len(block.Transactions()))
		require.Equal(t, txnHash, block.Transactions()[0].Hash().Hex())

		txn, pending, err := provider.TransactionByHash(context.Background(), common.HexToHash(txnHash))
		require.NoError(t, err)
		require.False(t, pending)
		require.Equal(t, txnHash, txn.Hash().Hex())
	}

	// Etherlink node, strict validation
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com", ethrpc.WithStrictValidation())
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

	// Etherlink node, strict validation – testing txns
	{
		provider, err := ethrpc.NewProvider("https://node.mainnet.etherlink.com", ethrpc.WithStrictValidation())
		require.NoError(t, err)

		block, err := provider.BlockByNumber(context.Background(), big.NewInt(3750261))
		require.NoError(t, err)
		require.Equal(t, "3750261", block.Number().String())
		// NOTE: this hash is actually invalid, because its computed from the rlp encoding,
		// which in the case of the etherlink.com is different then expected from other Ethereum nodes.
		require.Equal(t, "0x78ee37515270fa805637bfe280ca01887316f1e1774c5ac95984e22f5680d197", block.Hash().Hex())

		header, err := provider.HeaderByNumber(context.Background(), big.NewInt(3750261))
		require.NoError(t, err)
		require.Equal(t, "3750261", header.Number.String())
		require.Equal(t, "0x78ee37515270fa805637bfe280ca01887316f1e1774c5ac95984e22f5680d197", header.Hash().Hex())

		txnHash := "0xaf7cff635f61ec84e139a190945d9b9ab3f07d0524bef592e7a9b6e6bfc5b80e"
		require.Equal(t, 1, len(block.Transactions()))
		require.Equal(t, txnHash, block.Transactions()[0].Hash().Hex())

		txn, pending, err := provider.TransactionByHash(context.Background(), common.HexToHash(txnHash))
		require.NoError(t, err)
		require.False(t, pending)
		require.Equal(t, txnHash, txn.Hash().Hex())
	}

}
