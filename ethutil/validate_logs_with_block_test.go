package ethutil

import (
	"bytes"
	"context"
	"math/big"
	"testing"

	"github.com/0xsequence/ethkit/ethrpc"
	"github.com/0xsequence/ethkit/go-ethereum"
	"github.com/0xsequence/ethkit/go-ethereum/common"
	"github.com/0xsequence/ethkit/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestValidateLogsWithBlockHeader(t *testing.T) {
	p, err := ethrpc.NewProvider("https://nodes.sequence.app/polygon")
	require.NoError(t, err)

	header, err := p.HeaderByNumber(context.Background(), big.NewInt(20_000_003))
	require.NoError(t, err)
	require.NotNil(t, header)

	logs, err := p.FilterLogs(context.Background(), ethereum.FilterQuery{
		FromBlock: big.NewInt(20_000_003),
		ToBlock:   big.NewInt(20_000_003),
	})
	require.NoError(t, err)

	require.True(t, ValidateLogsWithBlockHeader(logs, header))
}

func TestValidateLogsWithBlockHeaderWithCustomCheck(t *testing.T) {
	logs := []types.Log{
		{
			Address: common.HexToAddress("0x0000000000000000000000000000000000000001"),
			Topics:  []common.Hash{common.HexToHash("0x01")},
		},
		{
			Address: common.HexToAddress("0x0000000000000000000000000000000000000002"),
			Topics:  []common.Hash{common.HexToHash("0x02")},
		},
	}

	headerFull := &types.Header{Bloom: ConvertLogsToBloom(logs)}
	headerFiltered := &types.Header{Bloom: ConvertLogsToBloom(logs[1:])}

	require.True(t, ValidateLogsWithBlockHeader(logs, headerFull))
	require.False(t, ValidateLogsWithBlockHeader(logs, headerFiltered))

	customCheck := func(ls []types.Log, header *types.Header) bool {
		// Ignore the first log (e.g., system tx) and validate bloom against the remainder.
		filtered := ls[1:]
		return bytes.Equal(ConvertLogsToBloom(filtered).Bytes(), header.Bloom.Bytes())
	}

	require.True(t, ValidateLogsWithBlockHeader(logs, headerFiltered, customCheck))
}
